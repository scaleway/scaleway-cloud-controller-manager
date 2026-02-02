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
	"context"
	"fmt"

	scwinstance "github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	scwipam "github.com/scaleway/scaleway-sdk-go/api/ipam/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

type instances struct {
	api  InstanceAPI
	ipam IPAMAPI
	pnID string
}

type InstanceAPI interface {
	ListServers(req *scwinstance.ListServersRequest, opts ...scw.RequestOption) (*scwinstance.ListServersResponse, error)
	GetServer(req *scwinstance.GetServerRequest, opts ...scw.RequestOption) (*scwinstance.GetServerResponse, error)
}

type IPAMAPI interface {
	ListIPs(req *scwipam.ListIPsRequest, opts ...scw.RequestOption) (*scwipam.ListIPsResponse, error)
}

func newInstances(client *client, pnID string) *instances {
	return &instances{
		api:  scwinstance.NewAPI(client.scaleway),
		ipam: scwipam.NewAPI(client.scaleway),
		pnID: pnID,
	}
}

// NodeAddresses returns the addresses of the specified instance.
func (i *instances) NodeAddresses(ctx context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	server, err := i.getServerByName(string(name))
	if err != nil {
		return nil, err
	}
	return i.instanceAddresses(server)
}

// NodeAddressesByProviderID returns the addresses of the specified instance.
// The instance is specified using the providerID of the node.
func (i *instances) NodeAddressesByProviderID(ctx context.Context, providerID string) ([]v1.NodeAddress, error) {
	instanceServer, err := i.getServerByProviderID(providerID)
	if err != nil {
		return nil, err
	}
	return i.instanceAddresses(instanceServer)
}

// InstanceID returns the cloud provider ID of the node with the specified NodeName.
// Note that if the instance does not exist, we must return ("", cloudprovider.InstanceNotFound)
func (i *instances) InstanceID(ctx context.Context, name types.NodeName) (string, error) {
	instanceServer, err := i.getServerByName(string(name))
	if err != nil {
		return "", err
	}
	return BuildProviderID(InstanceTypeInstance, string(instanceServer.Zone), instanceServer.ID), nil
}

// InstanceType returns the type of the specified instance (ex. DEV1-M, GP1-XS,...).
func (i *instances) InstanceType(ctx context.Context, name types.NodeName) (string, error) {
	instanceServer, err := i.getServerByName(string(name))
	if err != nil {
		return "", err
	}
	return instanceServer.CommercialType, nil
}

// InstanceTypeByProviderID returns the type of the specified instance (ex. DEV1-M, GP1-XS,...).
func (i *instances) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	instanceServer, err := i.getServerByProviderID(providerID)
	if err != nil {
		return "", err
	}
	return instanceServer.CommercialType, nil
}

// return the machine's hostname
func (i *instances) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	return types.NodeName(hostname), nil
}

func (i *instances) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	_, err := i.getServerByProviderID(providerID)
	if err != nil {
		if err == cloudprovider.InstanceNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// InstanceShutdownByProviderID returns true if the instance is shutdown in cloudprovider
func (i *instances) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	instanceServer, err := i.getServerByProviderID(providerID)
	if err != nil {
		return false, err
	}

	if instanceServer.State == scwinstance.ServerStateRunning {
		return false, nil
	}

	return true, nil
}

// AddSSHKeyToAllInstances adds an SSH public key as a legal identity for all instances
// expected format for the key is standard ssh-keygen format: <protocol> <blob>
func (i *instances) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	return cloudprovider.NotImplemented
}

// GetZoneByProviderID returns the Zone containing the current zone and locality region of the node specified by providerID
// This method is particularly used in the context of external cloud providers where node initialization must be done
// outside the kubelets.
func (i *instances) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	instanceServer, err := i.getServerByProviderID(providerID)
	if err != nil {
		return cloudprovider.Zone{Region: "", FailureDomain: ""}, err
	}
	return instanceZone(instanceServer)

}

// GetZoneByNodeName returns the Zone containing the current zone and locality region of the node specified by node name
// This method is particularly used in the context of external cloud providers where node initialization must be done
// outside the kubelets.
func (i *instances) GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error) {
	instanceServer, err := i.getServerByName(string(nodeName))
	if err != nil {
		return cloudprovider.Zone{Region: "", FailureDomain: ""}, err
	}
	return instanceZone(instanceServer)
}

// ===========================

// instanceAddresses extracts NodeAdress from the server
func (i *instances) instanceAddresses(server *scwinstance.Server) ([]v1.NodeAddress, error) {
	addresses := []v1.NodeAddress{
		{Type: v1.NodeHostName, Address: server.Hostname},
	}

	// get the first dhcp provisionned ip if any (meaning not manually attached)
	for _, ip := range server.PublicIPs {
		if ip.Family == scwinstance.ServerIPIPFamilyInet && ip.ProvisioningMode == scwinstance.ServerIPProvisioningModeDHCP {
			addresses = append(
				addresses,
				v1.NodeAddress{Type: v1.NodeExternalIP, Address: ip.Address.String()},
				v1.NodeAddress{Type: v1.NodeExternalDNS, Address: fmt.Sprintf("%s.pub.instances.scw.cloud", server.ID)},
			)
			break
		}
	}

	// Try to get private network IPs
	privateAddresses, err := i.getPrivateNetworkAddresses(server)
	if err != nil {
		klog.Warningf("error getting private network addresses for node %s: %v", server.Name, err)
	}

	if len(privateAddresses) > 0 {
		addresses = append(addresses, privateAddresses...)
		return addresses, nil
	}

	// fallback to legacy private ip
	if server.PrivateIP != nil && *server.PrivateIP != "" {
		addresses = append(
			addresses,
			v1.NodeAddress{Type: v1.NodeInternalIP, Address: *server.PrivateIP},
			v1.NodeAddress{Type: v1.NodeInternalDNS, Address: fmt.Sprintf("%s.priv.instances.scw.cloud", server.ID)},
		)
	}

	return addresses, nil
}

// getPrivateNetworkAddresses returns private network IPs for the server.
// If pnID is configured, it only looks up IPs for that specific private network.
// If pnID is not configured, it iterates through all private NICs and returns all found IPs.
func (i *instances) getPrivateNetworkAddresses(server *scwinstance.Server) ([]v1.NodeAddress, error) {
	if len(server.PrivateNics) == 0 {
		return nil, nil
	}

	region, err := server.Zone.Region()
	if err != nil {
		return nil, fmt.Errorf("unable to get region from zone %s: %v", server.Zone, err)
	}

	var addresses []v1.NodeAddress

	// If a specific private network ID is configured, only look for that one
	if i.pnID != "" {
		for _, pNIC := range server.PrivateNics {
			if pNIC.PrivateNetworkID == i.pnID {
				nicAddresses, err := i.getIPsForPrivateNIC(server, pNIC, region)
				if err != nil {
					return nil, err
				}
				return nicAddresses, nil
			}
		}
		// Configured pnID not found in server's private NICs
		return nil, nil
	}

	// No specific pnID configured - iterate through all private NICs
	for _, pNIC := range server.PrivateNics {
		nicAddresses, err := i.getIPsForPrivateNIC(server, pNIC, region)
		if err != nil {
			klog.Warningf("error getting IPs for private NIC %s on node %s: %v", pNIC.ID, server.Name, err)
			continue
		}
		addresses = append(addresses, nicAddresses...)
	}

	return addresses, nil
}

// getIPsForPrivateNIC queries IPAM for IPs assigned to a specific private NIC
// It retries on transient 5xx errors with exponential backoff.
func (i *instances) getIPsForPrivateNIC(server *scwinstance.Server, pNIC *scwinstance.PrivateNIC, region scw.Region) ([]v1.NodeAddress, error) {
	var ips *scwipam.ListIPsResponse
	var err error

	ips, err = i.ipam.ListIPs(&scwipam.ListIPsRequest{
		ProjectID:    &server.Project,
		ResourceType: scwipam.ResourceTypeInstancePrivateNic,
		ResourceID:   &pNIC.ID,
		IsIPv6:       scw.BoolPtr(false),
		Region:       region,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to query ipam for node %s NIC %s: %v",
			server.Name, pNIC.ID, err)
	}

	var addresses []v1.NodeAddress
	for _, nicIP := range ips.IPs {
		addresses = append(
			addresses,
			v1.NodeAddress{Type: v1.NodeInternalIP, Address: nicIP.Address.IP.String()},
		)
	}

	return addresses, nil
}

func instanceZone(instanceServer *scwinstance.Server) (cloudprovider.Zone, error) {
	region, err := instanceServer.Zone.Region()
	if err != nil {
		return cloudprovider.Zone{}, err
	}
	return cloudprovider.Zone{
		Region:        string(region),
		FailureDomain: string(instanceServer.Zone),
	}, nil
}

// getServerByName returns a *instance.Server matching a name
// it must match the exact name
func (i *instances) getServerByName(name string) (*scwinstance.Server, error) {
	if name == "" {
		return nil, cloudprovider.InstanceNotFound
	}

	var instanceServer *scwinstance.Server
	allZones := (*scwinstance.API)(nil).Zones()
	for _, zoneReq := range allZones {
		resp, err := i.api.ListServers(&scwinstance.ListServersRequest{
			Zone: zoneReq,
			Name: &name,
		}, scw.WithAllPages())

		if err != nil {
			if is404Error(err) {
				continue
			}
			return nil, err
		}

		for _, srv := range resp.Servers {
			if srv.Name == name {
				if instanceServer != nil {
					klog.Errorf("more than one server matching the name %s", name)
					return nil, InstanceDuplicated
				}
				instanceServer = srv
			}
		}
	}

	if instanceServer == nil {
		return nil, cloudprovider.InstanceNotFound
	}

	// we've got exactly one server
	return instanceServer, nil

}

// getServerByProviderID returns a *instance,Server matching the given uuid and in the specified zone
// if the zone is empty, it will try all the zones
func (i *instances) getServerByProviderID(providerID string) (*scwinstance.Server, error) {
	_, zone, id, err := ServerInfoFromProviderID(providerID)
	if err != nil {
		return nil, err
	}

	resp, err := i.api.GetServer(&scwinstance.GetServerRequest{
		ServerID: id,
		Zone:     scw.Zone(zone),
	})
	if err != nil {
		if is404Error(err) {
			return nil, cloudprovider.InstanceNotFound
		}
		return nil, err
	}

	return resp.Server, nil
}

// InstanceV2

// InstanceExists returns true if the instance for the given node exists according to the cloud provider.
// Use the node.name or node.spec.providerID field to find the node in the cloud provider.
func (i *instances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	var err error

	if node.Spec.ProviderID == "" {
		_, err = i.getServerByName(node.Name)
	} else {
		_, err = i.getServerByProviderID(node.Spec.ProviderID)
	}

	if err == cloudprovider.InstanceNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// InstanceShutdown returns true if the instance is shutdown according to the cloud provider.
// Use the node.name or node.spec.providerID field to find the node in the cloud provider.
func (i *instances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	var instance *scwinstance.Server
	var err error

	if node.Spec.ProviderID == "" {
		instance, err = i.getServerByName(node.Name)
	} else {
		instance, err = i.getServerByProviderID(node.Spec.ProviderID)
	}

	if err != nil {
		return false, err
	}

	switch instance.State {
	case scwinstance.ServerStateRunning, scwinstance.ServerStateStarting:
		return false, nil
	default:
		return true, nil
	}
}

// InstanceMetadata returns the instance's metadata. The values returned in InstanceMetadata are
// translated into specific fields in the Node object on registration.
// Use the node.name or node.spec.providerID field to find the node in the cloud provider.
func (i *instances) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	var instance *scwinstance.Server
	var err error

	if node.Spec.ProviderID == "" {
		instance, err = i.getServerByName(node.Name)
	} else {
		instance, err = i.getServerByProviderID(node.Spec.ProviderID)
	}

	if err != nil {
		return nil, err
	}

	region, err := instance.Zone.Region()
	if err != nil {
		return nil, err
	}

	addresses, err := i.instanceAddresses(instance)
	if err != nil {
		return nil, err
	}

	return &cloudprovider.InstanceMetadata{
		ProviderID:    BuildProviderID(InstanceTypeInstance, instance.Zone.String(), instance.ID),
		InstanceType:  instance.CommercialType,
		NodeAddresses: addresses,
		Region:        region.String(),
		Zone:          instance.Zone.String(),
	}, nil
}
