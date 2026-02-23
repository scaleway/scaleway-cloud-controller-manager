/*
Copyright 2021 Scaleway

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
	"sort"
	"testing"

	scwlb "github.com/scaleway/scaleway-sdk-go/api/lb/v1"
	scwvpc "github.com/scaleway/scaleway-sdk-go/api/vpc/v2"
	"github.com/scaleway/scaleway-sdk-go/scw"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// fakeLBAPI implements the subset of LoadBalancerAPI needed for attachPrivateNetworks tests.
type fakeLBAPI struct {
	privateNetworks []*scwlb.PrivateNetwork
	attachCalls     []string // track attached PN IDs
	detachCalls     []string // track detached PN IDs
}

func (f *fakeLBAPI) ListLBPrivateNetworks(req *scwlb.ZonedAPIListLBPrivateNetworksRequest, opts ...scw.RequestOption) (*scwlb.ListLBPrivateNetworksResponse, error) {
	return &scwlb.ListLBPrivateNetworksResponse{
		PrivateNetwork: f.privateNetworks,
		TotalCount:     uint32(len(f.privateNetworks)),
	}, nil
}

func (f *fakeLBAPI) AttachPrivateNetwork(req *scwlb.ZonedAPIAttachPrivateNetworkRequest, opts ...scw.RequestOption) (*scwlb.PrivateNetwork, error) {
	f.attachCalls = append(f.attachCalls, req.PrivateNetworkID)
	return &scwlb.PrivateNetwork{
		PrivateNetworkID: req.PrivateNetworkID,
		Status:           scwlb.PrivateNetworkStatusReady,
	}, nil
}

func (f *fakeLBAPI) DetachPrivateNetwork(req *scwlb.ZonedAPIDetachPrivateNetworkRequest, opts ...scw.RequestOption) error {
	f.detachCalls = append(f.detachCalls, req.PrivateNetworkID)
	return nil
}

// Stubs for remaining LoadBalancerAPI interface methods (not used in these tests)
func (f *fakeLBAPI) ListLBs(req *scwlb.ZonedAPIListLBsRequest, opts ...scw.RequestOption) (*scwlb.ListLBsResponse, error) {
	return nil, nil
}
func (f *fakeLBAPI) GetLB(req *scwlb.ZonedAPIGetLBRequest, opts ...scw.RequestOption) (*scwlb.LB, error) {
	return nil, nil
}
func (f *fakeLBAPI) CreateLB(req *scwlb.ZonedAPICreateLBRequest, opts ...scw.RequestOption) (*scwlb.LB, error) {
	return nil, nil
}
func (f *fakeLBAPI) UpdateLB(req *scwlb.ZonedAPIUpdateLBRequest, opts ...scw.RequestOption) (*scwlb.LB, error) {
	return nil, nil
}
func (f *fakeLBAPI) DeleteLB(req *scwlb.ZonedAPIDeleteLBRequest, opts ...scw.RequestOption) error {
	return nil
}
func (f *fakeLBAPI) MigrateLB(req *scwlb.ZonedAPIMigrateLBRequest, opts ...scw.RequestOption) (*scwlb.LB, error) {
	return nil, nil
}
func (f *fakeLBAPI) ListIPs(req *scwlb.ZonedAPIListIPsRequest, opts ...scw.RequestOption) (*scwlb.ListIPsResponse, error) {
	return nil, nil
}
func (f *fakeLBAPI) ListBackends(req *scwlb.ZonedAPIListBackendsRequest, opts ...scw.RequestOption) (*scwlb.ListBackendsResponse, error) {
	return nil, nil
}
func (f *fakeLBAPI) CreateBackend(req *scwlb.ZonedAPICreateBackendRequest, opts ...scw.RequestOption) (*scwlb.Backend, error) {
	return nil, nil
}
func (f *fakeLBAPI) UpdateBackend(req *scwlb.ZonedAPIUpdateBackendRequest, opts ...scw.RequestOption) (*scwlb.Backend, error) {
	return nil, nil
}
func (f *fakeLBAPI) DeleteBackend(req *scwlb.ZonedAPIDeleteBackendRequest, opts ...scw.RequestOption) error {
	return nil
}
func (f *fakeLBAPI) SetBackendServers(req *scwlb.ZonedAPISetBackendServersRequest, opts ...scw.RequestOption) (*scwlb.Backend, error) {
	return nil, nil
}
func (f *fakeLBAPI) UpdateHealthCheck(req *scwlb.ZonedAPIUpdateHealthCheckRequest, opts ...scw.RequestOption) (*scwlb.HealthCheck, error) {
	return nil, nil
}
func (f *fakeLBAPI) ListFrontends(req *scwlb.ZonedAPIListFrontendsRequest, opts ...scw.RequestOption) (*scwlb.ListFrontendsResponse, error) {
	return nil, nil
}
func (f *fakeLBAPI) CreateFrontend(req *scwlb.ZonedAPICreateFrontendRequest, opts ...scw.RequestOption) (*scwlb.Frontend, error) {
	return nil, nil
}
func (f *fakeLBAPI) UpdateFrontend(req *scwlb.ZonedAPIUpdateFrontendRequest, opts ...scw.RequestOption) (*scwlb.Frontend, error) {
	return nil, nil
}
func (f *fakeLBAPI) DeleteFrontend(req *scwlb.ZonedAPIDeleteFrontendRequest, opts ...scw.RequestOption) error {
	return nil
}
func (f *fakeLBAPI) ListACLs(req *scwlb.ZonedAPIListACLsRequest, opts ...scw.RequestOption) (*scwlb.ListACLResponse, error) {
	return nil, nil
}
func (f *fakeLBAPI) CreateACL(req *scwlb.ZonedAPICreateACLRequest, opts ...scw.RequestOption) (*scwlb.ACL, error) {
	return nil, nil
}
func (f *fakeLBAPI) DeleteACL(req *scwlb.ZonedAPIDeleteACLRequest, opts ...scw.RequestOption) error {
	return nil
}
func (f *fakeLBAPI) UpdateACL(req *scwlb.ZonedAPIUpdateACLRequest, opts ...scw.RequestOption) (*scwlb.ACL, error) {
	return nil, nil
}
func (f *fakeLBAPI) SetACLs(req *scwlb.ZonedAPISetACLsRequest, opts ...scw.RequestOption) (*scwlb.SetACLsResponse, error) {
	return nil, nil
}

// fakeVPCAPI implements VPCAPI for testing name resolution.
type fakeVPCAPI struct {
	vpcs            []*scwvpc.VPC
	privateNetworks []*scwvpc.PrivateNetwork
}

func (f *fakeVPCAPI) GetPrivateNetwork(req *scwvpc.GetPrivateNetworkRequest, opts ...scw.RequestOption) (*scwvpc.PrivateNetwork, error) {
	for _, pn := range f.privateNetworks {
		if pn.ID == req.PrivateNetworkID {
			return pn, nil
		}
	}
	return nil, &scw.ResourceNotFoundError{}
}

func (f *fakeVPCAPI) ListPrivateNetworks(req *scwvpc.ListPrivateNetworksRequest, opts ...scw.RequestOption) (*scwvpc.ListPrivateNetworksResponse, error) {
	var result []*scwvpc.PrivateNetwork
	for _, pn := range f.privateNetworks {
		if req.VpcID != nil && pn.VpcID != *req.VpcID {
			continue
		}
		if req.Name != nil && pn.Name != *req.Name {
			continue
		}
		result = append(result, pn)
	}
	return &scwvpc.ListPrivateNetworksResponse{
		PrivateNetworks: result,
		TotalCount:      uint32(len(result)),
	}, nil
}

func (f *fakeVPCAPI) ListVPCs(req *scwvpc.ListVPCsRequest, opts ...scw.RequestOption) (*scwvpc.ListVPCsResponse, error) {
	var result []*scwvpc.VPC
	for _, vpc := range f.vpcs {
		if req.Name != nil && vpc.Name != *req.Name {
			continue
		}
		if req.ProjectID != nil && vpc.ProjectID != *req.ProjectID {
			continue
		}
		result = append(result, vpc)
	}
	return &scwvpc.ListVPCsResponse{
		Vpcs:       result,
		TotalCount: uint32(len(result)),
	}, nil
}

func newTestLB(lbAPI *fakeLBAPI, vpcAPI *fakeVPCAPI, pnID string) *loadbalancers {
	return &loadbalancers{
		api:  lbAPI,
		vpc:  vpcAPI,
		pnID: pnID,
	}
}

func TestAttachPrivateNetworks(t *testing.T) {
	testLB := &scwlb.LB{
		ID:   "lb-1",
		Zone: scw.ZoneFrPar2,
	}

	vpcAPI := &fakeVPCAPI{
		vpcs: []*scwvpc.VPC{
			{ID: "vpc-default", Name: "default", Region: scw.RegionFrPar, ProjectID: "proj-1"},
			{ID: "vpc-other", Name: "other-vpc", Region: scw.RegionFrPar, ProjectID: "proj-1"},
		},
		privateNetworks: []*scwvpc.PrivateNetwork{
			{ID: "pn-resolved-1", Name: "my-cluster", VpcID: "vpc-default"},
			{ID: "pn-resolved-2", Name: "my-other-pn", VpcID: "vpc-other"},
		},
	}

	tests := []struct {
		name                string
		service             *v1.Service
		envPNID             string
		existingPNs         []*scwlb.PrivateNetwork
		lbExternallyManaged bool
		wantConfiguredIDs   []string
		wantAttachCalls     []string
		wantDetachCalls     []string
		wantErr             bool
	}{
		{
			name: "Priority 1: pn-ids annotation attaches PN and returns IDs",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-pn-ids": "pn-explicit-1",
					},
				},
			},
			existingPNs:       nil,
			wantConfiguredIDs: []string{"pn-explicit-1"},
			wantAttachCalls:   []string{"pn-explicit-1"},
		},
		{
			name: "Priority 1: multiple pn-ids",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-pn-ids": "pn-a,pn-b",
					},
				},
			},
			existingPNs:       nil,
			wantConfiguredIDs: []string{"pn-a", "pn-b"},
			wantAttachCalls:   []string{"pn-a", "pn-b"},
		},
		{
			name: "Priority 1: pn-ids already attached skips attach call",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-pn-ids": "pn-already-attached",
					},
				},
			},
			existingPNs: []*scwlb.PrivateNetwork{
				{PrivateNetworkID: "pn-already-attached", Status: scwlb.PrivateNetworkStatusReady},
			},
			wantConfiguredIDs: []string{"pn-already-attached"},
			wantAttachCalls:   nil, // no attach needed
		},
		{
			name: "Priority 2: pn-names resolves and returns IDs",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-pn-names": "default/my-cluster",
					},
				},
			},
			existingPNs:       nil,
			wantConfiguredIDs: []string{"pn-resolved-1"},
			wantAttachCalls:   []string{"pn-resolved-1"},
		},
		{
			name: "Priority 2: multiple pn-names across VPCs",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-pn-names": "default/my-cluster,other-vpc/my-other-pn",
					},
				},
			},
			existingPNs:       nil,
			wantConfiguredIDs: []string{"pn-resolved-1", "pn-resolved-2"},
			wantAttachCalls:   []string{"pn-resolved-1", "pn-resolved-2"},
		},
		{
			name: "Priority 3: env var PN_ID used when no annotations",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			envPNID:           "pn-from-env",
			existingPNs:       nil,
			wantConfiguredIDs: []string{"pn-from-env"},
			wantAttachCalls:   []string{"pn-from-env"},
		},
		{
			name: "Priority: pn-ids takes precedence over pn-names",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-pn-ids":   "pn-explicit",
						"service.beta.kubernetes.io/scw-loadbalancer-pn-names": "default/my-cluster",
					},
				},
			},
			existingPNs:       nil,
			wantConfiguredIDs: []string{"pn-explicit"},
			wantAttachCalls:   []string{"pn-explicit"},
		},
		{
			name: "Priority: pn-ids takes precedence over env var",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-pn-ids": "pn-explicit",
					},
				},
			},
			envPNID:           "pn-from-env",
			existingPNs:       nil,
			wantConfiguredIDs: []string{"pn-explicit"},
			wantAttachCalls:   []string{"pn-explicit"},
		},
		{
			name: "Priority: pn-names takes precedence over env var",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-pn-names": "default/my-cluster",
					},
				},
			},
			envPNID:           "pn-from-env",
			existingPNs:       nil,
			wantConfiguredIDs: []string{"pn-resolved-1"},
			wantAttachCalls:   []string{"pn-resolved-1"},
		},
		{
			name: "No PN config returns nil",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			envPNID:           "",
			existingPNs:       nil,
			wantConfiguredIDs: nil,
			wantAttachCalls:   nil,
		},
		{
			name: "Detaches unmatched PNs when not externally managed",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-pn-ids": "pn-wanted",
					},
				},
			},
			existingPNs: []*scwlb.PrivateNetwork{
				{PrivateNetworkID: "pn-stale", Status: scwlb.PrivateNetworkStatusReady},
			},
			wantConfiguredIDs: []string{"pn-wanted"},
			wantAttachCalls:   []string{"pn-wanted"},
			wantDetachCalls:   []string{"pn-stale"},
		},
		{
			name: "Externally managed: skips annotation-based PNs, uses env var",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"service.beta.kubernetes.io/scw-loadbalancer-pn-ids": "pn-ignored",
					},
				},
			},
			lbExternallyManaged: true,
			envPNID:             "pn-from-env",
			existingPNs:         nil,
			wantConfiguredIDs:   []string{"pn-from-env"},
			wantAttachCalls:     []string{"pn-from-env"},
		},
		{
			name: "Externally managed: does not detach unmatched PNs",
			service: &v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			lbExternallyManaged: true,
			envPNID:             "pn-from-env",
			existingPNs: []*scwlb.PrivateNetwork{
				{PrivateNetworkID: "pn-other", Status: scwlb.PrivateNetworkStatusReady},
			},
			wantConfiguredIDs: []string{"pn-from-env"},
			wantAttachCalls:   []string{"pn-from-env"},
			wantDetachCalls:   nil, // externally managed: no detach
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbAPI := &fakeLBAPI{
				privateNetworks: tt.existingPNs,
			}
			lb := newTestLB(lbAPI, vpcAPI, tt.envPNID)

			gotIDs, err := lb.attachPrivateNetworks(testLB, tt.service, tt.lbExternallyManaged, scw.RegionFrPar, "proj-1")
			if (err != nil) != tt.wantErr {
				t.Fatalf("attachPrivateNetworks() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Sort for deterministic comparison (map iteration order is random)
			sort.Strings(gotIDs)
			sort.Strings(tt.wantConfiguredIDs)
			if len(gotIDs) == 0 && len(tt.wantConfiguredIDs) == 0 {
				// both empty/nil, ok
			} else if len(gotIDs) != len(tt.wantConfiguredIDs) {
				t.Errorf("attachPrivateNetworks() returned IDs = %v, want %v", gotIDs, tt.wantConfiguredIDs)
			} else {
				for i := range gotIDs {
					if gotIDs[i] != tt.wantConfiguredIDs[i] {
						t.Errorf("attachPrivateNetworks() returned IDs = %v, want %v", gotIDs, tt.wantConfiguredIDs)
						break
					}
				}
			}

			// Verify attach calls
			sort.Strings(lbAPI.attachCalls)
			sort.Strings(tt.wantAttachCalls)
			if len(lbAPI.attachCalls) == 0 && len(tt.wantAttachCalls) == 0 {
				// both empty, ok
			} else if len(lbAPI.attachCalls) != len(tt.wantAttachCalls) {
				t.Errorf("attach calls = %v, want %v", lbAPI.attachCalls, tt.wantAttachCalls)
			} else {
				for i := range lbAPI.attachCalls {
					if lbAPI.attachCalls[i] != tt.wantAttachCalls[i] {
						t.Errorf("attach calls = %v, want %v", lbAPI.attachCalls, tt.wantAttachCalls)
						break
					}
				}
			}

			// Verify detach calls
			sort.Strings(lbAPI.detachCalls)
			sort.Strings(tt.wantDetachCalls)
			if len(lbAPI.detachCalls) == 0 && len(tt.wantDetachCalls) == 0 {
				// both empty, ok
			} else if len(lbAPI.detachCalls) != len(tt.wantDetachCalls) {
				t.Errorf("detach calls = %v, want %v", lbAPI.detachCalls, tt.wantDetachCalls)
			} else {
				for i := range lbAPI.detachCalls {
					if lbAPI.detachCalls[i] != tt.wantDetachCalls[i] {
						t.Errorf("detach calls = %v, want %v", lbAPI.detachCalls, tt.wantDetachCalls)
						break
					}
				}
			}
		})
	}
}
