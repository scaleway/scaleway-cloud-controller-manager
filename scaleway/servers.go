package scaleway

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

const (
	InstanceTypeInstance = "instance"
	InstanceTypeBaremtal = "baremetal"

	// nodeLabelDisableLifeCycle is a label for nulling cloudprovider.InstancesV2 interface
	nodeLabelDisableLifeCycle = "k8s.scw.cloud/disable-lifcycle"

	// nodeAnnotationNodePublicIP is an annotation to override External-IP of a node
	nodeLabelNodePublicIP = "k8s.scw.cloud/node-public-ip"
)

type Servers interface {
	cloudprovider.Instances
	cloudprovider.InstancesV2
	GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error)
	GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error)
}

type servers struct {
	instances Servers
	baremetal Servers
}

func newServers(client *client) Servers {
	return &servers{
		instances: newInstances(client),
		baremetal: newBaremetal(client),
	}
}

var (
	regexpProduct      = "product"
	regexpLocalization = "localization"
	regexpUUID         = "uuid"
	providerIDRegexp   = regexp.MustCompile(fmt.Sprintf("%s://((?P<%s>.*?)/(?P<%s>.*?)/(?P<%s>.*)|(?P<%s>.*))", ProviderName, regexpProduct, regexpLocalization, regexpUUID, regexpUUID))
)

// ServerInfoFromProviderID extract the product type, zone and uuid from the providerID
// the providerID looks like scaleway://12345 or scaleway://instance/fr-par-1/12345
func ServerInfoFromProviderID(providerID string) (string, string, string, error) {
	if !providerIDRegexp.MatchString(providerID) {
		return "", "", "", BadProviderID
	}

	match := providerIDRegexp.FindStringSubmatch(providerID)

	result := make(map[string]string)
	for i, name := range providerIDRegexp.SubexpNames() {
		if i != 0 && name != "" {
			if match[i] != "" {
				result[name] = match[i]
			}
		}
	}

	if _, ok := result[regexpProduct]; !ok {
		result[regexpProduct] = InstanceTypeInstance
	}

	return result[regexpProduct], result[regexpLocalization], result[regexpUUID], nil
}

// BuildProviderID build the providerID from given informations
func BuildProviderID(product, localization, id string) string {
	return fmt.Sprintf("%s://%s/%s/%s", ProviderName, product, localization, id)
}

// getImplementationByProviderID returns the corresponding cloudprovider.Instances implementation
// At scaleway, the new baremetal offer is not integrated with instance API.
func (s *servers) getImplementationByProviderID(providerID string) Servers {
	if strings.HasPrefix(providerID, ProviderName+"://"+InstanceTypeBaremtal) {
		return s.baremetal
	}
	return s.instances
}

// NodeAddresses returns the addresses of the specified instance.
func (s *servers) NodeAddresses(ctx context.Context, name types.NodeName) ([]v1.NodeAddress, error) {
	addrs, err := s.instances.NodeAddresses(ctx, name)
	if err == cloudprovider.InstanceNotFound {
		return s.baremetal.NodeAddresses(ctx, name)
	}
	return addrs, err
}

// NodeAddressesByProviderID returns the addresses of the specified instance.
// The instance is specified using the providerID of the node. The
// ProviderID is a unique identifier of the node. This will not be called
// from the node whose nodeaddresses are being queried. i.e. local metadata
// services cannot be used in this method to obtain nodeaddresses
func (s *servers) NodeAddressesByProviderID(ctx context.Context, providerID string) ([]v1.NodeAddress, error) {
	return s.getImplementationByProviderID(providerID).NodeAddressesByProviderID(ctx, providerID)
}

// InstanceID returns the cloud provider ID of the node with the specified NodeName.
// Note that if the instance does not exist, we must return ("", cloudprovider.InstanceNotFound)
// cloudprovider.InstanceNotFound should NOT be returned for instances that exist but are stopped/sleeping
func (s *servers) InstanceID(ctx context.Context, nodeName types.NodeName) (string, error) {
	addrs, err := s.instances.InstanceID(ctx, nodeName)
	if err == cloudprovider.InstanceNotFound {
		return s.baremetal.InstanceID(ctx, nodeName)
	}
	return addrs, err
}

// InstanceType returns the type of the specified instance.
func (s *servers) InstanceType(ctx context.Context, nodeName types.NodeName) (string, error) {
	addrs, err := s.instances.InstanceType(ctx, nodeName)
	if err == cloudprovider.InstanceNotFound {
		return s.baremetal.InstanceType(ctx, nodeName)
	}
	return addrs, err
}

// InstanceTypeByProviderID returns the type of the specified instance.
func (s *servers) InstanceTypeByProviderID(ctx context.Context, providerID string) (string, error) {
	return s.getImplementationByProviderID(providerID).InstanceTypeByProviderID(ctx, providerID)
}

// AddSSHKeyToAllInstances adds an SSH public key as a legal identity for all instances
// expected format for the key is standard ssh-keygen format: <protocol> <blob>
func (s *servers) AddSSHKeyToAllInstances(ctx context.Context, user string, keyData []byte) error {
	klog.Errorf("addsshkeytoallinstances is not implemented")
	return cloudprovider.NotImplemented
}

// CurrentNodeName returns the name of the node we are currently running on
// On most clouds (e.g. GCE) this is the hostname, so we provide the hostname
func (s *servers) CurrentNodeName(ctx context.Context, hostname string) (types.NodeName, error) {
	return types.NodeName(hostname), nil
}

// InstanceExistsByProviderID returns true if the instance for the given provider exists.
// If false is returned with no error, the instance will be immediately deleted by the cloud controller manager.
// This method should still return true for instances that exist but are stopped/sleeping.
func (s *servers) InstanceExistsByProviderID(ctx context.Context, providerID string) (bool, error) {
	return s.getImplementationByProviderID(providerID).InstanceExistsByProviderID(ctx, providerID)
}

// InstanceShutdownByProviderID returns true if the instance is shutdown in cloudprovider
func (s *servers) InstanceShutdownByProviderID(ctx context.Context, providerID string) (bool, error) {
	return s.getImplementationByProviderID(providerID).InstanceShutdownByProviderID(ctx, providerID)
}

func (s *servers) GetZoneByProviderID(ctx context.Context, providerID string) (cloudprovider.Zone, error) {
	return s.getImplementationByProviderID(providerID).GetZoneByProviderID(ctx, providerID)
}

func (s *servers) GetZoneByNodeName(ctx context.Context, nodeName types.NodeName) (cloudprovider.Zone, error) {
	zone, err := s.instances.GetZoneByNodeName(ctx, nodeName)
	if err == cloudprovider.InstanceNotFound {
		return s.baremetal.GetZoneByNodeName(ctx, nodeName)
	}
	return zone, err
}

// Instances V2

// InstanceExists returns true if the instance for the given node exists according to the cloud provider.
// Use the node.name or node.spec.providerID field to find the node in the cloud provider.
func (s *servers) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	if _, ok := node.Annotations[nodeLabelDisableLifeCycle]; ok {
		return true, nil
	}

	if node.Spec.ProviderID == "" {
		exists, err := s.instances.InstanceExists(ctx, node)
		if err != nil {
			return false, err
		}
		if !exists {
			return s.baremetal.InstanceExists(ctx, node)
		}
		return exists, nil
	}
	return s.getImplementationByProviderID(node.Spec.ProviderID).InstanceExists(ctx, node)
}

// InstanceShutdown returns true if the instance is shutdown according to the cloud provider.
// Use the node.name or node.spec.providerID field to find the node in the cloud provider.
func (s *servers) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	if _, ok := node.Annotations[nodeLabelDisableLifeCycle]; ok {
		return false, nil
	}

	if node.Spec.ProviderID == "" {
		shutdown, err := s.instances.InstanceShutdown(ctx, node)
		if err == cloudprovider.InstanceNotFound {
			return s.baremetal.InstanceShutdown(ctx, node)
		}
		return shutdown, err
	}
	return s.getImplementationByProviderID(node.Spec.ProviderID).InstanceShutdown(ctx, node)
}

// InstanceMetadata returns the instance's metadata. The values returned in InstanceMetadata are
// translated into specific fields in the Node object on registration.
// Use the node.name or node.spec.providerID field to find the node in the cloud provider.
func (s *servers) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	if address, ok := node.Annotations[nodeLabelNodePublicIP]; ok {
		return &cloudprovider.InstanceMetadata{
			NodeAddresses: []v1.NodeAddress{{
				Type:    v1.NodeExternalIP,
				Address: address,
			}},
		}, nil
	}

	if node.Spec.ProviderID == "" {
		metadata, err := s.instances.InstanceMetadata(ctx, node)
		if err == cloudprovider.InstanceNotFound {
			return s.baremetal.InstanceMetadata(ctx, node)
		}
		return metadata, err
	}
	return s.getImplementationByProviderID(node.Spec.ProviderID).InstanceMetadata(ctx, node)
}
