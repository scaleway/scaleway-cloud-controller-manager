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
	"net"
	"strings"
	"testing"

	scwinstance "github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	scwipam "github.com/scaleway/scaleway-sdk-go/api/ipam/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cloudprovider "k8s.io/cloud-provider"
)

type fakeInstanceAPI struct {
	Servers map[string]*scwinstance.Server
}

func newFakeInstanceAPI() *fakeInstanceAPI {
	return &fakeInstanceAPI{
		Servers: map[string]*scwinstance.Server{
			// a normal server with tags
			"232bf860-9ffe-4b08-97da-158c0316ed15": {
				Zone:           "fr-par-1",
				Name:           "scw-nervous-mccarthy",
				Tags:           []string{"instance", "k8s", "test=works"},
				CommercialType: "GP1-XS",
				State:          scwinstance.ServerStateRunning,
				PrivateIP:      scw.StringPtr("10.14.0.1"),
				PublicIP: &scwinstance.ServerIP{
					Address: net.ParseIP("62.210.16.2"),
					Dynamic: false,
				},
				PublicIPs: []*scwinstance.ServerIP{
					{
						Address:          net.ParseIP("62.210.16.2"),
						ProvisioningMode: scwinstance.ServerIPProvisioningModeDHCP,
						Family:           scwinstance.ServerIPIPFamilyInet,
					},
					{
						Address:          net.ParseIP("ff::"),
						ProvisioningMode: scwinstance.ServerIPProvisioningModeDHCP,
						Family:           scwinstance.ServerIPIPFamilyInet6,
					},
				},
				EnableIPv6: scw.BoolPtr(true),
				IPv6: &scwinstance.ServerIPv6{
					Address: net.ParseIP("2001:bc8:4::1"),
					Gateway: net.ParseIP("2001:bc8:4::"),
					Netmask: "64",
				},
			},
			// a stopped in place server
			"428ed385-4508-4a19-837b-a7c3653cdda2": {
				Zone:           "fr-par-1",
				Name:           "scw-sleepy-visvesvaraya",
				Tags:           []string{"stopped"},
				CommercialType: "GP1-XS",
				State:          scwinstance.ServerStateStoppedInPlace,
				PrivateIP:      scw.StringPtr("10.14.0.1"),
				PublicIP: &scwinstance.ServerIP{
					Address: net.ParseIP("62.210.16.2"),
					Dynamic: false,
				},
				PublicIPs: []*scwinstance.ServerIP{
					{
						Address:          net.ParseIP("ff::"),
						ProvisioningMode: scwinstance.ServerIPProvisioningModeDHCP,
						Family:           scwinstance.ServerIPIPFamilyInet6,
					},
					{
						Address:          net.ParseIP("62.210.16.2"),
						ProvisioningMode: scwinstance.ServerIPProvisioningModeDHCP,
						Family:           scwinstance.ServerIPIPFamilyInet,
					},
				},
				EnableIPv6: scw.BoolPtr(true),
				IPv6: &scwinstance.ServerIPv6{
					Address: net.ParseIP("2001:bc8:4::1"),
					Gateway: net.ParseIP("2001:bc8:4::"),
					Netmask: "64",
				},
			},
			// a server without flexible ip
			"a7ab9055-5fe2-4009-afd0-7787399e6755": {
				Zone:           "fr-par-1",
				Name:           "scw-charming-feistel",
				CommercialType: "GP1-XS",
				State:          scwinstance.ServerStateRunning,
				PrivateIP:      scw.StringPtr("10.14.0.1"),
				PublicIP:       nil,
				EnableIPv6:     scw.BoolPtr(true),
				IPv6: &scwinstance.ServerIPv6{
					Address: net.ParseIP("2001:bc8:4::1"),
					Gateway: net.ParseIP("2001:bc8:4::"),
					Netmask: "64",
				},
			},
			// a server in a transiant state (stopping)
			"23ffb180-80cb-489d-9eba-f2b6ebe44d32": {
				Zone:           "fr-par-1",
				Name:           "scw-laughing-blackburn",
				CommercialType: "GP1-XS",
				State:          scwinstance.ServerStateStopping,
			},
			// a stopped server
			"f082dc51-bffa-4cb2-b0ac-e8b1ddeefaf0": {
				Zone:           "fr-par-1",
				Name:           "scw-gifted-bose",
				CommercialType: "GP1-XS",
				State:          scwinstance.ServerStateStopped,
			},
			// a starting server
			"8472b253-ea1f-464a-a49b-086b965467ff": {
				Zone:           "fr-par-1",
				Name:           "scw-funny-gallois",
				CommercialType: "GP1-XS",
				State:          scwinstance.ServerStateStarting,
			},
			// a server with private network only (no public IP)
			"private-only-server-id": {
				Zone:           "fr-par-2",
				Name:           "scw-private-only",
				CommercialType: "PLAY2-NANO",
				State:          scwinstance.ServerStateRunning,
				Project:        "test-project",
				PrivateNics: []*scwinstance.PrivateNIC{
					{
						ID:               "nic-private-only",
						PrivateNetworkID: "pn-12345",
					},
				},
			},
			// a server with both public and private network
			"both-networks-server-id": {
				Zone:           "fr-par-2",
				Name:           "scw-both-networks",
				CommercialType: "PLAY2-NANO",
				State:          scwinstance.ServerStateRunning,
				Project:        "test-project",
				PublicIPs: []*scwinstance.ServerIP{
					{
						Address:          net.ParseIP("51.159.100.1"),
						ProvisioningMode: scwinstance.ServerIPProvisioningModeDHCP,
						Family:           scwinstance.ServerIPIPFamilyInet,
					},
				},
				PrivateNics: []*scwinstance.PrivateNIC{
					{
						ID:               "nic-both-networks",
						PrivateNetworkID: "pn-12345",
					},
				},
			},
			// a server with multiple private networks
			"multi-pn-server-id": {
				Zone:           "fr-par-2",
				Name:           "scw-multi-pn",
				CommercialType: "PLAY2-NANO",
				State:          scwinstance.ServerStateRunning,
				Project:        "test-project",
				PrivateNics: []*scwinstance.PrivateNIC{
					{
						ID:               "nic-private-only",
						PrivateNetworkID: "pn-12345",
					},
					{
						ID:               "nic-other-pn",
						PrivateNetworkID: "pn-67890",
					},
				},
			},
		},
	}
}

func (f *fakeInstanceAPI) ListServers(req *scwinstance.ListServersRequest, opts ...scw.RequestOption) (*scwinstance.ListServersResponse, error) {
	servers := make([]*scwinstance.Server, 0)
	for id, server := range f.Servers {
		if req.Zone != server.Zone {
			continue
		}

		if req.Name == nil || strings.Contains(server.Name, *req.Name) {
			server.ID = id
			server.Hostname = server.Name
			servers = append(servers, server)
		}
	}

	return &scwinstance.ListServersResponse{
		Servers:    servers,
		TotalCount: uint32(len(servers)),
	}, nil
}

func (f *fakeInstanceAPI) GetServer(req *scwinstance.GetServerRequest, opts ...scw.RequestOption) (*scwinstance.GetServerResponse, error) {
	server, ok := f.Servers[req.ServerID]
	if !ok {
		return nil, &scw.ResourceNotFoundError{}
	}

	server.ID = req.ServerID
	server.Hostname = server.Name
	return &scwinstance.GetServerResponse{
		Server: server,
	}, nil
}

// fakeIPAMAPI implements IPAMAPI for testing
type fakeIPAMAPI struct {
	// IPs maps NIC ID to list of IPs
	IPs map[string][]*scwipam.IP
	// ErrorToReturn allows tests to inject errors
	ErrorToReturn error
	// CallCount tracks how many times ListIPs was called
	CallCount int
	// FailUntilAttempt causes errors until this attempt number (1-indexed)
	FailUntilAttempt int
}

func newFakeIPAMAPI() *fakeIPAMAPI {
	return &fakeIPAMAPI{
		IPs: map[string][]*scwipam.IP{
			// IPs for private NIC on server with private network only
			"nic-private-only": {
				{
					Address: scw.IPNet{IPNet: net.IPNet{IP: net.ParseIP("10.200.0.5"), Mask: net.CIDRMask(20, 32)}},
				},
			},
			// IPs for private NIC on server with both public and private
			"nic-both-networks": {
				{
					Address: scw.IPNet{IPNet: net.IPNet{IP: net.ParseIP("10.200.0.10"), Mask: net.CIDRMask(20, 32)}},
				},
			},
			// IPs for another private network
			"nic-other-pn": {
				{
					Address: scw.IPNet{IPNet: net.IPNet{IP: net.ParseIP("192.168.1.5"), Mask: net.CIDRMask(24, 32)}},
				},
			},
		},
	}
}

func (f *fakeIPAMAPI) ListIPs(req *scwipam.ListIPsRequest, opts ...scw.RequestOption) (*scwipam.ListIPsResponse, error) {
	f.CallCount++

	// Return error if configured and we haven't reached the success attempt yet
	if f.ErrorToReturn != nil && (f.FailUntilAttempt == 0 || f.CallCount <= f.FailUntilAttempt) {
		return nil, f.ErrorToReturn
	}

	if req.ResourceID == nil {
		return &scwipam.ListIPsResponse{IPs: []*scwipam.IP{}}, nil
	}

	ips, ok := f.IPs[*req.ResourceID]
	if !ok {
		return &scwipam.ListIPsResponse{IPs: []*scwipam.IP{}}, nil
	}

	return &scwipam.ListIPsResponse{
		IPs:        ips,
		TotalCount: uint64(len(ips)),
	}, nil
}

func newFakeInstances() *instances {
	return &instances{
		api:  newFakeInstanceAPI(),
		ipam: newFakeIPAMAPI(),
	}
}

func newFakeInstancesWithPNID(pnID string) *instances {
	return &instances{
		api:  newFakeInstanceAPI(),
		ipam: newFakeIPAMAPI(),
		pnID: pnID,
	}
}

func TestInstances_NodeAddresses(t *testing.T) {
	instance := newFakeInstances()

	t.Run("WithPublic", func(t *testing.T) {
		expectedAddresses := []v1.NodeAddress{
			{Type: v1.NodeHostName, Address: "scw-nervous-mccarthy"},
			{Type: v1.NodeExternalIP, Address: "62.210.16.2"},
			{Type: v1.NodeExternalDNS, Address: "232bf860-9ffe-4b08-97da-158c0316ed15.pub.instances.scw.cloud"},
			{Type: v1.NodeInternalIP, Address: "10.14.0.1"},
			{Type: v1.NodeInternalDNS, Address: "232bf860-9ffe-4b08-97da-158c0316ed15.priv.instances.scw.cloud"},
		}

		returnedAddresses, err := instance.NodeAddresses(context.TODO(), "scw-nervous-mccarthy")
		AssertNoError(t, err)
		Equals(t, expectedAddresses, returnedAddresses)
	})

	t.Run("WithoutPublic", func(t *testing.T) {
		expectedAddresses := []v1.NodeAddress{
			{Type: v1.NodeHostName, Address: "scw-charming-feistel"},
			{Type: v1.NodeInternalIP, Address: "10.14.0.1"},
			{Type: v1.NodeInternalDNS, Address: "a7ab9055-5fe2-4009-afd0-7787399e6755.priv.instances.scw.cloud"},
		}

		returnedAddresses, err := instance.NodeAddresses(context.TODO(), "scw-charming-feistel")
		AssertNoError(t, err)
		Equals(t, expectedAddresses, returnedAddresses)
	})
}

func TestInstances_NodeAddressesByProviderID(t *testing.T) {
	instance := newFakeInstances()

	t.Run("WithPublic", func(t *testing.T) {
		expectedAddresses := []v1.NodeAddress{
			{Type: v1.NodeHostName, Address: "scw-nervous-mccarthy"},
			{Type: v1.NodeExternalIP, Address: "62.210.16.2"},
			{Type: v1.NodeExternalDNS, Address: "232bf860-9ffe-4b08-97da-158c0316ed15.pub.instances.scw.cloud"},
			{Type: v1.NodeInternalIP, Address: "10.14.0.1"},
			{Type: v1.NodeInternalDNS, Address: "232bf860-9ffe-4b08-97da-158c0316ed15.priv.instances.scw.cloud"},
		}

		returnedAddresses, err := instance.NodeAddressesByProviderID(context.TODO(), "scaleway://instance/fr-par-1/232bf860-9ffe-4b08-97da-158c0316ed15")
		AssertNoError(t, err)
		Equals(t, returnedAddresses, expectedAddresses)
	})

	t.Run("WithoutPublic", func(t *testing.T) {
		expected := []v1.NodeAddress{
			{Type: v1.NodeHostName, Address: "scw-charming-feistel"},
			{Type: v1.NodeInternalIP, Address: "10.14.0.1"},
			{Type: v1.NodeInternalDNS, Address: "a7ab9055-5fe2-4009-afd0-7787399e6755.priv.instances.scw.cloud"},
		}

		result, err := instance.NodeAddressesByProviderID(context.TODO(), "scaleway://instance/fr-par-1/a7ab9055-5fe2-4009-afd0-7787399e6755")
		AssertNoError(t, err)
		Equals(t, expected, result)
	})
}

func TestInstances_InstanceID(t *testing.T) {
	instance := newFakeInstances()

	t.Run("Found", func(t *testing.T) {
		result, err := instance.InstanceID(context.TODO(), "scw-nervous-mccarthy")
		AssertNoError(t, err)
		Equals(t, "scaleway://instance/fr-par-1/232bf860-9ffe-4b08-97da-158c0316ed15", result)
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := instance.InstanceID(context.TODO(), "scw-missing-node")
		Equals(t, err, cloudprovider.InstanceNotFound)
	})
}

func TestInstances_InstanceType(t *testing.T) {
	instance := newFakeInstances()

	t.Run("Found", func(t *testing.T) {
		result, err := instance.InstanceType(context.TODO(), "scw-nervous-mccarthy")
		AssertNoError(t, err)
		Equals(t, "GP1-XS", result)
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := instance.InstanceType(context.TODO(), "scw-missing-node")
		Equals(t, err, cloudprovider.InstanceNotFound)
	})
}

func TestInstances_InstanceTypeByProviderID(t *testing.T) {
	instance := newFakeInstances()

	t.Run("Found", func(t *testing.T) {
		result, err := instance.InstanceTypeByProviderID(context.TODO(), "scaleway://instance/fr-par-1/232bf860-9ffe-4b08-97da-158c0316ed15")
		AssertNoError(t, err)
		Equals(t, "GP1-XS", result)
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := instance.InstanceTypeByProviderID(context.TODO(), "scaleway://instance/fr-par-1/b5c9fe34-4fa7-4902-86fd-8b68c230b0df")
		Equals(t, err, cloudprovider.InstanceNotFound)
	})
}

func TestInstances_InstanceExistsByProviderID(t *testing.T) {
	instance := newFakeInstances()

	t.Run("Running", func(t *testing.T) {
		result, err := instance.InstanceExistsByProviderID(context.TODO(), "scaleway://instance/fr-par-1/232bf860-9ffe-4b08-97da-158c0316ed15")
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("Shutdown", func(t *testing.T) {
		result, err := instance.InstanceExistsByProviderID(context.TODO(), "scaleway://instance/fr-par-1/f082dc51-bffa-4cb2-b0ac-e8b1ddeefaf0")
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("NotFound", func(t *testing.T) {
		result, err := instance.InstanceExistsByProviderID(context.TODO(), "scaleway://instance/fr-par-1/b5c9fe34-4fa7-4902-86fd-8b68c230b0df")
		AssertNoError(t, err)
		AssertFalse(t, result)
	})
}

func TestInstances_InstanceShutdownByProviderID(t *testing.T) {
	instance := newFakeInstances()

	t.Run("Running", func(t *testing.T) {
		result, err := instance.InstanceShutdownByProviderID(context.TODO(), "scaleway://instance/fr-par-1/232bf860-9ffe-4b08-97da-158c0316ed15")
		AssertNoError(t, err)
		AssertFalse(t, result)
	})

	t.Run("Stopping", func(t *testing.T) {
		result, err := instance.InstanceShutdownByProviderID(context.TODO(), "scaleway://instance/fr-par-1/23ffb180-80cb-489d-9eba-f2b6ebe44d32")
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("StoppedInPlace", func(t *testing.T) {
		result, err := instance.InstanceShutdownByProviderID(context.TODO(), "scaleway://instance/fr-par-1/428ed385-4508-4a19-837b-a7c3653cdda2")
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("Stopped", func(t *testing.T) {
		result, err := instance.InstanceShutdownByProviderID(context.TODO(), "scaleway://instance/fr-par-1/f082dc51-bffa-4cb2-b0ac-e8b1ddeefaf0")
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("NotFound", func(t *testing.T) {
		result, err := instance.InstanceShutdownByProviderID(context.TODO(), "scaleway://instance/fr-par-1/b5c9fe34-4fa7-4902-86fd-8b68c230b0df")
		Equals(t, err, cloudprovider.InstanceNotFound)
		AssertFalse(t, result)
	})
}

func TestInstances_GetZoneByNodeName(t *testing.T) {
	instance := newFakeInstances()

	// when running, FailureDomain is specified
	t.Run("Running", func(t *testing.T) {
		expected := cloudprovider.Zone{Region: "fr-par", FailureDomain: "fr-par-1"}

		result, err := instance.GetZoneByNodeName(context.TODO(), "scw-nervous-mccarthy")
		AssertNoError(t, err)
		Equals(t, expected, result)
	})

	// when stopped, FailureDomain is correclty set
	t.Run("Stopped", func(t *testing.T) {
		expected := cloudprovider.Zone{Region: "fr-par", FailureDomain: "fr-par-1"}

		result, err := instance.GetZoneByNodeName(context.TODO(), "scw-gifted-bose")
		AssertNoError(t, err)
		Equals(t, expected, result)
	})

	// when deleted, FailureDomain is empty as no slots is allocated
	t.Run("Deleted", func(t *testing.T) {
		expected := cloudprovider.Zone{Region: "", FailureDomain: ""}

		result, err := instance.GetZoneByNodeName(context.TODO(), "scw-missing-node")
		Equals(t, cloudprovider.InstanceNotFound, err)
		Equals(t, expected, result)
	})

}

func TestInstances_GetZoneByProviderID(t *testing.T) {
	instance := newFakeInstances()

	// when running, FailureDomain is specified
	t.Run("Running", func(t *testing.T) {
		expected := cloudprovider.Zone{Region: "fr-par", FailureDomain: "fr-par-1"}

		result, err := instance.GetZoneByProviderID(context.TODO(), "scaleway://instance/fr-par-1/232bf860-9ffe-4b08-97da-158c0316ed15")
		AssertNoError(t, err)
		Equals(t, expected, result)
	})

	// when stopped, FailureDomain is correclty set
	t.Run("Stopped", func(t *testing.T) {
		expected := cloudprovider.Zone{Region: "fr-par", FailureDomain: "fr-par-1"}

		result, err := instance.GetZoneByProviderID(context.TODO(), "scaleway://instance/fr-par-1/f082dc51-bffa-4cb2-b0ac-e8b1ddeefaf0")
		AssertNoError(t, err)
		Equals(t, expected, result)
	})
}

func TestInstances_InstanceExists(t *testing.T) {
	instance := newFakeInstances()

	t.Run("Running by ID", func(t *testing.T) {
		result, err := instance.InstanceExists(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://instance/fr-par-1/232bf860-9ffe-4b08-97da-158c0316ed15",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("Shutdown by ID", func(t *testing.T) {
		result, err := instance.InstanceExists(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://instance/fr-par-1/f082dc51-bffa-4cb2-b0ac-e8b1ddeefaf0",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("NotFound by ID", func(t *testing.T) {
		result, err := instance.InstanceExists(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://instance/fr-par-1/b5c9fe34-4fa7-4902-86fd-8b68c230b0df",
			},
		})
		AssertNoError(t, err)
		AssertFalse(t, result)
	})

	t.Run("Running by name", func(t *testing.T) {
		result, err := instance.InstanceExists(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scw-nervous-mccarthy",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("Shutdown by name", func(t *testing.T) {
		result, err := instance.InstanceExists(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scw-gifted-bose",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("NotFound by name", func(t *testing.T) {
		result, err := instance.InstanceExists(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-name",
			},
		})
		AssertNoError(t, err)
		AssertFalse(t, result)
	})
}

func TestInstances_InstanceShutdown(t *testing.T) {
	instance := newFakeInstances()

	t.Run("Running by ID", func(t *testing.T) {
		result, err := instance.InstanceShutdown(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://instance/fr-par-1/232bf860-9ffe-4b08-97da-158c0316ed15",
			},
		})
		AssertNoError(t, err)
		AssertFalse(t, result)
	})

	t.Run("Shutdown by ID", func(t *testing.T) {
		result, err := instance.InstanceShutdown(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://instance/fr-par-1/f082dc51-bffa-4cb2-b0ac-e8b1ddeefaf0",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("Starting by ID", func(t *testing.T) {
		result, err := instance.InstanceShutdown(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://instance/fr-par-1/8472b253-ea1f-464a-a49b-086b965467ff",
			},
		})
		AssertNoError(t, err)
		AssertFalse(t, result)
	})

	t.Run("NotFound by ID", func(t *testing.T) {
		_, err := instance.InstanceShutdown(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://instance/fr-par-1/b5c9fe34-4fa7-4902-86fd-8b68c230b0df",
			},
		})
		AssertTrue(t, err == cloudprovider.InstanceNotFound)
	})

	t.Run("Running by name", func(t *testing.T) {
		result, err := instance.InstanceShutdown(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scw-nervous-mccarthy",
			},
		})
		AssertNoError(t, err)
		AssertFalse(t, result)
	})

	t.Run("Shutdown by name", func(t *testing.T) {
		result, err := instance.InstanceShutdown(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scw-gifted-bose",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})
	t.Run("Starting by name", func(t *testing.T) {
		result, err := instance.InstanceShutdown(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scw-funny-gallois",
			},
		})
		AssertNoError(t, err)
		AssertFalse(t, result)
	})

	t.Run("NotFound by name", func(t *testing.T) {
		_, err := instance.InstanceShutdown(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-name",
			},
		})
		AssertTrue(t, err == cloudprovider.InstanceNotFound)
	})
}

func TestInstances_InstanceMetadata(t *testing.T) {
	instance := newFakeInstances()
	expectedAddresses := []v1.NodeAddress{
		{Type: v1.NodeHostName, Address: "scw-nervous-mccarthy"},
		{Type: v1.NodeExternalIP, Address: "62.210.16.2"},
		{Type: v1.NodeExternalDNS, Address: "232bf860-9ffe-4b08-97da-158c0316ed15.pub.instances.scw.cloud"},
		{Type: v1.NodeInternalIP, Address: "10.14.0.1"},
		{Type: v1.NodeInternalDNS, Address: "232bf860-9ffe-4b08-97da-158c0316ed15.priv.instances.scw.cloud"},
	}

	providerID := "scaleway://instance/fr-par-1/232bf860-9ffe-4b08-97da-158c0316ed15"
	nodeType := "GP1-XS"

	t.Run("By ID", func(t *testing.T) {
		metadata, err := instance.InstanceMetadata(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: providerID,
			},
		})

		AssertNoError(t, err)
		Equals(t, expectedAddresses, metadata.NodeAddresses)
		Equals(t, nodeType, metadata.InstanceType)
		Equals(t, providerID, metadata.ProviderID)
	})

	t.Run("By name", func(t *testing.T) {
		metadata, err := instance.InstanceMetadata(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scw-nervous-mccarthy",
			},
		})

		AssertNoError(t, err)
		Equals(t, expectedAddresses, metadata.NodeAddresses)
		Equals(t, nodeType, metadata.InstanceType)
		Equals(t, providerID, metadata.ProviderID)

	})
}

func TestInstances_PrivateNetworkAddresses(t *testing.T) {
	t.Run("PrivateNetworkOnly_NoPNID", func(t *testing.T) {
		// When no pnID is configured, the CCM should discover private IPs from all NICs
		instance := newFakeInstances()

		expectedAddresses := []v1.NodeAddress{
			{Type: v1.NodeHostName, Address: "scw-private-only"},
			{Type: v1.NodeInternalIP, Address: "10.200.0.5"},
		}

		returnedAddresses, err := instance.NodeAddressesByProviderID(context.TODO(), "scaleway://instance/fr-par-2/private-only-server-id")
		AssertNoError(t, err)
		Equals(t, expectedAddresses, returnedAddresses)
	})

	t.Run("PrivateNetworkOnly_WithPNID", func(t *testing.T) {
		// When pnID is configured, only look for that specific private network
		instance := newFakeInstancesWithPNID("pn-12345")

		expectedAddresses := []v1.NodeAddress{
			{Type: v1.NodeHostName, Address: "scw-private-only"},
			{Type: v1.NodeInternalIP, Address: "10.200.0.5"},
		}

		returnedAddresses, err := instance.NodeAddressesByProviderID(context.TODO(), "scaleway://instance/fr-par-2/private-only-server-id")
		AssertNoError(t, err)
		Equals(t, expectedAddresses, returnedAddresses)
	})

	t.Run("BothNetworks_NoPNID", func(t *testing.T) {
		// Server with both public and private network, no specific pnID configured
		instance := newFakeInstances()

		expectedAddresses := []v1.NodeAddress{
			{Type: v1.NodeHostName, Address: "scw-both-networks"},
			{Type: v1.NodeExternalIP, Address: "51.159.100.1"},
			{Type: v1.NodeExternalDNS, Address: "both-networks-server-id.pub.instances.scw.cloud"},
			{Type: v1.NodeInternalIP, Address: "10.200.0.10"},
		}

		returnedAddresses, err := instance.NodeAddressesByProviderID(context.TODO(), "scaleway://instance/fr-par-2/both-networks-server-id")
		AssertNoError(t, err)
		Equals(t, expectedAddresses, returnedAddresses)
	})

	t.Run("MultiplePrivateNetworks_NoPNID", func(t *testing.T) {
		// Server with multiple private networks, no specific pnID configured
		// Should return IPs from all private networks
		instance := newFakeInstances()

		expectedAddresses := []v1.NodeAddress{
			{Type: v1.NodeHostName, Address: "scw-multi-pn"},
			{Type: v1.NodeInternalIP, Address: "10.200.0.5"},
			{Type: v1.NodeInternalIP, Address: "192.168.1.5"},
		}

		returnedAddresses, err := instance.NodeAddressesByProviderID(context.TODO(), "scaleway://instance/fr-par-2/multi-pn-server-id")
		AssertNoError(t, err)
		Equals(t, expectedAddresses, returnedAddresses)
	})

	t.Run("MultiplePrivateNetworks_WithPNID", func(t *testing.T) {
		// Server with multiple private networks, specific pnID configured
		// Should only return IPs from the configured private network
		instance := newFakeInstancesWithPNID("pn-67890")

		expectedAddresses := []v1.NodeAddress{
			{Type: v1.NodeHostName, Address: "scw-multi-pn"},
			{Type: v1.NodeInternalIP, Address: "192.168.1.5"},
		}

		returnedAddresses, err := instance.NodeAddressesByProviderID(context.TODO(), "scaleway://instance/fr-par-2/multi-pn-server-id")
		AssertNoError(t, err)
		Equals(t, expectedAddresses, returnedAddresses)
	})
}
