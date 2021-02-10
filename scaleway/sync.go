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
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
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
	queue             workqueue.RateLimitingInterface
}

const (
	exponentialBaseDelay  = time.Second * 1
	exponentialMaxDelay   = time.Minute * 10
	exponentialMaxRetries = 30
)

// from k8s.io/apimachinery/pkg/util/validation/validation.go
const qnameCharFmt string = "[A-Za-z0-9]"
const qnameExtCharFmt string = "[-A-Za-z0-9_.]"
const qualifiedNameFmt string = "(" + qnameCharFmt + qnameExtCharFmt + "*)?" + qnameCharFmt
const qualifiedNameErrMsg string = "must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character"
const qualifiedNameMaxLength int = 63

var (
	splitRegexp         = regexp.MustCompile(`[:= ]+`)
	qualifiedNameRegexp = regexp.MustCompile("^" + qualifiedNameFmt + "$")
	labelsPrefix        = "k8s.scaleway.com/"
	taintsPrefix        = "k8s.scaleway.com/"
	labelTaintPrefix    = "taint="
	labelNoPrefix       = "noprefix="

	// K8S labels
	labelNodeRoleExcludeBalancer      = "node.kubernetes.io/exclude-from-external-load-balancers"
	labelNodeRoleExcludeBalancerValue = "managed-by-scaleway-ccm"
)

func newSyncController(client *client, clientset *kubernetes.Clientset, cacheUpdateFrequency time.Duration) *syncController {
	nodeListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "nodes", v1.NamespaceAll, fields.Everything())
	serviceListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "services", v1.NamespaceAll, fields.Everything())

	syncQueue := workqueue.NewRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(exponentialBaseDelay, exponentialMaxDelay))

	nodeIndexer, nodeController := cache.NewIndexerInformer(nodeListWatcher, &v1.Node{}, cacheUpdateFrequency, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			node, ok := obj.(*v1.Node)
			if ok {
				syncQueue.Add(node.Name)
			}
		},
	}, cache.Indexers{})
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
		queue:             syncQueue,
	}
}

func (s *syncController) Run(stopCh <-chan struct{}) {
	go s.nodeController.Run(stopCh)
	go s.serviceController.Run(stopCh)
	defer s.queue.ShutDown()

	if !cache.WaitForCacheSync(stopCh, s.nodeController.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for node cache to sync"))
		return
	}

	if !cache.WaitForCacheSync(stopCh, s.serviceController.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for service cache to sync"))
		return
	}

	go wait.Until(s.runSyncQueue, time.Second, stopCh)
	go wait.Until(s.SyncNodesTags, time.Minute*5, stopCh)
	go wait.Until(s.SyncLBTags, time.Minute*5, stopCh)
	<-stopCh
}

func (s *syncController) runSyncQueue() {
	for s.processNextActionItem() {
	}
}

func (s *syncController) processNextActionItem() bool {
	key, quit := s.queue.Get()
	if quit {
		return false
	}

	defer s.queue.Done(key)

	err := s.handleAction(key.(string))
	if err != nil {
		klog.Errorf("error syncing node: %v", err)
		if s.queue.NumRequeues(key) < exponentialMaxRetries {
			s.queue.AddRateLimited(key)
			return true
		}
	}
	s.queue.Forget(key)
	return true
}

func (s *syncController) handleAction(nodeName string) error {
	var err error
	obj, exists, err := s.nodeIndexer.GetByKey(nodeName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	node, ok := obj.(*v1.Node)
	if !ok {
		err = fmt.Errorf("could not cast node to v1.Node")
	}
	return s.syncNodeTags(node)
}

func (s *syncController) syncNodeTags(node *v1.Node) error {
	if node.Spec.ProviderID == "" {
		klog.Warningf("provider ID is empty for node %s, ignoring", node.Name)
		return nil
	}
	serverType, serverZone, serverID, err := ServerInfoFromProviderID(node.Spec.ProviderID)
	if err != nil {
		klog.Errorf("error getting server info from provider ID %s on node %s: %v", node.Spec.ProviderID, node.Name, err)
		return fmt.Errorf("error getting server info from provider ID %s on node %s: %v", node.Spec.ProviderID, node.Name, err)
	}
	if serverType != InstanceTypeInstance {
		klog.Warningf("server type %s is not supported yet for node %s, ignoring", serverType, node.Name)
		return nil
	}
	scwZone, err := scw.ParseZone(serverZone)
	if err != nil {
		klog.Errorf("error parsing provider ID zone %s for node %s: %v", serverZone, node.Name, err)
		return fmt.Errorf("error parsing provider ID zone %s for node %s: %v", serverZone, node.Name, err)
	}
	server, err := s.instanceAPI.GetServer(&instance.GetServerRequest{
		Zone:     scwZone,
		ServerID: serverID,
	})
	if err != nil {
		return err
	}

	nodeCopied := node.DeepCopy()
	if nodeCopied.ObjectMeta.Labels == nil {
		nodeCopied.ObjectMeta.Labels = map[string]string{}
	}

	patcher := NewNodePatcher(s.clientSet, nodeCopied)

	nodeLabels := map[string]string{}
	nodeTaints := []v1.Taint{}
	for _, tag := range server.Server.Tags {
		if strings.HasPrefix(tag, labelTaintPrefix) {
			key, value, effect := tagTaintParser(tag)
			if key == "" {
				continue
			}
			nodeTaints = append(nodeTaints, v1.Taint{
				Key:    key,
				Value:  value,
				Effect: effect,
			})
		} else {
			var key string
			var value string
			switch tag {
			case labelNodeRoleExcludeBalancer:
				key = labelNodeRoleExcludeBalancer
				value = labelNodeRoleExcludeBalancerValue
			default:
				key, value = tagLabelParser(tag)
				if key == "" {
					continue
				}
			}

			nodeLabels[key] = value
			nodeCopied.Labels[key] = value
		}
	}

	for key, value := range node.Labels {
		switch key {
		case labelNodeRoleExcludeBalancer:
			if value != labelNodeRoleExcludeBalancerValue {
				continue
			}
		default:
			if !strings.HasPrefix(key, labelsPrefix) {
				continue
			}
		}

		if _, ok := nodeLabels[key]; !ok {
			// delete label
			delete(nodeCopied.Labels, key)
		}
	}

	for _, taint := range node.Spec.Taints {
		if !strings.HasPrefix(taint.Key, taintsPrefix) {
			nodeTaints = append(nodeTaints, taint)
		}
	}

	nodeCopied.Spec.Taints = nodeTaints
	err = patcher.Patch()
	if err != nil {
		klog.Errorf("error patching service: %v", err)
		return err
	}
	return nil
}

func (s *syncController) SyncNodesTags() {
	nodesList := s.nodeIndexer.List()
	for _, n := range nodesList {
		node, ok := n.(*v1.Node)
		if !ok {
			continue
		}
		_ = s.syncNodeTags(node)
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

		loadbalancer, err := s.lbAPI.GetLB(&lb.GetLBRequest{
			Region: region,
			LBID:   loadBalancerID,
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
	prefix := labelsPrefix
	tagValue := tag

	if strings.HasPrefix(tag, labelNoPrefix) {
		prefix = ""
		tagValue = strings.TrimPrefix(tag, labelNoPrefix)
		tagSplit := strings.Split(tagValue, "/")

		if len(tagSplit) > 2 {
			klog.Errorf("tag %s is not valid: %s contains more than 1 '/'", tag, tagValue)
			return "", ""
		}
		if len(tagSplit) == 2 {
			if tagSplit[0] == "" {
				klog.Errorf("tag %s is not valid: prefix is empty")
				return "", ""
			}
			if errs := validation.IsDNS1123Subdomain(tagSplit[0]); len(errs) != 0 {
				klog.Errorf("tag %s is not valid: prefix is not composed of DNS labels: %s", tag, strings.Join(errs, ","))
				return "", ""
			}
			prefix = tagSplit[0] + "/"
			tagValue = tagSplit[1]
		}
	}

	split := splitRegexp.Split(tagValue, -1)

	if len(split[0]) == 0 {
		klog.Errorf("tag %s have an empty key", tag)
		return "", ""
	}
	if len(split[0]) > qualifiedNameMaxLength {
		klog.Errorf("tag %s have a key too long, got %d instead of max %d", tag, len(split[0]), qualifiedNameMaxLength)
		return "", ""
	}
	if !qualifiedNameRegexp.MatchString(split[0]) {
		klog.Errorf("tag %s key must consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character", tag)
		return "", ""
	}
	key = prefix + split[0]
	if len(split) > 1 {
		value = split[1]
		if errs := validation.IsValidLabelValue(value); len(errs) != 0 {
			klog.Errorf("tag %s value must be an empty string or consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character", tag)
			return "", ""
		}
	}

	return
}

func tagTaintParser(tag string) (key string, value string, effect v1.TaintEffect) {
	taint := strings.TrimPrefix(tag, labelTaintPrefix)
	taintParts := strings.Split(taint, ":")

	switch v1.TaintEffect(taintParts[len(taintParts)-1]) {
	case v1.TaintEffectNoExecute:
		effect = v1.TaintEffectNoExecute
	case v1.TaintEffectNoSchedule:
		effect = v1.TaintEffectNoSchedule
	case v1.TaintEffectPreferNoSchedule:
		effect = v1.TaintEffectPreferNoSchedule
	default:
		return
	}

	key, value = tagLabelParser(strings.Join(taintParts[:len(taintParts)-1], ""))
	return
}
