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

	scwbaremetal "github.com/scaleway/scaleway-sdk-go/api/baremetal/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cloudprovider "k8s.io/cloud-provider"
)

type fakeBaremetalAPI struct {
	Servers map[string]*scwbaremetal.Server
	Offers  map[string]*scwbaremetal.Offer
}

func newFakeBaremetalAPI() *fakeBaremetalAPI {
	offerID := "778f24b8-a506-49b5-b847-4506a23df695"
	return &fakeBaremetalAPI{
		Offers: map[string]*scwbaremetal.Offer{
			offerID: {
				Name: "TEST1-L",
			},
		},
		Servers: map[string]*scwbaremetal.Server{
			// a ready server with tags
			"53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed": {
				Zone:    "fr-par-2",
				Name:    "bm-keen-feistel",
				Status:  scwbaremetal.ServerStatusReady,
				Tags:    []string{"baremetal", "something"},
				OfferID: offerID,
				IPs: []*scwbaremetal.IP{
					{
						Address: net.ParseIP("62.210.16.2"),
						Reverse: "53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed.fr-par-2.baremetal.scw.cloud",
						Version: scwbaremetal.IPVersionIPv4,
					},
					{
						Address: net.ParseIP("2001:bc8:4::5"),
						Reverse: "53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed.fr-par-2.baremetal.scw.cloud",
						Version: scwbaremetal.IPVersionIPv6,
					},
				},
				Domain: "53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed.fr-par-2.baremetal.scw.cloud",
			},
			// a ready server
			"7c9877e0-4415-447a-819f-12cc0047e3fe": {
				Zone:    "fr-par-2",
				Name:    "bm-laughing-ritchie",
				Status:  scwbaremetal.ServerStatusReady,
				OfferID: offerID,
				IPs: []*scwbaremetal.IP{
					{
						Address: net.ParseIP("62.210.16.2"),
						Reverse: "7c9877e0-4415-447a-819f-12cc0047e3fe.fr-par-2.baremetal.scw.cloud",
						Version: scwbaremetal.IPVersionIPv4,
					},
					{
						Address: net.ParseIP("2001:bc8:4::5"),
						Reverse: "7c9877e0-4415-447a-819f-12cc0047e3fe.fr-par-2.baremetal.scw.cloud",
						Version: scwbaremetal.IPVersionIPv6,
					},
				},
				Domain: "7c9877e0-4415-447a-819f-12cc0047e3fe.fr-par-2.baremetal.scw.cloud",
			},
			// a stopped server
			"42d74979-992e-4650-9d05-31b834547d69": {
				Zone:    "fr-par-2",
				Name:    "bm-optimistic-cohen",
				Status:  scwbaremetal.ServerStatusStopped,
				OfferID: offerID,
				IPs: []*scwbaremetal.IP{
					{
						Address: net.ParseIP("62.210.16.2"),
						Reverse: "42d74979-992e-4650-9d05-31b834547d69.fr-par-2.baremetal.scw.cloud",
						Version: scwbaremetal.IPVersionIPv4,
					},
					{
						Address: net.ParseIP("2001:bc8:4::5"),
						Reverse: "42d74979-992e-4650-9d05-31b834547d69.fr-par-2.baremetal.scw.cloud",
						Version: scwbaremetal.IPVersionIPv6,
					},
				},
				Domain: "42d74979-992e-4650-9d05-31b834547d69.fr-par-2.baremetal.scw.cloud",
			},
			// a server in a transiant state (deleting)
			"0d4656bf-53e0-4762-9b63-2fbc6a83d5b4": {
				Zone:    "fr-par-2",
				Name:    "bm-jovial-raman",
				Status:  scwbaremetal.ServerStatusDeleting,
				OfferID: offerID,
				IPs: []*scwbaremetal.IP{
					{
						Address: net.ParseIP("62.210.16.2"),
						Reverse: "0d4656bf-53e0-4762-9b63-2fbc6a83d5b4.fr-par-2.baremetal.scw.cloud",
						Version: scwbaremetal.IPVersionIPv4,
					},
					{
						Address: net.ParseIP("2001:bc8:4::5"),
						Reverse: "0d4656bf-53e0-4762-9b63-2fbc6a83d5b4.fr-par-2.baremetal.scw.cloud",
						Version: scwbaremetal.IPVersionIPv6,
					},
				},
				Domain: "0d4656bf-53e0-4762-9b63-2fbc6a83d5b4.fr-par-2.baremetal.scw.cloud",
			},
		},
	}
}

func newFakeBaremetal() *baremetal {
	return &baremetal{
		api: newFakeBaremetalAPI(),
	}
}

func (f *fakeBaremetalAPI) ListServers(req *scwbaremetal.ListServersRequest, opts ...scw.RequestOption) (*scwbaremetal.ListServersResponse, error) {
	servers := make([]*scwbaremetal.Server, 0)
	for id, server := range f.Servers {
		if req.Zone != server.Zone {
			continue
		}

		if req.Name == nil || strings.Contains(server.Name, *req.Name) {
			server.ID = id
			servers = append(servers, server)
		}
	}

	return &scwbaremetal.ListServersResponse{
		Servers:    servers,
		TotalCount: uint32(len(servers)),
	}, nil
}

func (f *fakeBaremetalAPI) GetServer(req *scwbaremetal.GetServerRequest, opts ...scw.RequestOption) (*scwbaremetal.Server, error) {
	server, ok := f.Servers[req.ServerID]
	if !ok {
		return nil, &scw.ResourceNotFoundError{}
	}

	server.ID = req.ServerID
	return server, nil
}

func (f *fakeBaremetalAPI) GetOffer(req *scwbaremetal.GetOfferRequest, opts ...scw.RequestOption) (*scwbaremetal.Offer, error) {
	offer, ok := f.Offers[req.OfferID]
	if !ok {
		return nil, &scw.ResourceNotFoundError{}
	}

	offer.ID = req.OfferID
	return offer, nil
}

func TestBaremetal_NodeAddresses(t *testing.T) {
	baremetal := newFakeBaremetal()

	expectedAddresses := []v1.NodeAddress{
		{Type: v1.NodeHostName, Address: "bm-keen-feistel"},
		{Type: v1.NodeExternalIP, Address: "62.210.16.2"},
		{Type: v1.NodeExternalDNS, Address: "53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed.fr-par-2.baremetal.scw.cloud"},
	}

	returnedAddresses, err := baremetal.NodeAddresses(context.TODO(), "bm-keen-feistel")
	AssertNoError(t, err)
	Equals(t, expectedAddresses, returnedAddresses)
}

func TestBaremetal_NodeAddressesByProviderID(t *testing.T) {
	baremetal := newFakeBaremetal()

	expectedAddresses := []v1.NodeAddress{
		{Type: v1.NodeHostName, Address: "bm-keen-feistel"},
		{Type: v1.NodeExternalIP, Address: "62.210.16.2"},
		{Type: v1.NodeExternalDNS, Address: "53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed.fr-par-2.baremetal.scw.cloud"},
	}

	returnedAddresses, err := baremetal.NodeAddressesByProviderID(context.TODO(), "scaleway://baremetal/fr-par-2/53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed")
	AssertNoError(t, err)
	Equals(t, expectedAddresses, returnedAddresses)
}

func TestBaremetal_InstanceID(t *testing.T) {
	baremetal := newFakeBaremetal()

	t.Run("Found", func(t *testing.T) {
		result, err := baremetal.InstanceID(context.TODO(), "bm-keen-feistel")
		AssertNoError(t, err)
		Equals(t, "scaleway://baremetal/fr-par-2/53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed", result)
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := baremetal.InstanceID(context.TODO(), "bm-not-found")
		Equals(t, err, cloudprovider.InstanceNotFound)
	})
}

func TestBaremetal_InstanceType(t *testing.T) {
	baremetal := newFakeBaremetal()

	result, err := baremetal.InstanceType(context.TODO(), "bm-keen-feistel")
	AssertNoError(t, err)
	Equals(t, "TEST1-L", result)
}

func TestBaremetal_InstanceTypeByProviderID(t *testing.T) {
	baremetal := newFakeBaremetal()

	result, err := baremetal.InstanceTypeByProviderID(context.TODO(), "scaleway://baremetal/fr-par-2/53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed")
	AssertNoError(t, err)
	Equals(t, "TEST1-L", result)
}

func TestBaremetal_InstanceExistsByProviderID(t *testing.T) {
	baremetal := newFakeBaremetal()

	t.Run("Running", func(t *testing.T) {
		result, err := baremetal.InstanceExistsByProviderID(context.TODO(), "scaleway://baremetal/fr-par-2/53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed")
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("Stopped", func(t *testing.T) {
		result, err := baremetal.InstanceExistsByProviderID(context.TODO(), "scaleway://baremetal/fr-par-2/42d74979-992e-4650-9d05-31b834547d69")
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("NotFound", func(t *testing.T) {
		result, err := baremetal.InstanceExistsByProviderID(context.TODO(), "scaleway://baremetal/fr-par-2/4d17aad4-c77e-4d95-a6e8-d7bf3f02d345")
		AssertNoError(t, err)
		AssertFalse(t, result)
	})
}

func TestBaremetal_InstanceShutdownByProviderID(t *testing.T) {
	baremetal := newFakeBaremetal()

	t.Run("Running", func(t *testing.T) {
		result, err := baremetal.InstanceShutdownByProviderID(context.TODO(), "scaleway://baremetal/fr-par-2/53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed")
		AssertNoError(t, err)
		AssertFalse(t, result)
	})

	t.Run("Deleting", func(t *testing.T) {
		result, err := baremetal.InstanceShutdownByProviderID(context.TODO(), "scaleway://baremetal/fr-par-2/42d74979-992e-4650-9d05-31b834547d69")
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("Stopped", func(t *testing.T) {
		result, err := baremetal.InstanceShutdownByProviderID(context.TODO(), "scaleway://baremetal/fr-par-2/42d74979-992e-4650-9d05-31b834547d69")
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("NotFound", func(t *testing.T) {
		result, err := baremetal.InstanceShutdownByProviderID(context.TODO(), "scaleway://baremetal/fr-par-2/4d17aad4-c77e-4d95-a6e8-d7bf3f02d345")
		Equals(t, err, cloudprovider.InstanceNotFound)
		AssertFalse(t, result)
	})
}

func TestBaremetal_GetZoneByNodeName(t *testing.T) {
	baremetal := newFakeBaremetal()

	// scaleway baremeral is hard allocated
	expected := cloudprovider.Zone{Region: "fr-par-2", FailureDomain: "unavailable"}
	result, err := baremetal.GetZoneByNodeName(context.TODO(), "bm-keen-feistel")
	AssertNoError(t, err)
	Equals(t, expected, result)
}

func TestBaremetal_GetZoneByProviderID(t *testing.T) {
	baremetal := newFakeBaremetal()

	// scaleway baremeral is hard allocated
	expected := cloudprovider.Zone{Region: "fr-par-2", FailureDomain: "unavailable"}
	result, err := baremetal.GetZoneByProviderID(context.TODO(), "scaleway://baremetal/fr-par-2/53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed")
	AssertNoError(t, err)
	Equals(t, expected, result)
}

func TestBaremetal_InstanceExists(t *testing.T) {
	baremetal := newFakeBaremetal()

	t.Run("Running by ID", func(t *testing.T) {
		result, err := baremetal.InstanceExists(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://baremetal/fr-par-2/53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("Stopped by ID", func(t *testing.T) {
		result, err := baremetal.InstanceExists(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://baremetal/fr-par-2/42d74979-992e-4650-9d05-31b834547d69",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("NotFound by ID", func(t *testing.T) {
		result, err := baremetal.InstanceExists(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://baremetal/fr-par-2/4d17aad4-c77e-4d95-a6e8-d7bf3f02d345",
			},
		})
		AssertNoError(t, err)
		AssertFalse(t, result)
	})
	t.Run("Running by name", func(t *testing.T) {
		result, err := baremetal.InstanceExists(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bm-keen-feistel",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("Stopped by name", func(t *testing.T) {
		result, err := baremetal.InstanceExists(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bm-optimistic-cohen",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("NotFound by name", func(t *testing.T) {
		result, err := baremetal.InstanceExists(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-name",
			},
		})
		AssertNoError(t, err)
		AssertFalse(t, result)
	})

}

func TestBaremetal_InstanceShutdown(t *testing.T) {
	baremetal := newFakeBaremetal()

	t.Run("Running by ID", func(t *testing.T) {
		result, err := baremetal.InstanceShutdown(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://baremetal/fr-par-2/53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed",
			},
		})
		AssertNoError(t, err)
		AssertFalse(t, result)
	})

	t.Run("Stopped by ID", func(t *testing.T) {
		result, err := baremetal.InstanceShutdown(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://baremetal/fr-par-2/42d74979-992e-4650-9d05-31b834547d69",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("NotFound by ID", func(t *testing.T) {
		_, err := baremetal.InstanceShutdown(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: "scaleway://baremetal/fr-par-2/4d17aad4-c77e-4d95-a6e8-d7bf3f02d345",
			},
		})
		AssertTrue(t, err == cloudprovider.InstanceNotFound)
	})

	t.Run("Running by name", func(t *testing.T) {
		result, err := baremetal.InstanceShutdown(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bm-keen-feistel",
			},
		})
		AssertNoError(t, err)
		AssertFalse(t, result)
	})

	t.Run("Stopped by name", func(t *testing.T) {
		result, err := baremetal.InstanceShutdown(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bm-optimistic-cohen",
			},
		})
		AssertNoError(t, err)
		AssertTrue(t, result)
	})

	t.Run("NotFound by name", func(t *testing.T) {
		_, err := baremetal.InstanceShutdown(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-name",
			},
		})
		AssertTrue(t, err == cloudprovider.InstanceNotFound)
	})
}

func TestBaremetal_InstanceMetadata(t *testing.T) {
	baremetal := newFakeBaremetal()

	expectedAddresses := []v1.NodeAddress{
		{Type: v1.NodeHostName, Address: "bm-keen-feistel"},
		{Type: v1.NodeExternalIP, Address: "62.210.16.2"},
		{Type: v1.NodeExternalDNS, Address: "53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed.fr-par-2.baremetal.scw.cloud"},
	}

	providerID := "scaleway://baremetal/fr-par-2/53a6ca14-0d8d-4f2e-9887-2f1bcfba58ed"
	offerName := "TEST1-L"

	t.Run("By ID", func(t *testing.T) {
		metadata, err := baremetal.InstanceMetadata(context.TODO(), &v1.Node{
			Spec: v1.NodeSpec{
				ProviderID: providerID,
			},
		})
		AssertNoError(t, err)
		Equals(t, expectedAddresses, metadata.NodeAddresses)
		Equals(t, offerName, metadata.InstanceType)
		Equals(t, providerID, metadata.ProviderID)
	})

	t.Run("By Name", func(t *testing.T) {
		metadata, err := baremetal.InstanceMetadata(context.TODO(), &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bm-keen-feistel",
			},
		})
		AssertNoError(t, err)
		Equals(t, expectedAddresses, metadata.NodeAddresses)
		Equals(t, offerName, metadata.InstanceType)
		Equals(t, providerID, metadata.ProviderID)
	})
}
