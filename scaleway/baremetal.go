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

	scwbaremetal "github.com/scaleway/scaleway-sdk-go/api/baremetal/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

type baremetal struct {
	api BaremetalAPI
}

type BaremetalAPI interface {
	ListServers(req *scwbaremetal.ListServersRequest, opts ...scw.RequestOption) (*scwbaremetal.ListServersResponse, error)
	GetServer(req *scwbaremetal.GetServerRequest, opts ...scw.RequestOption) (*scwbaremetal.Server, error)
	GetOffer(req *scwbaremetal.GetOfferRequest, opts ...scw.RequestOption) (*scwbaremetal.Offer, error)
}

func newBaremetal(client *client) *baremetal {
	return &baremetal{
		api: scwbaremetal.NewAPI(client.scaleway),
	}
}

// NodeAddresses returns the addresses of the specified instance.
func (b *baremetal) NodeAddresses(ctx context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	baremetalServer, err := b.getServerByName(string(name))
	if err != nil {
		return nil, err
	}
	return baremetalAddresses(baremetalServer), nil
}

// NodeAddressesByProviderID returns the addresses of the specified instance.
// The instance is specified using the providerID of the node.
func (b *baremetal) NodeAddressesByProviderID(ctx context.Context, providerID string) ([]v1.NodeAddress, error) {
	baremetalServer, err := b.getServerByProviderID(providerID)
	if err != nil {
		return nil, err
	}
	return baremetalAddresses(baremetalServer), nil
}

// InstanceID returns the cloud provider ID of the node with the specified NodeName.
// Note that if the instance does not exist, we must return ("", cloudprovider.InstanceNotFound)
func (b *baremetal) InstanceID(ctx context.Context, nodeName types.NodeName) (string, error) {
	baremetalServer, err := b.getServerByName(string(nodeName))
	if err != nil {
		return "", err
	}
	return BuildProviderID(InstanceTypeBaremtal, string(baremetalServer.Zone), baremetalServer.ID), nil
}

// InstanceType returns the type of the specified instance.
func (b *baremetal) InstanceType(ctx context.Context, name types.NodeName) (string, error) {
	baremetalServer, err := b.getServerByName(string(name))
	if err != nil {
		return "", err
	}
	return b.getServerOfferName(baremetalServer)
}

// InstanceTypeByProviderID returns the type of the specified instance (ex. GP-BM1-M, HC-BM1-S,...).
func (b *baremetal) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	baremetalServer, err := b.getServerByProviderID(providerID)
	if err != nil {
		return "", err
	}
	return b.getServerOfferName(baremetalServer)
}

// AddSSHKeyToAllInstances adds an SSH public key as a legal identity for all instances
// expected format for the key is standard ssh-keygen format: <protocol> <blob>
func (b *baremetal) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	return cloudprovider.NotImplemented
}

// CurrentNodeName returns the name of the node we are currently running on
// On most clouds (e.g. GCE) this is the hostname, so we provide the hostname
func (b *baremetal) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	return types.NodeName(hostname), nil
}

// InstanceExistsByProviderID returns true if the instance for the given provider exists.
// If false is returned with no error, the instance will be immediately deleted by the cloud controller manager.
// This method should still return true for instances that exist but are stopped/sleeping.
func (b *baremetal) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	_, err := b.getServerByProviderID(providerID)
	if err != nil {
		if err == cloudprovider.InstanceNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// InstanceShutdownByProviderID returns true if the instance is shutdown in cloudprovider
func (b *baremetal) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	baremetalServer, err := b.getServerByProviderID(providerID)
	if err != nil {
		return false, err
	}

	if baremetalServer.Status == scwbaremetal.ServerStatusReady {
		return false, nil
	}

	return true, nil
}

// GetZoneByProviderID returns the Zone containing the current zone and locality region of the node specified by providerID
// This method is particularly used in the context of external cloud providers where node initialization must be done
// outside the kubelets.
func (b *baremetal) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	baremetalServer, err := b.getServerByProviderID(providerID)
	if err != nil {
		return cloudprovider.Zone{Region: "", FailureDomain: ""}, err
	}
	return baremetalZone(baremetalServer)
}

// GetZoneByNodeName returns the Zone containing the current zone and locality region of the node specified by node name
// This method is particularly used in the context of external cloud providers where node initialization must be done
// outside the kubelets.
func (b *baremetal) GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error) {
	baremetalServer, err := b.getServerByName(string(nodeName))
	if err != nil {
		return cloudprovider.Zone{Region: "", FailureDomain: ""}, err
	}
	return baremetalZone(baremetalServer)
}

// ===========================

// baremetalAddresses extracts NodeAdress from the server
func baremetalAddresses(server *scwbaremetal.Server) []v1.NodeAddress {
	addresses := []v1.NodeAddress{
		{Type: v1.NodeHostName, Address: server.Name},
	}
	for _, ip := range server.IPs {
		if ip.Version == scwbaremetal.IPVersionIPv4 {
			addresses = append(
				addresses,
				v1.NodeAddress{Type: v1.NodeExternalIP, Address: ip.Address.String()},
				v1.NodeAddress{Type: v1.NodeExternalDNS, Address: server.Domain},
			)
			break
		}
	}

	return addresses
}

// baremetalZone extract zone from the server
func baremetalZone(server *scwbaremetal.Server) (cloudprovider.Zone, error) {
	zone := cloudprovider.Zone{
		Region:        string(server.Zone),
		FailureDomain: "unavailable",
	}

	return zone, nil
}

// getServerOfferName returns the offer name of a baremetal server
func (b *baremetal) getServerOfferName(server *scwbaremetal.Server) (string, error) {
	offer, err := b.api.GetOffer(&scwbaremetal.GetOfferRequest{
		OfferID: server.OfferID,
		Zone:    server.Zone,
	})
	if err != nil {
		if is404Error(err) {
			return "UNKNOWN", nil
		}
		return "", err
	}

	return offer.Name, nil
}

// getServerByName returns a *instance.Server matching a name
// it must match the exact name
func (b *baremetal) getServerByName(name string) (*scwbaremetal.Server, error) {
	if name == "" {
		return nil, cloudprovider.InstanceNotFound
	}

	var server *scwbaremetal.Server
	for _, zoneReq := range scw.AllZones {
		resp, err := b.api.ListServers(&scwbaremetal.ListServersRequest{
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
				if server != nil {
					klog.Errorf("more than one server matching the name %s", name)
					return nil, InstanceDuplicated
				}
				server = srv
			}
		}
	}

	if server == nil {
		return nil, cloudprovider.InstanceNotFound
	}

	// we've got exactly one server
	return server, nil
}

// getServerByProviderID returns a *instance,Server matchig the given uuid and in the specified zone
// if the zone is empty, it will try all the zones
func (b *baremetal) getServerByProviderID(providerID string) (*scwbaremetal.Server, error) {
	_, zone, id, err := ServerInfoFromProviderID(providerID)
	if err != nil {
		return nil, err
	}

	server, err := b.api.GetServer(&scwbaremetal.GetServerRequest{
		ServerID: id,
		Zone:     scw.Zone(zone),
	})
	if err != nil {
		if is404Error(err) {
			return nil, cloudprovider.InstanceNotFound
		}
		return nil, err
	}
	return server, nil
}

// InstanceV2

// InstanceExists returns true if the instance for the given node exists according to the cloud provider.
// Use the node.name or node.spec.providerID field to find the node in the cloud provider.
func (b *baremetal) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	var err error

	if node.Spec.ProviderID == "" {
		_, err = b.getServerByName(node.Name)
	} else {
		_, err = b.getServerByProviderID(node.Spec.ProviderID)
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
func (b *baremetal) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	var bm *scwbaremetal.Server
	var err error

	if node.Spec.ProviderID == "" {
		bm, err = b.getServerByName(node.Name)
	} else {
		bm, err = b.getServerByProviderID(node.Spec.ProviderID)
	}

	if err != nil {
		return false, err
	}

	switch bm.Status {
	case scwbaremetal.ServerStatusReady, scwbaremetal.ServerStatusStarting:
		return false, nil
	default:
		return true, nil
	}
}

// InstanceMetadata returns the instance's metadata. The values returned in InstanceMetadata are
// translated into specific fields in the Node object on registration.
// Use the node.name or node.spec.providerID field to find the node in the cloud provider.
func (b *baremetal) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	var bm *scwbaremetal.Server
	var err error

	if node.Spec.ProviderID == "" {
		bm, err = b.getServerByName(node.Name)
	} else {
		bm, err = b.getServerByProviderID(node.Spec.ProviderID)
	}

	if err != nil {
		return nil, err
	}

	offerName, err := b.getServerOfferName(bm)
	if err != nil {
		return nil, err
	}

	region, err := bm.Zone.Region()
	if err != nil {
		return nil, err
	}

	return &cloudprovider.InstanceMetadata{
		ProviderID:    BuildProviderID(InstanceTypeBaremtal, bm.Zone.String(), bm.ID),
		InstanceType:  offerName,
		NodeAddresses: baremetalAddresses(bm),
		Region:        region.String(),
		Zone:          bm.Zone.String(),
	}, nil
}
