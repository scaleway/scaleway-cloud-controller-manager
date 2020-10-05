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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/scaleway/scaleway-sdk-go/logger"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"k8s.io/client-go/kubernetes"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
)

const (
	// define the provider
	providerName         = "scaleway"
	cacheUpdateFrequency = time.Minute * 10

	// optional fields
	scwCcmPrefixEnv        = "SCW_CCM_PREFIX"
	scwCcmTagsEnv          = "SCW_CCM_TAGS"
	scwCcmTagsDelimiterEnv = "SCW_CCM_TAGS_DELIMITER"

	// extraUserAgentEnv is the environment variable that adds some string at the end of the user agent
	extraUserAgentEnv = "EXTRA_USER_AGENT"
	// disableInterfacesEnv is the environment variable used to disable some cloud interfaces
	disableInterfacesEnv      = "DISABLE_INTERFACES"
	instancesInterfaceName    = "instances"
	loadBalancerInterfaceName = "loadbalancer"
	zonesInterfaceName        = "zones"

	// loadBalancerDefaultTypeEnv is the environment to choose the default LB type
	loadBalancerDefaultTypeEnv = "LB_DEFAULT_TYPE"
)

type cloud struct {
	client         *client
	instances      cloudprovider.Instances
	instancesV2    cloudprovider.InstancesV2
	zones          cloudprovider.Zones
	loadbalancers  cloudprovider.LoadBalancer
	syncController *syncController
}

func newCloud(config io.Reader) (cloudprovider.Interface, error) {
	logger.SetLogger(logging)

	userAgent := fmt.Sprintf("scaleway/ccm %s (%s)", ccmVersion, gitCommit)
	if extraUA := os.Getenv(extraUserAgentEnv); extraUA != "" {
		userAgent = userAgent + " " + extraUA
	}

	// Create a Scaleway client
	// use theses env variable to set or overwrite profile values
	// SCW_ACCESS_KEY
	// SCW_SECRET_KEY
	// SCW_DEFAULT_ORGANIZATION_ID
	// SCW_DEFAULT_REGION
	// SCW_DEFAULT_ZONE

	scwClient, err := scw.NewClient(
		scw.WithUserAgent(userAgent),
		scw.WithEnv(),
	)
	if err != nil {
		klog.Errorf("error creating scaleway client api: %v", err)
		return nil, err
	}

	if _, set := scwClient.GetDefaultRegion(); !set {
		return nil, errors.New("region is required")
	}

	client := newClient(scwClient)

	instancesInterface := newServers(client)
	loadbalancerInterface := newLoadbalancers(client, os.Getenv(loadBalancerDefaultTypeEnv))
	zonesInterface := newZones(client)

	for _, disableInterface := range strings.Split(os.Getenv(disableInterfacesEnv), ",") {
		switch strings.ToLower(disableInterface) {
		case instancesInterfaceName:
			instancesInterface = nil
		case loadBalancerInterfaceName:
			loadbalancerInterface = nil
		case zonesInterfaceName:
			zonesInterface = nil
		}
	}

	return &cloud{
		client:        client,
		instances:     instancesInterface,
		instancesV2:   instancesInterface,
		zones:         zonesInterface,
		loadbalancers: loadbalancerInterface,
	}, nil
}

func init() {
	cloudprovider.RegisterCloudProvider(providerName, func(config io.Reader) (cloudprovider.Interface, error) {
		return newCloud(config)
	})
}

func (c *cloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	restConfig := clientBuilder.ConfigOrDie("cloud-controller-manager")
	c.client.kubernetes = kubernetes.NewForConfigOrDie(restConfig)

	klog.Infof("clientset initialized")

	c.syncController = newSyncController(c.client, c.client.kubernetes, cacheUpdateFrequency)

	go c.syncController.Run(stop)
}

func (c *cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return c.loadbalancers, true
}

func (c *cloud) Instances() (cloudprovider.Instances, bool) {
	return c.instances, true
}

func (c *cloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return c.instancesV2, true
}

func (c *cloud) Zones() (cloudprovider.Zones, bool) {
	return c.zones, true
}

// clusters is not implemented
func (c *cloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

// routes is not implemented
func (c *cloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

func (c *cloud) ProviderName() string {
	return providerName
}

// has cluster id is not implemented
func (c *cloud) HasClusterID() bool {
	return false
}
