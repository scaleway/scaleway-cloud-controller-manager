package scaleway

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"

	scwlb "github.com/scaleway/scaleway-sdk-go/api/lb/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const MaxEntriesPerACL = 60

func (l *loadbalancers) reconcileACLs(ctx context.Context, loadbalancer *scwlb.LB, frontend *scwlb.Frontend, service *v1.Service, nodes []*v1.Node) error {
	// List ACLs for the frontend
	aclName := makeACLPrefix(frontend)
	aclsResp, err := l.api.ListACLs(&scwlb.ZonedAPIListACLsRequest{
		Zone:       loadbalancer.Zone,
		FrontendID: frontend.ID,
		Name:       &aclName,
	}, scw.WithAllPages(), scw.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed to list ACLs for frontend: %s port: %d loadbalancer: %s err: %v", frontend.ID, frontend.InboundPort, loadbalancer.ID, err)
	}

	svcAcls := makeACLSpecs(service, nodes, frontend)
	if !aclsEquals(aclsResp.ACLs, svcAcls) {
		klog.Infof("set ACLs for frontend: %s port: %d loadbalancer: %s", frontend.ID, frontend.InboundPort, loadbalancer.ID)
		if _, err := l.api.SetACLs(&scwlb.ZonedAPISetACLsRequest{
			Zone:       loadbalancer.Zone,
			FrontendID: frontend.ID,
			ACLs:       svcAcls,
		}, scw.WithContext(ctx)); err != nil {
			return fmt.Errorf("failed setting ACLs for frontend: %s port: %d loadbalancer: %s err: %v", frontend.ID, frontend.InboundPort, loadbalancer.ID, err)
		}
	}

	return nil
}

// makeACLPrefix returns the ACL prefix for rules
func makeACLPrefix(frontend *scwlb.Frontend) string {
	if frontend == nil {
		return "lb-source-range"
	}
	return fmt.Sprintf("%s-lb-source-range", frontend.ID)
}

// makeACLSpecs converts a service frontend definition to acl specifications
func makeACLSpecs(service *v1.Service, nodes []*v1.Node, frontend *scwlb.Frontend) []*scwlb.ACLSpec {
	if len(service.Spec.LoadBalancerSourceRanges) == 0 {
		return []*scwlb.ACLSpec{}
	}

	sourceRanges := make([]string, 0, len(service.Spec.LoadBalancerSourceRanges))
	for _, sourceRange := range service.Spec.LoadBalancerSourceRanges {
		if _, _, err := net.ParseCIDR(sourceRange); err != nil {
			klog.Warningf("ignoring invalid CIDR %s in LoadBalancerSourceRanges for service %s/%s: %v", sourceRange, service.Namespace, service.Name, err)
			continue
		}

		if strings.Contains(sourceRange, ":") {
			sourceRange = strings.TrimSuffix(sourceRange, "/128")
		} else {
			sourceRange = strings.TrimSuffix(sourceRange, "/32")
		}

		sourceRanges = append(sourceRanges, sourceRange)
	}

	aclPrefix := makeACLPrefix(frontend)
	whitelist := extractNodesInternalIps(nodes)
	whitelist = append(whitelist, extractNodesExternalIps(nodes)...)
	whitelist = append(whitelist, sourceRanges...)

	slices.Sort(whitelist)

	subnetsChunks := chunkArray(whitelist, MaxEntriesPerACL)
	acls := make([]*scwlb.ACLSpec, len(subnetsChunks)+1)

	for idx, subnets := range subnetsChunks {
		acls[idx] = &scwlb.ACLSpec{
			Name: fmt.Sprintf("%s-%d", aclPrefix, idx),
			Action: &scwlb.ACLAction{
				Type: scwlb.ACLActionTypeAllow,
			},
			Index: int32(idx),
			Match: &scwlb.ACLMatch{
				IPSubnet: scw.StringSlicePtr(subnets),
			},
		}
	}

	acls[len(acls)-1] = &scwlb.ACLSpec{
		Name: fmt.Sprintf("%s-end", aclPrefix),
		Action: &scwlb.ACLAction{
			Type: scwlb.ACLActionTypeDeny,
		},
		Index: int32(len(acls) - 1),
		Match: &scwlb.ACLMatch{
			IPSubnet: scw.StringSlicePtr([]string{"0.0.0.0/0", "::/0"}),
		},
	}

	return acls
}

// aclsEquals returns true if both acl lists are equal
func aclsEquals(got []*scwlb.ACL, want []*scwlb.ACLSpec) bool {
	if len(got) != len(want) {
		return false
	}

	slices.SortStableFunc(got, func(a, b *scwlb.ACL) int { return int(a.Index - b.Index) })
	slices.SortStableFunc(want, func(a, b *scwlb.ACLSpec) int { return int(a.Index - b.Index) })
	for idx := range want {
		if want[idx].Name != got[idx].Name {
			return false
		}
		if want[idx].Index != got[idx].Index {
			return false
		}
		if (want[idx].Action == nil) != (got[idx].Action == nil) {
			return false
		}
		if want[idx].Action != nil && want[idx].Action.Type != got[idx].Action.Type {
			return false
		}
		if (want[idx].Match == nil) != (got[idx].Match == nil) {
			return false
		}
		if want[idx].Match != nil && !stringPtrArrayEqual(want[idx].Match.IPSubnet, got[idx].Match.IPSubnet) {
			return false
		}
		if want[idx].Match != nil && want[idx].Match.Invert != got[idx].Match.Invert {
			return false
		}
	}

	return true
}
