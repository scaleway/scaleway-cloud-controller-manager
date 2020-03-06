/*
Copyright 2018 Scaleway

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scaleway

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/api/lb/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

type syncController struct {
	client            *client
	lbAPI             LoadBalancerAPI
	instanceAPI       InstanceAPI
	clientSet         *kubernetes.Clientset
	nodeIndexer       cache.Indexer
	nodeController    cache.Controller
	serviceIndexer    cache.Indexer
	serviceController cache.Controller
}

var (
	splitRegexp  = regexp.MustCompile(`[:= ]+`)
	labelsRegexp = regexp.MustCompile(`^(([A-Za-z0-9][-A-Za-z0-9_\\.]*)?[A-Za-z0-9])?$`)
	labelsPrefix = "k8s.scaleway.com/"
)

func newSyncController(client *client, clientset *kubernetes.Clientset, cacheUpdateFrequency time.Duration) *syncController {
	nodeListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
	serviceListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "services", v1.NamespaceAll, fields.Everything())

	nodeIndexer, nodeController := cache.NewIndexerInformer(nodeListWatcher, &v1.Node{}, cacheUpdateFrequency, cache.ResourceEventHandlerFuncs{}, cache.Indexers{})
	serviceIndexer, serviceController := cache.NewIndexerInformer(serviceListWatcher, &v1.Service{}, cacheUpdateFrequency, cache.ResourceEventHandlerFuncs{}, cache.Indexers{})

	return &syncController{
		client:            client,
		clientSet:         clientset,
		instanceAPI:       instance.NewAPI(client.scaleway),
		lbAPI:             lb.NewAPI(client.scaleway),
		nodeIndexer:       nodeIndexer,
		nodeController:    nodeController,
		serviceIndexer:    serviceIndexer,
		serviceController: serviceController,
	}
}

func (s *syncController) Run(stopCh <-chan struct{}) {
	go s.nodeController.Run(stopCh)
	go s.serviceController.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, s.nodeController.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for node cache to sync"))
		return
	}

	if !cache.WaitForCacheSync(stopCh, s.serviceController.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for service cache to sync"))
		return
	}

	go wait.Until(s.SyncNodesTags, time.Minute*5, stopCh)
	go wait.Until(s.SyncLBTags, time.Minute*5, stopCh)
	<-stopCh
}

func (s *syncController) SyncNodesTags() {
	nodesList := s.nodeIndexer.List()
	for _, n := range nodesList {
		node, ok := n.(*v1.Node)
		if !ok {
			continue
		}
		if node.Spec.ProviderID == "" {
			klog.Warningf("provider ID is empty for node %s, ignoring", node.Name)
			continue
		}
		serverType, serverZone, serverID, err := ServerInfoFromProviderID(node.Spec.ProviderID)
		if err != nil {
			klog.Errorf("error getting server info from provider ID %s on node %s: %v", node.Spec.ProviderID, node.Name, err)
			continue
		}
		if serverType != InstanceTypeInstance {
			klog.Warningf("server type %s is not supported yet for node %s, ignoring", serverType, node.Name)
			continue
		}
		scwZone, err := scw.ParseZone(serverZone)
		if err != nil {
			klog.Errorf("error parsing provider ID zone %s for node %s: %v", serverZone, node.Name, err)
			continue
		}
		server, err := s.instanceAPI.GetServer(&instance.GetServerRequest{
			Zone:     scwZone,
			ServerID: serverID,
		})
		if err != nil {
			continue
		}

		nodeCopied := node.DeepCopy()
		if nodeCopied.ObjectMeta.Labels == nil {
			nodeCopied.ObjectMeta.Labels = map[string]string{}
		}

		patcher := NewNodePatcher(s.clientSet, nodeCopied)

		nodeLabels := map[string]string{}
		for _, tag := range server.Server.Tags {
			key, value := tagLabelParser(tag)
			if key == "" {
				continue
			}

			nodeLabels[key] = value
			nodeCopied.Labels[key] = value
		}

		for key := range node.Labels {
			if !strings.HasPrefix(key, labelsPrefix) {
				continue
			}

			if _, ok := nodeLabels[key]; !ok {
				// delete label
				delete(nodeCopied.Labels, key)
			}
		}

		err = patcher.Patch()
		if err != nil {
			klog.Errorf("error patching service: %v", err)
			continue
		}
	}
}

func (s *syncController) SyncLBTags() {
	servicesList := s.serviceIndexer.List()
	for _, svcitem := range servicesList {
		svc, ok := svcitem.(*v1.Service)
		if !ok {
			continue
		}
		// skip non LB services
		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			continue
		}

		region, loadBalancerID, err := getLoadBalancerID(svc)
		if err != nil || loadBalancerID == "" {
			klog.Warningf("service %s/%s does not have loadbalancer ID as annotation yet, skipping", svc.Namespace, svc.Name)
			continue
		}

		loadbalancer, err := s.lbAPI.GetLb(&lb.GetLbRequest{
			Region: region,
			LbID:   loadBalancerID,
		})
		if err != nil {
			klog.Errorf("error getting lb: %v", err)
			continue
		}

		// do not alter cache
		svcCopied := svc.DeepCopy()
		if svcCopied.ObjectMeta.Labels == nil {
			svcCopied.ObjectMeta.Labels = map[string]string{}
		}

		patcher := NewServicePatcher(s.clientSet, svcCopied)

		lbLabels := map[string]string{}
		for _, tag := range loadbalancer.Tags {
			key, value := tagLabelParser(tag)
			if key == "" {
				continue
			}

			lbLabels[key] = value
			svcCopied.Labels[key] = value
		}

		for key := range svc.Labels {
			if !strings.HasPrefix(key, labelsPrefix) {
				continue
			}

			if _, ok := lbLabels[key]; !ok {
				// delete label
				delete(svcCopied.Labels, key)
			}
		}

		err = patcher.Patch()
		if err != nil {
			klog.Errorf("error patching service: %v", err)
			continue
		}
	}
}

func tagLabelParser(tag string) (key string, value string) {
	split := splitRegexp.Split(tag, -1)

	key = labelsPrefix + labelsRegexp.FindString(split[0])
	if key == labelsPrefix || len(key) > 63 {
		return "", ""
	}

	if len(split) > 1 {
		value = labelsRegexp.FindString(split[1])
		if value == "" || len(value) > 63 {
			return "", ""
		}
	}

	return
}
