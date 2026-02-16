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
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	v1 "k8s.io/api/core/v1"
	"k8s.io/cloud-provider/api"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	scwipam "github.com/scaleway/scaleway-sdk-go/api/ipam/v1"
	scwlb "github.com/scaleway/scaleway-sdk-go/api/lb/v1"
	scwvpc "github.com/scaleway/scaleway-sdk-go/api/vpc/v2"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

const MaxEntriesPerACL = 60

type loadbalancers struct {
	api           LoadBalancerAPI
	ipam          IPAMAPI
	vpc           VPCAPI
	client        *client // for patcher
	defaultLBType string
	pnID          string
}

type VPCAPI interface {
	GetPrivateNetwork(req *scwvpc.GetPrivateNetworkRequest, opts ...scw.RequestOption) (*scwvpc.PrivateNetwork, error)
}

type LoadBalancerAPI interface {
	ListLBs(req *scwlb.ZonedAPIListLBsRequest, opts ...scw.RequestOption) (*scwlb.ListLBsResponse, error)
	GetLB(req *scwlb.ZonedAPIGetLBRequest, opts ...scw.RequestOption) (*scwlb.LB, error)
	CreateLB(req *scwlb.ZonedAPICreateLBRequest, opts ...scw.RequestOption) (*scwlb.LB, error)
	UpdateLB(req *scwlb.ZonedAPIUpdateLBRequest, opts ...scw.RequestOption) (*scwlb.LB, error)
	DeleteLB(req *scwlb.ZonedAPIDeleteLBRequest, opts ...scw.RequestOption) error
	MigrateLB(req *scwlb.ZonedAPIMigrateLBRequest, opts ...scw.RequestOption) (*scwlb.LB, error)
	ListIPs(req *scwlb.ZonedAPIListIPsRequest, opts ...scw.RequestOption) (*scwlb.ListIPsResponse, error)
	ListBackends(req *scwlb.ZonedAPIListBackendsRequest, opts ...scw.RequestOption) (*scwlb.ListBackendsResponse, error)
	CreateBackend(req *scwlb.ZonedAPICreateBackendRequest, opts ...scw.RequestOption) (*scwlb.Backend, error)
	UpdateBackend(req *scwlb.ZonedAPIUpdateBackendRequest, opts ...scw.RequestOption) (*scwlb.Backend, error)
	DeleteBackend(req *scwlb.ZonedAPIDeleteBackendRequest, opts ...scw.RequestOption) error
	SetBackendServers(req *scwlb.ZonedAPISetBackendServersRequest, opts ...scw.RequestOption) (*scwlb.Backend, error)
	UpdateHealthCheck(req *scwlb.ZonedAPIUpdateHealthCheckRequest, opts ...scw.RequestOption) (*scwlb.HealthCheck, error)
	ListFrontends(req *scwlb.ZonedAPIListFrontendsRequest, opts ...scw.RequestOption) (*scwlb.ListFrontendsResponse, error)
	CreateFrontend(req *scwlb.ZonedAPICreateFrontendRequest, opts ...scw.RequestOption) (*scwlb.Frontend, error)
	UpdateFrontend(req *scwlb.ZonedAPIUpdateFrontendRequest, opts ...scw.RequestOption) (*scwlb.Frontend, error)
	DeleteFrontend(req *scwlb.ZonedAPIDeleteFrontendRequest, opts ...scw.RequestOption) error
	ListACLs(req *scwlb.ZonedAPIListACLsRequest, opts ...scw.RequestOption) (*scwlb.ListACLResponse, error)
	CreateACL(req *scwlb.ZonedAPICreateACLRequest, opts ...scw.RequestOption) (*scwlb.ACL, error)
	DeleteACL(req *scwlb.ZonedAPIDeleteACLRequest, opts ...scw.RequestOption) error
	UpdateACL(req *scwlb.ZonedAPIUpdateACLRequest, opts ...scw.RequestOption) (*scwlb.ACL, error)
	SetACLs(req *scwlb.ZonedAPISetACLsRequest, opts ...scw.RequestOption) (*scwlb.SetACLsResponse, error)
	ListLBPrivateNetworks(req *scwlb.ZonedAPIListLBPrivateNetworksRequest, opts ...scw.RequestOption) (*scwlb.ListLBPrivateNetworksResponse, error)
	AttachPrivateNetwork(req *scwlb.ZonedAPIAttachPrivateNetworkRequest, opts ...scw.RequestOption) (*scwlb.PrivateNetwork, error)
	DetachPrivateNetwork(req *scwlb.ZonedAPIDetachPrivateNetworkRequest, opts ...scw.RequestOption) error
}

func newLoadbalancers(client *client, defaultLBType, pnID string) *loadbalancers {
	lbType := "lb-s"
	if defaultLBType != "" {
		lbType = strings.ToLower(defaultLBType)
	}
	return &loadbalancers{
		api:           scwlb.NewZonedAPI(client.scaleway),
		ipam:          scwipam.NewAPI(client.scaleway),
		vpc:           scwvpc.NewAPI(client.scaleway),
		client:        client,
		defaultLBType: lbType,
		pnID:          pnID,
	}
}

// GetLoadBalancer returns whether the specified load balancer exists, and
// if so, what its status is.
// Implementations must treat the *v1.Service parameter as read-only and not modify it.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (l *loadbalancers) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*v1.LoadBalancerStatus, bool, error) {
	if service.Spec.LoadBalancerClass != nil {
		return nil, false, nil
	}

	lb, err := l.fetchLoadBalancer(ctx, clusterName, service)
	if err != nil {
		if err == LoadBalancerNotFound {
			klog.Infof("no load balancer found for service %s/%s", service.Namespace, service.Name)
			return nil, false, nil
		}

		klog.Errorf("error getting load balancer for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil, false, err
	}

	status, err := l.createServiceStatus(service, lb)
	if err != nil {
		klog.Errorf("error getting loadbalancer status for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil, true, err
	}
	return status, true, nil
}

// GetLoadBalancerName returns the name of the load balancer. Implementations must treat the
// *v1.Service parameter as read-only and not modify it.
func (l *loadbalancers) GetLoadBalancerName(ctx context.Context, clusterName string, service *v1.Service) string {
	loadbalancerPrefix := os.Getenv(scwCcmPrefixEnv)
	kubelbName := string(service.UID)

	return loadbalancerPrefix + kubelbName
}

// EnsureLoadBalancer creates a new load balancer 'name', or updates the existing one. Returns the status of the balancer
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (l *loadbalancers) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	if service.Spec.LoadBalancerClass != nil {
		return nil, fmt.Errorf("scaleway-cloud-controller-manager cannot handle loadBalancerClass %s", *service.Spec.LoadBalancerClass)
	}

	lbExternallyManaged, err := svcExternallyManaged(service)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerExternallyManaged)
		return nil, fmt.Errorf("invalid value for annotation %s: expected boolean", serviceAnnotationLoadBalancerExternallyManaged)
	}

	lbPrivate, err := svcPrivate(service)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerPrivate)
		return nil, fmt.Errorf("invalid value for annotation %s: expected boolean", serviceAnnotationLoadBalancerPrivate)
	}

	if lbPrivate && l.pnID == "" {
		return nil, fmt.Errorf("scaleway-cloud-controller-manager cannot create private load balancers without a private network")
	}

	if lbPrivate && hasLoadBalancerStaticIPs(service) {
		return nil, fmt.Errorf("scaleway-cloud-controller-manager can only handle static IPs for public load balancers. Unsetting the static IP can result in the loss of the IP")
	}

	lb, err := l.fetchLoadBalancer(ctx, clusterName, service)
	switch err {
	case nil:
		// continue
	case LoadBalancerNotFound:
		// create LoadBalancer
		lb, err = l.createLoadBalancer(ctx, clusterName, service)
		if err != nil {
			return nil, err
		}
	default:
		// any other kind of Error
		klog.Errorf("error getting loadbalancer for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil, err
	}

	if !lbExternallyManaged {
		privateModeMismatch := lbPrivate != (len(lb.IP) == 0)
		reservedIPMismatch := hasLoadBalancerStaticIPs(service) && !hasEqualLoadBalancerStaticIPs(service, lb)
		if privateModeMismatch || reservedIPMismatch {
			err = l.deleteLoadBalancer(ctx, lb, clusterName, service)
			if err != nil {
				return nil, err
			}

			lb, err = l.createLoadBalancer(ctx, clusterName, service)
			if err != nil {
				return nil, err
			}
		}
	}

	if lb.Status != scwlb.LBStatusReady {
		return nil, LoadBalancerNotReady
	}

	err = l.updateLoadBalancer(ctx, lb, service, nodes)
	if err != nil {
		klog.Errorf("error updating loadbalancer for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil, err
	}

	status, err := l.createServiceStatus(service, lb)
	if err != nil {
		klog.Errorf("error making loadbalancer status for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil, err
	}

	return status, nil
}

// UpdateLoadBalancer updates hosts under the specified load balancer.
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (l *loadbalancers) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) error {
	if service.Spec.LoadBalancerClass != nil {
		return fmt.Errorf("scaleway-cloud-controller-manager cannot handle loadBalancerClas %s", *service.Spec.LoadBalancerClass)
	}

	lb, err := l.fetchLoadBalancer(ctx, clusterName, service)
	if err != nil {
		klog.Errorf("error getting loadbalancer for service %s: %v", service.Name, err)
		return err
	}

	err = l.updateLoadBalancer(ctx, lb, service, nodes)
	if err != nil {
		klog.Errorf("error updating loadbalancer for service %s: %v", service.Name, err)
		return err
	}

	return nil
}

// EnsureLoadBalancerDeleted deletes the specified load balancer if it
// exists, returning nil if the load balancer specified either didn't exist or
// was successfully deleted.
// Implementations must treat the *v1.Service parameter as read-only and not modify it.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (l *loadbalancers) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	if service.Spec.LoadBalancerClass != nil {
		return nil
	}

	lb, err := l.fetchLoadBalancer(ctx, clusterName, service)
	if err != nil {
		if err == LoadBalancerNotFound {
			return nil
		}

		klog.Errorf("error getting loadbalancer for service %s: %v", service.Name, err)
		return err
	}

	lbExternallyManaged, err := svcExternallyManaged(service)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerExternallyManaged)
		return fmt.Errorf("invalid value for annotation %s: expected boolean", serviceAnnotationLoadBalancerExternallyManaged)
	}

	if lbExternallyManaged {
		return l.removeExternallyManagedResources(ctx, lb, service)
	}

	return l.deleteLoadBalancer(ctx, lb, clusterName, service)
}

func (l *loadbalancers) removeExternallyManagedResources(ctx context.Context, lb *scwlb.LB, service *v1.Service) error {
	prefixFilter := fmt.Sprintf("%s_", string(service.UID))

	// List all frontends associated with the LB
	respFrontends, err := l.api.ListFrontends(&scwlb.ZonedAPIListFrontendsRequest{
		Name: &prefixFilter,
		Zone: lb.Zone,
		LBID: lb.ID,
	}, scw.WithAllPages())
	if err != nil {
		return fmt.Errorf("error listing frontends for load balancer %s: %v", lb.ID, err)
	}

	// List all backends associated with the LB
	respBackends, err := l.api.ListBackends(&scwlb.ZonedAPIListBackendsRequest{
		Name: &prefixFilter,
		Zone: lb.Zone,
		LBID: lb.ID,
	}, scw.WithAllPages())
	if err != nil {
		return fmt.Errorf("error listing backend for load balancer %s: %v", lb.ID, err)
	}

	// Remove extra frontends
	for _, f := range respFrontends.Frontends {
		klog.V(3).Infof("deleting frontend: %s port: %d loadbalancer: %s", f.ID, f.InboundPort, lb.ID)
		if err := l.api.DeleteFrontend(&scwlb.ZonedAPIDeleteFrontendRequest{
			Zone:       lb.Zone,
			FrontendID: f.ID,
		}); err != nil {
			return fmt.Errorf("failed deleting frontend: %s port: %d loadbalancer: %s err: %v", f.ID, f.InboundPort, lb.ID, err)
		}
	}

	// Remove extra backends
	for _, b := range respBackends.Backends {
		klog.V(3).Infof("deleting backend: %s port: %d loadbalancer: %s", b.ID, b.ForwardPort, lb.ID)
		if err := l.api.DeleteBackend(&scwlb.ZonedAPIDeleteBackendRequest{
			Zone:      lb.Zone,
			BackendID: b.ID,
		}); err != nil {
			return fmt.Errorf("failed deleting backend: %s port: %d loadbalancer: %s err: %v", b.ID, b.ForwardPort, lb.ID, err)
		}
	}

	return nil
}

func (l *loadbalancers) deleteLoadBalancer(ctx context.Context, lb *scwlb.LB, clusterName string, service *v1.Service) error {
	// remove loadbalancer annotation
	if err := l.unannotateAndPatch(service); err != nil {
		return err
	}

	// if loadbalancer is renamed, do not delete it.
	if lb.Name != l.GetLoadBalancerName(ctx, clusterName, service) {
		klog.Warningf("load balancer for service %s/%s was renamed, not removing", service.Namespace, service.Name)
		return nil
	}

	request := &scwlb.ZonedAPIDeleteLBRequest{
		Zone:      lb.Zone,
		LBID:      lb.ID,
		ReleaseIP: !hasLoadBalancerStaticIPs(service), // if no static IP is set, it implies an ephemeral IP
	}

	err := l.api.DeleteLB(request)
	if err != nil {
		klog.Errorf("error deleting load balancer %s for service %s/%s: %v", lb.ID, service.Namespace, service.Name, err)
		return fmt.Errorf("error deleting load balancer %s for service %s/%s: %v", lb.ID, service.Namespace, service.Name, err)
	}

	return nil
}

// get the nodes ip addresses
// for a given service return the list of associated node ip
// as we need to loadbalance via public nat addresses we use the NodeExternalIP
// the NodeExternalIP are provider by server.go which set the public ip on k8s nodes
func extractNodesExternalIps(nodes []*v1.Node) []string {
	var nodesList []string
	for _, node := range nodes {
		for _, address := range node.Status.Addresses {
			if address.Type == v1.NodeExternalIP {
				nodesList = append(nodesList, address.Address)
			}
		}
	}

	return nodesList
}

// get the internal nodes ip addresses
func extractNodesInternalIps(nodes []*v1.Node) []string {
	var nodesList []string
	for _, node := range nodes {
		for _, address := range node.Status.Addresses {
			if address.Type == v1.NodeInternalIP {
				nodesList = append(nodesList, address.Address)
			}
		}
	}

	return nodesList
}

func (l *loadbalancers) fetchLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*scwlb.LB, error) {
	lbExternallyManaged, err := svcExternallyManaged(service)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerExternallyManaged)
		return nil, fmt.Errorf("invalid value for annotation %s: %v", serviceAnnotationLoadBalancerExternallyManaged, err)
	}

	zone, loadBalancerID, err := getLoadBalancerID(service)
	if err != nil && !errors.Is(err, errLoadBalancerInvalidAnnotation) {
		return nil, err
	}

	if lbExternallyManaged && loadBalancerID == "" {
		return nil, fmt.Errorf("loadbalancer id must be defined for externally managed loadbalancer for service %s/%s", service.Namespace, service.Name)
	}

	if loadBalancerID != "" {
		resp, err := l.api.GetLB(&scwlb.ZonedAPIGetLBRequest{
			LBID: loadBalancerID,
			Zone: zone,
		})
		if err != nil {
			if is404Error(err) {
				return nil, LoadBalancerNotFound
			}
			klog.Errorf("an error occurred while fetching loadbalancer '%s/%s' for service '%s/%s'", zone, loadBalancerID, service.Namespace, service.Name)
			return nil, err
		}

		if lbExternallyManaged && strings.HasPrefix(resp.Name, os.Getenv(scwCcmPrefixEnv)) {
			klog.Errorf("externally managed loadbalancer must not be prefixed by the cluster id")
			return nil, fmt.Errorf("externally managed loadbalancer must not be prefixed by the cluster id")
		}

		return resp, nil
	}

	// fallback to fetching LoadBalancer by name
	return l.getLoadbalancerByName(ctx, clusterName, service)
}

func (l *loadbalancers) getLoadbalancerByName(ctx context.Context, clusterName string, service *v1.Service) (*scwlb.LB, error) {
	name := l.GetLoadBalancerName(ctx, clusterName, service)

	var loadbalancer *scwlb.LB
	resp, err := l.api.ListLBs(&scwlb.ZonedAPIListLBsRequest{
		Name: &name,
		Zone: getLoadBalancerZone(service),
	}, scw.WithAllPages())
	if err != nil {
		return nil, err
	}

	for _, lb := range resp.LBs {
		if lb.Name == name {
			if loadbalancer != nil {
				klog.Errorf("more than one loadbalancing matching the name %s", name)
				return nil, LoadBalancerDuplicated
			}

			loadbalancer = lb
		}
	}

	if loadbalancer == nil {
		klog.Infof("no loadbalancer matching the name %s", name)
		return nil, LoadBalancerNotFound
	}

	// annotate existing loadBalancer
	if err := l.annotateAndPatch(service, loadbalancer); err != nil {
		return nil, err
	}

	return loadbalancer, nil
}

func (l *loadbalancers) createLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (*scwlb.LB, error) {
	// ExternallyManaged LB
	lbExternallyManaged, err := svcExternallyManaged(service)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerExternallyManaged)
		return nil, fmt.Errorf("invalid value for annotation %s: expected boolean", serviceAnnotationLoadBalancerExternallyManaged)
	}
	if lbExternallyManaged {
		return nil, fmt.Errorf("cannot create an externally managed load balancer, please provide an existing load balancer instead")
	}

	lbPrivate, err := svcPrivate(service)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerPrivate)
		return nil, fmt.Errorf("invalid value for annotation %s: expected boolean", serviceAnnotationLoadBalancerPrivate)
	}

	sslCompatibilityLevel, err := getSSLCompatibilityLevel(service)
	if err != nil {
		klog.Errorf("error getting SSL compatibility level for service %s(%s): %v", service.Name, service.UID, err)
		return nil, fmt.Errorf("error getting SSL compatibility level for service %s(%s): %v", service.Name, service.UID, err)
	}

	// Attach specific IP if set
	var ipIDs []string
	if !lbPrivate {
		ipIDs, err = l.getLoadBalancerStaticIPIDs(service)
		if err != nil {
			return nil, err
		}
	}

	lbName := l.GetLoadBalancerName(ctx, clusterName, service)
	lbType := getLoadBalancerType(service)
	if lbType == "" {
		lbType = l.defaultLBType
	}
	scwCcmTagsDelimiter := os.Getenv(scwCcmTagsDelimiterEnv)
	if scwCcmTagsDelimiter == "" {
		scwCcmTagsDelimiter = ","
	}
	scwCcmTags := os.Getenv(scwCcmTagsEnv)
	tags := []string{}
	if scwCcmTags != "" {
		tags = strings.Split(scwCcmTags, scwCcmTagsDelimiter)
	}
	tags = append(tags, "managed-by-scaleway-cloud-controller-manager")

	request := scwlb.ZonedAPICreateLBRequest{
		Zone:        getLoadBalancerZone(service),
		Name:        lbName,
		Description: "kubernetes service " + service.Name,
		Tags:        tags,
		IPIDs:       ipIDs,
		Type:        lbType,
		// We must only assign a flexible IP if LB is public AND no IP ID is provided.
		// If IP IDs are provided, there must be at least one IPv4.
		AssignFlexibleIP:      scw.BoolPtr(!lbPrivate && len(ipIDs) == 0),
		SslCompatibilityLevel: sslCompatibilityLevel,
	}
	lb, err := l.api.CreateLB(&request)
	if err != nil {
		klog.Errorf("error creating load balancer for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil, fmt.Errorf("error creating load balancer for service %s/%s: %v", service.Namespace, service.Name, err)
	}

	// annotate newly created load balancer
	if err := l.annotateAndPatch(service, lb); err != nil {
		return nil, err
	}

	return lb, nil
}

// getLoadBalancerStaticIPIDs returns user-provided static IPs for the LB from annotations.
// If no annotation is found, it uses the LoadBalancerIP field from service spec.
// It returns nil if user provided no static IP. In this case, the CCM must manage a dynamic IP.
func (l *loadbalancers) getLoadBalancerStaticIPIDs(service *v1.Service) ([]string, error) {
	if ipIDs := getIPIDs(service); len(ipIDs) > 0 {
		return ipIDs, nil
	}

	if service.Spec.LoadBalancerIP != "" {
		ipsResp, err := l.api.ListIPs(&scwlb.ZonedAPIListIPsRequest{
			IPAddress: &service.Spec.LoadBalancerIP,
			Zone:      getLoadBalancerZone(service),
		})
		if err != nil {
			klog.Errorf("error getting ip for service %s/%s: %v", service.Namespace, service.Name, err)
			return nil, fmt.Errorf("createLoadBalancer: error getting ip for service %s: %s", service.Name, err.Error())
		}

		if len(ipsResp.IPs) == 0 {
			return nil, IPAddressNotFound
		}

		if ipsResp.IPs[0].LBID != nil && *ipsResp.IPs[0].LBID != "" {
			return nil, IPAddressInUse
		}

		return []string{ipsResp.IPs[0].ID}, nil
	}

	return nil, nil
}

// annotateAndPatch adds the loadbalancer id to the service's annotations
func (l *loadbalancers) annotateAndPatch(service *v1.Service, loadbalancer *scwlb.LB) error {
	service = service.DeepCopy()
	patcher := NewServicePatcher(l.client.kubernetes, service)

	if service.ObjectMeta.Annotations == nil {
		service.ObjectMeta.Annotations = map[string]string{}
	}
	service.ObjectMeta.Annotations[serviceAnnotationLoadBalancerID] = loadbalancer.Zone.String() + "/" + loadbalancer.ID

	return patcher.Patch()
}

// unannotateAndPatch removes the loadbalancer id from the service's annotations
func (l *loadbalancers) unannotateAndPatch(service *v1.Service) error {
	service = service.DeepCopy()
	patcher := NewServicePatcher(l.client.kubernetes, service)

	if service.ObjectMeta.Annotations != nil {
		delete(service.ObjectMeta.Annotations, serviceAnnotationLoadBalancerID)
	}

	return patcher.Patch()
}

// updateLoadBalancer updates the loadbalancer's resources
func (l *loadbalancers) updateLoadBalancer(ctx context.Context, loadbalancer *scwlb.LB, service *v1.Service, nodes []*v1.Node) error {
	// Skip update if the service is being deleted
	if service.ObjectMeta.DeletionTimestamp != nil {
		klog.V(3).Infof("skipping loadbalancer update for service %s/%s: service is being deleted", service.Namespace, service.Name)
		return nil
	}

	lbExternallyManaged, err := svcExternallyManaged(service)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerExternallyManaged)
		return fmt.Errorf("invalid value for annotation %s: expected boolean", serviceAnnotationLoadBalancerExternallyManaged)
	}

	nodes = filterNodes(service, nodes)
	if err := nodesInitialized(nodes); err != nil {
		return err
	}

	if err := l.attachPrivateNetworks(loadbalancer, service, lbExternallyManaged); err != nil {
		return fmt.Errorf("failed to attach private networks: %w", err)
	}

	var targetIPs []string
	if getForceInternalIP(service) || l.pnID != "" || len(getPrivateNetworkIDs(service)) > 0 {
		targetIPs = extractNodesInternalIps(nodes)
		klog.V(3).Infof("using internal nodes ips: %s on loadbalancer %s", strings.Join(targetIPs, ","), loadbalancer.ID)
	} else {
		targetIPs = extractNodesExternalIps(nodes)
		klog.V(3).Infof("using external nodes ips: %s on loadbalancer %s", strings.Join(targetIPs, ","), loadbalancer.ID)
	}

	// List all frontends associated with the LB
	respFrontends, err := l.api.ListFrontends(&scwlb.ZonedAPIListFrontendsRequest{
		Zone: loadbalancer.Zone,
		LBID: loadbalancer.ID,
	}, scw.WithAllPages())
	if err != nil {
		return fmt.Errorf("error listing frontends for load balancer %s: %v", loadbalancer.ID, err)
	}

	// List all backends associated with the LB
	respBackends, err := l.api.ListBackends(&scwlb.ZonedAPIListBackendsRequest{
		Zone: loadbalancer.Zone,
		LBID: loadbalancer.ID,
	}, scw.WithAllPages())
	if err != nil {
		return fmt.Errorf("error listing backend for load balancer %s: %v", loadbalancer.ID, err)
	}

	svcFrontends, svcBackends, err := serviceToLB(service, loadbalancer, targetIPs)
	if err != nil {
		return fmt.Errorf("failed to convert service to frontend and backends on loadbalancer %s: %v", loadbalancer.ID, err)
	}

	prefixFilter := ""
	if lbExternallyManaged {
		prefixFilter = fmt.Sprintf("%s_", string(service.UID))
	}

	frontendsOps := compareFrontends(respFrontends.Frontends, svcFrontends, prefixFilter)
	backendsOps := compareBackends(respBackends.Backends, svcBackends, prefixFilter)

	// Remove extra frontends
	for _, f := range frontendsOps.remove {
		klog.V(3).Infof("deleting frontend: %s port: %d loadbalancer: %s", f.ID, f.InboundPort, loadbalancer.ID)
		if err := l.api.DeleteFrontend(&scwlb.ZonedAPIDeleteFrontendRequest{
			Zone:       loadbalancer.Zone,
			FrontendID: f.ID,
		}); err != nil {
			return fmt.Errorf("failed deleting frontend: %s port: %d loadbalancer: %s err: %v", f.ID, f.InboundPort, loadbalancer.ID, err)
		}
	}

	// Remove extra backends
	for _, b := range backendsOps.remove {
		klog.V(3).Infof("deleting backend: %s port: %d loadbalancer: %s", b.ID, b.ForwardPort, loadbalancer.ID)
		if err := l.api.DeleteBackend(&scwlb.ZonedAPIDeleteBackendRequest{
			Zone:      loadbalancer.Zone,
			BackendID: b.ID,
		}); err != nil {
			return fmt.Errorf("failed deleting backend: %s port: %d loadbalancer: %s err: %v", b.ID, b.ForwardPort, loadbalancer.ID, err)
		}
	}

	for _, port := range service.Spec.Ports {
		var backend *scwlb.Backend
		// Update backend
		if b, ok := backendsOps.update[port.NodePort]; ok {
			klog.V(3).Infof("update backend: %s port: %d loadbalancer: %s", b.ID, b.ForwardPort, loadbalancer.ID)
			updatedBackend, err := l.updateBackend(service, loadbalancer, b)
			if err != nil {
				return fmt.Errorf("failed updating backend %s port: %d loadbalancer: %s err: %v", b.ID, b.ForwardPort, loadbalancer.ID, err)
			}
			backend = updatedBackend
		}
		// Create backend
		if b, ok := backendsOps.create[port.NodePort]; ok {
			klog.V(3).Infof("create backend port: %d loadbalancer: %s", b.ForwardPort, loadbalancer.ID)
			createdBackend, err := l.createBackend(service, loadbalancer, b)
			if err != nil {
				return fmt.Errorf("failed creating backend port: %d loadbalancer: %s err: %v", b.ForwardPort, loadbalancer.ID, err)
			}
			backend = createdBackend
		}

		if backend == nil {
			b, ok := backendsOps.keep[port.NodePort]
			if !ok {
				return fmt.Errorf("undefined backend port: %d loadbalancer: %s", port.NodePort, loadbalancer.ID)
			}
			backend = b
		}

		// Update backend servers
		if !stringArrayEqual(backend.Pool, targetIPs) {
			// Safety: refuse to clear existing backends when no replacement IPs are found
			if len(targetIPs) == 0 && len(backend.Pool) > 0 {
				klog.Warningf("refusing to clear backend pool for backend %s on loadbalancer %s — keeping %d existing servers",
					backend.ID, loadbalancer.ID, len(backend.Pool))
				continue
			}

			klog.V(3).Infof("update server list for backend: %s port: %d loadbalancer: %s", backend.ID, port.NodePort, loadbalancer.ID)
			if _, err := l.api.SetBackendServers(&scwlb.ZonedAPISetBackendServersRequest{
				Zone:      loadbalancer.Zone,
				BackendID: backend.ID,
				ServerIP:  targetIPs,
			}); err != nil {
				return fmt.Errorf("failed updating server list for backend: %s port: %d loadbalancer: %s err: %v", backend.ID, port.NodePort, loadbalancer.ID, err)
			}
		}

		var frontend *scwlb.Frontend
		// Update frontend
		if f, ok := frontendsOps.update[port.Port]; ok {
			klog.V(3).Infof("update frontend: %s port: %d loadbalancer: %s", f.ID, port.Port, loadbalancer.ID)
			ff, err := l.updateFrontend(service, loadbalancer, f, backend)
			if err != nil {
				return fmt.Errorf("failed updating frontend: %s port: %d loadbalancer: %s err: %v", f.ID, port.Port, loadbalancer.ID, err)
			}
			frontend = ff
		}
		// Create frontend
		if f, ok := frontendsOps.create[port.Port]; ok {
			klog.V(3).Infof("create frontend port: %d loadbalancer: %s", port.Port, loadbalancer.ID)
			ff, err := l.createFrontend(service, loadbalancer, f, backend)
			if err != nil {
				return fmt.Errorf("failed creating frontend port: %d loadbalancer: %s err: %v", port.Port, loadbalancer.ID, err)
			}
			frontend = ff
		}

		if frontend == nil {
			f, ok := frontendsOps.keep[port.Port]
			if !ok {
				return fmt.Errorf("undefined frontend port: %d loadbalancer: %s", port.Port, loadbalancer.ID)
			}
			frontend = f
		}

		// List ACLs for the frontend
		aclName := makeACLPrefix(frontend)
		aclsResp, err := l.api.ListACLs(&scwlb.ZonedAPIListACLsRequest{
			Zone:       loadbalancer.Zone,
			FrontendID: frontend.ID,
			Name:       &aclName,
		}, scw.WithAllPages())
		if err != nil {
			return fmt.Errorf("failed to list ACLs for frontend: %s port: %d loadbalancer: %s err: %v", frontend.ID, frontend.InboundPort, loadbalancer.ID, err)
		}

		svcAcls := makeACLSpecs(service, nodes, frontend)
		if !aclsEquals(aclsResp.ACLs, svcAcls) {
			// Replace ACLs
			klog.Infof("remove all ACLs from frontend: %s port: %d loadbalancer: %s", frontend.ID, frontend.InboundPort, loadbalancer.ID)
			for _, acl := range aclsResp.ACLs {
				if err := l.api.DeleteACL(&scwlb.ZonedAPIDeleteACLRequest{
					Zone:  loadbalancer.Zone,
					ACLID: acl.ID,
				}); err != nil {
					return fmt.Errorf("failed removing ACL %s from frontend: %s port: %d loadbalancer: %s err: %v", acl.Name, frontend.ID, frontend.InboundPort, loadbalancer.ID, err)
				}
			}

			klog.Infof("create all ACLs for frontend: %s port: %d loadbalancer: %s", frontend.ID, frontend.InboundPort, loadbalancer.ID)
			for _, acl := range svcAcls {
				if _, err := l.api.SetACLs(&scwlb.ZonedAPISetACLsRequest{
					Zone:       loadbalancer.Zone,
					FrontendID: frontend.ID,
					ACLs:       svcAcls,
				}); err != nil {
					return fmt.Errorf("failed creating ACL %s for frontend: %s port: %d loadbalancer: %s err: %v", acl.Name, frontend.ID, frontend.InboundPort, loadbalancer.ID, err)
				}
			}
		}
	}

	if !lbExternallyManaged {
		loadBalancerType := getLoadBalancerType(service)
		if loadBalancerType != "" && strings.ToLower(loadbalancer.Type) != loadBalancerType {
			_, err := l.api.MigrateLB(&scwlb.ZonedAPIMigrateLBRequest{
				Zone: loadbalancer.Zone,
				LBID: loadbalancer.ID,
				Type: loadBalancerType,
			})
			if err != nil {
				klog.Errorf("error updating load balancer %s: %v", loadbalancer.ID, err)
				return fmt.Errorf("error updating load balancer %s: %v", loadbalancer.ID, err)
			}
		}

		// Update SSL compatibility level if needed
		sslCompatibilityLevel, err := getSSLCompatibilityLevel(service)
		if err != nil {
			klog.Errorf("error getting SSL compatibility level on the service %s for load balancer %s: %v", service.Name, loadbalancer.ID, err)
			return fmt.Errorf("error getting SSL compatibility level on the service %s for load balancer %s: %v", service.Name, loadbalancer.ID, err)
		}
		if loadbalancer.SslCompatibilityLevel != sslCompatibilityLevel {
			_, err := l.api.UpdateLB(&scwlb.ZonedAPIUpdateLBRequest{
				Zone:                  loadbalancer.Zone,
				LBID:                  loadbalancer.ID,
				Name:                  loadbalancer.Name,
				Description:           loadbalancer.Description,
				Tags:                  loadbalancer.Tags,
				SslCompatibilityLevel: sslCompatibilityLevel,
			})
			if err != nil {
				klog.Errorf("error updating load balancer %s: %v", loadbalancer.ID, err)
				return fmt.Errorf("error updating load balancer %s: %v", loadbalancer.ID, err)
			}
		}
	}

	return nil
}

func (l *loadbalancers) attachPrivateNetworks(loadbalancer *scwlb.LB, service *v1.Service, lbExternallyManaged bool) error {
	if l.pnID == "" {
		return nil
	}

	// maps pnID => attached
	pnIDs := make(map[string]bool)

	// Fetch user-specified PrivateNetworkIDs unless LB is externally managed.
	if !lbExternallyManaged {
		for _, pnID := range getPrivateNetworkIDs(service) {
			pnIDs[pnID] = false
		}
	}

	if len(pnIDs) == 0 {
		pnIDs[l.pnID] = false
	}

	respPN, err := l.api.ListLBPrivateNetworks(&scwlb.ZonedAPIListLBPrivateNetworksRequest{
		Zone: loadbalancer.Zone,
		LBID: loadbalancer.ID,
	})
	if err != nil {
		return fmt.Errorf("error listing private networks of load balancer %s: %v", loadbalancer.ID, err)
	}

	for _, pNIC := range respPN.PrivateNetwork {
		if _, ok := pnIDs[pNIC.PrivateNetworkID]; ok {
			// Mark this Private Network as attached.
			pnIDs[pNIC.PrivateNetworkID] = true
			continue
		}

		// this PN should not be attached to this loadbalancer
		if !lbExternallyManaged {
			klog.V(3).Infof("detach extra private network %s from load balancer %s", pNIC.PrivateNetworkID, loadbalancer.ID)
			err = l.api.DetachPrivateNetwork(&scwlb.ZonedAPIDetachPrivateNetworkRequest{
				Zone:             loadbalancer.Zone,
				LBID:             loadbalancer.ID,
				PrivateNetworkID: pNIC.PrivateNetworkID,
			})
			if err != nil {
				return fmt.Errorf("unable to detach unmatched private network %s from %s: %v", pNIC.PrivateNetworkID, loadbalancer.ID, err)
			}
		}
	}

	for pnID, attached := range pnIDs {
		if attached {
			continue
		}

		klog.V(3).Infof("attach private network %s to load balancer %s", pnID, loadbalancer.ID)
		_, err = l.api.AttachPrivateNetwork(&scwlb.ZonedAPIAttachPrivateNetworkRequest{
			Zone:             loadbalancer.Zone,
			LBID:             loadbalancer.ID,
			PrivateNetworkID: pnID,
			DHCPConfig:       &scwlb.PrivateNetworkDHCPConfig{},
		})
		if err != nil {
			return fmt.Errorf("unable to attach private network %s on %s: %v", pnID, loadbalancer.ID, err)
		}
	}

	return nil
}

// createPrivateServiceStatus creates a LoadBalancer status for services with private load balancers
func (l *loadbalancers) createPrivateServiceStatus(service *v1.Service, lb *scwlb.LB, ipMode *v1.LoadBalancerIPMode) (*v1.LoadBalancerStatus, error) {
	if l.pnID == "" {
		return nil, fmt.Errorf("cannot create a status for service %s/%s: a private load balancer requires a private network", service.Namespace, service.Name)
	}

	region, err := lb.Zone.Region()
	if err != nil {
		return nil, fmt.Errorf("error creating status for service %s/%s: %v", service.Namespace, service.Name, err)
	}

	status := &v1.LoadBalancerStatus{}

	if getUseHostname(service) {
		pn, err := l.vpc.GetPrivateNetwork(&scwvpc.GetPrivateNetworkRequest{
			Region:           region,
			PrivateNetworkID: l.pnID,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to query private network for lb %s for service %s/%s: %v", lb.ID, service.Namespace, service.Name, err)
		}

		status.Ingress = []v1.LoadBalancerIngress{
			{
				Hostname: fmt.Sprintf("%s.%s", lb.ID, pn.Name),
			},
		}
	} else {
		ipamRes, err := l.ipam.ListIPs(&scwipam.ListIPsRequest{
			ProjectID:    &lb.ProjectID,
			ResourceType: scwipam.ResourceTypeLBServer,
			ResourceID:   &lb.ID,
			IsIPv6:       scw.BoolPtr(false),
			Region:       region,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to query ipam for lb %s for service %s/%s: %v", lb.ID, service.Namespace, service.Name, err)
		}

		if len(ipamRes.IPs) == 0 {
			return nil, fmt.Errorf("no private network ip for lb %s for service %s/%s", lb.ID, service.Namespace, service.Name)
		}

		status.Ingress = make([]v1.LoadBalancerIngress, len(ipamRes.IPs))
		for idx, ip := range ipamRes.IPs {
			status.Ingress[idx].IP = ip.Address.IP.String()
			status.Ingress[idx].IPMode = ipMode
		}
	}

	return status, nil
}

// createPublicServiceStatus creates a LoadBalancer status for services with public load balancers
func (l *loadbalancers) createPublicServiceStatus(service *v1.Service, lb *scwlb.LB, ipMode *v1.LoadBalancerIPMode) (*v1.LoadBalancerStatus, error) {
	status := &v1.LoadBalancerStatus{}
	status.Ingress = make([]v1.LoadBalancerIngress, 0)
	for _, ip := range lb.IP {
		if getUseHostname(service) {
			status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{Hostname: ip.Reverse})
		} else {
			status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: ip.IPAddress, IPMode: ipMode})
		}
	}

	if len(status.Ingress) == 0 {
		return nil, fmt.Errorf("no ip found for lb %s", lb.Name)
	}

	return status, nil
}

// createServiceStatus creates a LoadBalancer status for the service
func (l *loadbalancers) createServiceStatus(service *v1.Service, lb *scwlb.LB) (*v1.LoadBalancerStatus, error) {
	lbPrivate, err := svcPrivate(service)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerPrivate)
		return nil, fmt.Errorf("invalid value for annotation %s: expected boolean", serviceAnnotationLoadBalancerPrivate)
	}

	ipMode, err := ipMode(service)
	if err != nil {
		return nil, err
	}

	if lbPrivate {
		return l.createPrivateServiceStatus(service, lb, ipMode)
	}

	return l.createPublicServiceStatus(service, lb, ipMode)
}

// ipMode returns the LoadBalancer IP Mode to use for the service.
// It returns Proxy when there is at least one port set on the service
// and ALL the ports are configured with proxy protocol. Otherwise, it
// returns VIP.
// The user can set the "service.beta.kubernetes.io/scw-loadbalancer-ip-mode" annotation
// to bypass the previous logic and override the ipMode.
func ipMode(service *v1.Service) (*v1.LoadBalancerIPMode, error) {
	// If the user did set the IPMode value manually, we should use it.
	if ipMode := getLoadBalancerIPMode(service); ipMode != nil {
		return ipMode, nil
	}

	if len(service.Spec.Ports) == 0 {
		return ptr.To(v1.LoadBalancerIPModeVIP), nil
	}

	ppCount := 0
	for _, port := range service.Spec.Ports {
		proxyProtocol, err := getProxyProtocol(service, port.NodePort)
		if err != nil {
			return nil, err
		}

		if slices.Contains([]scwlb.ProxyProtocol{
			scwlb.ProxyProtocolProxyProtocolV1,
			scwlb.ProxyProtocolProxyProtocolV2,
		}, proxyProtocol) {
			ppCount++
		}
	}

	if ppCount == len(service.Spec.Ports) {
		return ptr.To(v1.LoadBalancerIPModeProxy), nil
	}

	return ptr.To(v1.LoadBalancerIPModeVIP), nil
}

func isPortInRange(r string, p int32) (bool, error) {
	boolValue, err := strconv.ParseBool(r)
	if err == nil && r != "1" && r != "0" {
		return boolValue, nil
	}
	if r == "*" {
		return true, nil
	}
	if r == "" {
		return false, nil
	}
	ports := strings.Split(strings.ReplaceAll(r, " ", ""), ",")
	for _, port := range ports {
		intPort, err := strconv.ParseInt(port, 0, 64)
		if err != nil {
			return false, err
		}
		// Validate port is within valid range (1-65535)
		if intPort < 1 || intPort > 65535 {
			return false, fmt.Errorf("port %d is outside valid range (1-65535)", intPort)
		}
		if int64(p) == intPort {
			return true, nil
		}
	}
	return false, nil
}

// filterNodes uses node labels to filter the nodes that should be targeted by the load balancer,
// checking if all the labels provided in an annotation are present in the nodes
//
// Original version: https://github.com/kubernetes/legacy-cloud-providers/blob/1aa918bf227e52af6f8feb3fa065dabff251a0a3/aws/aws_loadbalancer.go#L1631
func filterNodes(service *v1.Service, nodes []*v1.Node) []*v1.Node {
	nodeLabels, ok := service.Annotations[serviceAnnotationLoadBalancerTargetNodeLabels]
	if !ok {
		return nodes
	}

	targetNodeLabels := getKeyValueFromAnnotation(nodeLabels)

	if len(targetNodeLabels) == 0 {
		return nodes
	}

	targetNodes := make([]*v1.Node, 0, len(nodes))

	for _, node := range nodes {
		if node.Labels != nil && len(node.Labels) > 0 {
			allFiltersMatch := true

			for targetLabelKey, targetLabelValue := range targetNodeLabels {
				if nodeLabelValue, ok := node.Labels[targetLabelKey]; !ok || (nodeLabelValue != targetLabelValue && targetLabelValue != "") {
					allFiltersMatch = false
					break
				}
			}

			if allFiltersMatch {
				targetNodes = append(targetNodes, node)
			}
		}
	}

	return targetNodes
}

// servicePortToFrontend converts a specific port of a service definition to a load balancer frontend
func servicePortToFrontend(service *v1.Service, loadbalancer *scwlb.LB, port v1.ServicePort) (*scwlb.Frontend, error) {
	timeoutClient, err := getTimeoutClient(service)
	if err != nil {
		return nil, fmt.Errorf("error getting %s annotation for loadbalancer %s: %v",
			serviceAnnotationLoadBalancerTimeoutClient, loadbalancer.ID, err)
	}

	certificateIDs, err := getCertificateIDs(service, port.Port)
	if err != nil {
		return nil, fmt.Errorf("error getting certificate IDs for loadbalancer %s: %v", loadbalancer.ID, err)
	}

	connectionRateLimit, err := getConnectionRateLimit(service)
	if err != nil {
		return nil, fmt.Errorf("error getting %s annotation for loadbalancer %s: %v",
			serviceAnnotationLoadBalancerConnectionRateLimit, loadbalancer.ID, err)
	}

	enableAccessLogs, err := getEnableAccessLogs(service)
	if err != nil {
		return nil, fmt.Errorf("error getting %s annotation for loadbalancer %s: %v",
			serviceAnnotationLoadBalancerEnableAccessLogs, loadbalancer.ID, err)
	}

	enableHTTP3, err := getEnableHTTP3(service)
	if err != nil {
		return nil, fmt.Errorf("error getting %s annotation for loadbalancer %s: %v",
			serviceAnnotationLoadBalancerEnableHTTP3, loadbalancer.ID, err)
	}

	return &scwlb.Frontend{
		Name:                fmt.Sprintf("%s_tcp_%d", string(service.UID), port.Port),
		InboundPort:         port.Port,
		TimeoutClient:       &timeoutClient,
		ConnectionRateLimit: connectionRateLimit,
		CertificateIDs:      certificateIDs,
		EnableAccessLogs:    enableAccessLogs,
		EnableHTTP3:         enableHTTP3,
	}, nil
}

// servicePortToBackend converts a specific port of a service definition to a load balancer backend with the specified list of target nodes
func servicePortToBackend(service *v1.Service, loadbalancer *scwlb.LB, port v1.ServicePort, nodeIPs []string) (*scwlb.Backend, error) {
	protocol, err := getForwardProtocol(service, port.NodePort)
	if err != nil {
		return nil, err
	}

	sslBridging, err := getSSLBridging(service, port.NodePort)
	if err != nil {
		return nil, err
	}

	sslSkipVerify, err := getSSLBridgingSkipVerify(service, port.NodePort)
	if err != nil {
		return nil, err
	}

	forwardPortAlgorithm, err := getForwardPortAlgorithm(service)
	if err != nil {
		return nil, err
	}

	stickySessions, err := getStickySessions(service)
	if err != nil {
		return nil, err
	}

	proxyProtocol, err := getProxyProtocol(service, port.NodePort)
	if err != nil {
		return nil, err
	}

	timeoutServer, err := getTimeoutServer(service)
	if err != nil {
		return nil, err
	}

	timeoutConnect, err := getTimeoutConnect(service)
	if err != nil {
		return nil, err
	}

	timeoutTunnel, err := getTimeoutTunnel(service)
	if err != nil {
		return nil, err
	}

	timeoutQueue, err := getTimeoutQueue(service)
	if err != nil {
		return nil, err
	}

	onMarkedDownAction, err := getOnMarkedDownAction(service)
	if err != nil {
		return nil, err
	}

	redispatchAttemptCount, err := getRedisatchAttemptCount(service)
	if err != nil {
		return nil, err
	}

	maxConnections, err := getMaxConnections(service)
	if err != nil {
		return nil, err
	}

	maxRetries, err := getMaxRetries(service)
	if err != nil {
		return nil, err
	}

	failoverHost, err := getFailoverHost(service)
	if err != nil {
		return nil, err
	}

	healthCheck, err := getNativeHealthCheck(service, port.Port)
	if err != nil {
		return nil, err
	}

	if healthCheck == nil {
		healthCheck = &scwlb.HealthCheck{
			Port: port.NodePort,
		}

		healthCheckType, err := getHealthCheckType(service, port.NodePort)
		if err != nil {
			return nil, err
		}

		switch healthCheckType {
		case "mysql":
			hc, err := getMysqlHealthCheck(service, port.NodePort)
			if err != nil {
				return nil, err
			}
			healthCheck.MysqlConfig = hc
		case "ldap":
			hc, err := getLdapHealthCheck(service, port.NodePort)
			if err != nil {
				return nil, err
			}
			healthCheck.LdapConfig = hc
		case "redis":
			hc, err := getRedisHealthCheck(service, port.NodePort)
			if err != nil {
				return nil, err
			}
			healthCheck.RedisConfig = hc
		case "pgsql":
			hc, err := getPgsqlHealthCheck(service, port.NodePort)
			if err != nil {
				return nil, err
			}
			healthCheck.PgsqlConfig = hc
		case "tcp":
			hc, err := getTCPHealthCheck(service, port.NodePort)
			if err != nil {
				return nil, err
			}
			healthCheck.TCPConfig = hc
		case "http":
			hc, err := getHTTPHealthCheck(service, port.NodePort)
			if err != nil {
				return nil, err
			}
			healthCheck.HTTPConfig = hc
		case "https":
			hc, err := getHTTPSHealthCheck(service, port.NodePort)
			if err != nil {
				return nil, err
			}
			healthCheck.HTTPSConfig = hc
		default:
			klog.Errorf("wrong value for healthCheckType")
			return nil, errLoadBalancerInvalidAnnotation
		}

		healthCheckSendProxy, err := getHealthCheckSendProxy(service)
		if err != nil {
			return nil, err
		}
		healthCheck.CheckSendProxy = healthCheckSendProxy
	}

	healthCheckDelay, err := getHealthCheckDelay(service)
	if err != nil {
		return nil, err
	}
	healthCheck.CheckDelay = &healthCheckDelay

	healthCheckTimeout, err := getHealthCheckTimeout(service)
	if err != nil {
		return nil, err
	}
	healthCheck.CheckTimeout = &healthCheckTimeout

	healthCheckMaxRetries, err := getHealthCheckMaxRetries(service)
	if err != nil {
		return nil, err
	}
	healthCheck.CheckMaxRetries = healthCheckMaxRetries

	healthCheckTransientCheckDelay, err := getHealthCheckTransientCheckDelay(service)
	if err != nil {
		return nil, err
	}
	healthCheck.TransientCheckDelay = healthCheckTransientCheckDelay

	backend := &scwlb.Backend{
		Name:                   fmt.Sprintf("%s_tcp_%d", string(service.UID), port.NodePort),
		Pool:                   nodeIPs,
		ForwardProtocol:        protocol,
		SslBridging:            &sslBridging,
		IgnoreSslServerVerify:  sslSkipVerify,
		ForwardPort:            port.NodePort,
		ForwardPortAlgorithm:   forwardPortAlgorithm,
		StickySessions:         stickySessions,
		ProxyProtocol:          proxyProtocol,
		TimeoutServer:          &timeoutServer,
		TimeoutConnect:         &timeoutConnect,
		TimeoutTunnel:          &timeoutTunnel,
		TimeoutQueue:           timeoutQueue,
		OnMarkedDownAction:     onMarkedDownAction,
		HealthCheck:            healthCheck,
		RedispatchAttemptCount: redispatchAttemptCount,
		MaxConnections:         maxConnections,
		MaxRetries:             maxRetries,
		FailoverHost:           failoverHost,
	}

	if stickySessions == scwlb.StickySessionsTypeCookie {
		stickySessionsCookieName, err := getStickySessionsCookieName(service)
		if err != nil {
			return nil, err
		}
		if stickySessionsCookieName == "" {
			klog.Errorf("missing annotation %s", serviceAnnotationLoadBalancerStickySessionsCookieName)
			return nil, NewAnnorationError(serviceAnnotationLoadBalancerStickySessionsCookieName, stickySessionsCookieName)
		}
		backend.StickySessionsCookieName = stickySessionsCookieName
	}

	return backend, nil
}

// serviceToLB converts a service definition to a list of load balancer frontends and backends
func serviceToLB(service *v1.Service, loadbalancer *scwlb.LB, nodeIPs []string) (map[int32]*scwlb.Frontend, map[int32]*scwlb.Backend, error) {
	frontends := map[int32]*scwlb.Frontend{}
	backends := map[int32]*scwlb.Backend{}

	for _, port := range service.Spec.Ports {
		frontend, err := servicePortToFrontend(service, loadbalancer, port)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to prepare frontend for port %d: %v", port.Port, err)
		}

		backend, err := servicePortToBackend(service, loadbalancer, port, nodeIPs)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to prepare backend for port %d: %v", port.Port, err)
		}

		frontends[port.Port] = frontend
		backends[port.NodePort] = backend
	}

	return frontends, backends, nil
}

// frontendEquals returns true if the two frontends configuration are equal
func frontendEquals(got, want *scwlb.Frontend) bool {
	if got == nil || want == nil {
		return got == want
	}

	if got.Name != want.Name {
		klog.V(3).Infof("frontend.Name: %s - %s", got.Name, want.Name)
		return false
	}
	if got.InboundPort != want.InboundPort {
		klog.V(3).Infof("frontend.InboundPort: %d - %d", got.InboundPort, want.InboundPort)
		return false
	}
	if !durationPtrEqual(got.TimeoutClient, want.TimeoutClient) {
		klog.V(3).Infof("frontend.TimeoutClient: %s - %s", got.TimeoutClient, want.TimeoutClient)
		return false
	}

	if !stringArrayEqual(got.CertificateIDs, want.CertificateIDs) {
		klog.V(3).Infof("frontend.CertificateIDs: %s - %s", got.CertificateIDs, want.CertificateIDs)
		return false
	}

	if !uint32PtrEqual(got.ConnectionRateLimit, want.ConnectionRateLimit) {
		klog.V(3).Infof("frontend.ConnectionRateLimit: %s - %s", ptrUint32ToString(got.ConnectionRateLimit), ptrUint32ToString(want.ConnectionRateLimit))
		return false
	}

	if got.EnableAccessLogs != want.EnableAccessLogs {
		klog.V(3).Infof("frontend.EnableAccessLogs: %t - %t", got.EnableAccessLogs, want.EnableAccessLogs)
		return false
	}

	if got.EnableHTTP3 != want.EnableHTTP3 {
		klog.V(3).Infof("frontend.EnableHTTP3: %t - %t", got.EnableHTTP3, want.EnableHTTP3)
		return false
	}

	return true
}

// backendEquals returns true if the two backends configuration are equal
func backendEquals(got, want *scwlb.Backend) bool {
	if got == nil || want == nil {
		return got == want
	}

	if got.Name != want.Name {
		klog.V(3).Infof("backend.Name: %s - %s", got.Name, want.Name)
		return false
	}
	if got.ForwardProtocol != want.ForwardProtocol {
		klog.V(3).Infof("backend.ForwardProtocol: %s - %s", got.ForwardProtocol, want.ForwardProtocol)
		return false
	}
	if !reflect.DeepEqual(got.SslBridging, want.SslBridging) {
		klog.V(3).Infof("backend.SslBridging: %s - %s", ptrBoolToString(got.SslBridging), ptrBoolToString(want.SslBridging))
		return false
	}
	if !reflect.DeepEqual(got.IgnoreSslServerVerify, want.IgnoreSslServerVerify) {
		klog.V(3).Infof("backend.IgnoreSslServerVerify: %s - %s", ptrBoolToString(got.IgnoreSslServerVerify), ptrBoolToString(want.IgnoreSslServerVerify))
		return false
	}
	if got.ForwardPort != want.ForwardPort {
		klog.V(3).Infof("backend.ForwardPort: %d - %d", got.ForwardPort, want.ForwardPort)
		return false
	}
	if got.ForwardPortAlgorithm != want.ForwardPortAlgorithm {
		klog.V(3).Infof("backend.ForwardPortAlgorithm: %s - %s", got.ForwardPortAlgorithm, want.ForwardPortAlgorithm)
		return false
	}
	if got.StickySessions != want.StickySessions {
		klog.V(3).Infof("backend.StickySessions: %s - %s", got.StickySessions, want.StickySessions)
		return false
	}
	if got.ProxyProtocol != want.ProxyProtocol {
		klog.V(3).Infof("backend.ProxyProtocol: %s - %s", got.ProxyProtocol, want.ProxyProtocol)
		return false
	}
	if !durationPtrEqual(got.TimeoutServer, want.TimeoutServer) {
		klog.V(3).Infof("backend.TimeoutServer: %s - %s", got.TimeoutServer, want.TimeoutServer)
		return false
	}
	if !durationPtrEqual(got.TimeoutConnect, want.TimeoutConnect) {
		klog.V(3).Infof("backend.TimeoutConnect: %s - %s", got.TimeoutConnect, want.TimeoutConnect)
		return false
	}
	if !durationPtrEqual(got.TimeoutTunnel, want.TimeoutTunnel) {
		klog.V(3).Infof("backend.TimeoutTunnel: %s - %s", got.TimeoutTunnel, want.TimeoutTunnel)
		return false
	}
	if !durationPtrEqual(got.TimeoutQueue.ToTimeDuration(), want.TimeoutQueue.ToTimeDuration()) {
		klog.V(3).Infof("backend.TimeoutQueue: %s - %s", ptrScwDurationToString(got.TimeoutQueue), ptrScwDurationToString(want.TimeoutQueue))
		return false
	}
	if got.OnMarkedDownAction != want.OnMarkedDownAction {
		klog.V(3).Infof("backend.OnMarkedDownAction: %s - %s", got.OnMarkedDownAction, want.OnMarkedDownAction)
		return false
	}
	if !int32PtrEqual(got.RedispatchAttemptCount, want.RedispatchAttemptCount) {
		klog.V(3).Infof("backend.RedispatchAttemptCount: %s - %s", ptrInt32ToString(got.RedispatchAttemptCount), ptrInt32ToString(want.RedispatchAttemptCount))
		return false
	}
	if !int32PtrEqual(got.MaxConnections, want.MaxConnections) {
		klog.V(3).Infof("backend.MaxConnections: %s - %s", ptrInt32ToString(got.MaxConnections), ptrInt32ToString(want.MaxConnections))
		return false
	}
	if !int32PtrEqual(got.MaxRetries, want.MaxRetries) {
		klog.V(3).Infof("backend.MaxRetries: %s - %s", ptrInt32ToString(got.MaxRetries), ptrInt32ToString(want.MaxRetries))
		return false
	}
	if got.StickySessionsCookieName != want.StickySessionsCookieName {
		klog.V(3).Infof("backend.StickySessionsCookieName: %s - %s", got.StickySessionsCookieName, want.StickySessionsCookieName)
		return false
	}

	if !ptrStringEqual(got.FailoverHost, want.FailoverHost) {
		klog.V(3).Infof("backend.FailoverHost: %s - %s", ptrStringToString(got.FailoverHost), ptrStringToString(want.FailoverHost))
		return false
	}

	if !reflect.DeepEqual(got.HealthCheck, want.HealthCheck) {
		klog.V(3).Infof("backend.HealthCheck: %v - %v", got.HealthCheck, want.HealthCheck)
		return false
	}

	return true
}

type frontendOps struct {
	remove map[int32]*scwlb.Frontend
	update map[int32]*scwlb.Frontend
	create map[int32]*scwlb.Frontend
	keep   map[int32]*scwlb.Frontend
}

// compareFrontends returns the frontends operation to do to achieve the wanted configuration
// will ignore frontends with names not starting with the filterPrefix if provided
func compareFrontends(got []*scwlb.Frontend, want map[int32]*scwlb.Frontend, filterPrefix string) frontendOps {
	remove := make(map[int32]*scwlb.Frontend)
	update := make(map[int32]*scwlb.Frontend)
	create := make(map[int32]*scwlb.Frontend)
	keep := make(map[int32]*scwlb.Frontend)

	filteredGot := make([]*scwlb.Frontend, 0, len(got))
	for _, current := range got {
		if strings.HasPrefix(current.Name, filterPrefix) {
			filteredGot = append(filteredGot, current)
		}
	}

	// Check for deletions and updates
	for _, current := range filteredGot {
		if target, ok := want[current.InboundPort]; ok {
			if !frontendEquals(current, target) {
				target.ID = current.ID
				update[target.InboundPort] = target
			} else {
				keep[target.InboundPort] = current
			}
		} else {
			remove[current.InboundPort] = current
		}
	}

	// Check for additions
	for _, target := range want {
		found := false
		for _, current := range filteredGot {
			if current.InboundPort == target.InboundPort {
				found = true
				break
			}
		}
		if !found {
			create[target.InboundPort] = target
		}
	}

	return frontendOps{
		remove: remove,
		update: update,
		create: create,
		keep:   keep,
	}
}

type backendOps struct {
	remove map[int32]*scwlb.Backend
	update map[int32]*scwlb.Backend
	create map[int32]*scwlb.Backend
	keep   map[int32]*scwlb.Backend
}

// compareBackends returns the backends operation to do to achieve the wanted configuration
func compareBackends(got []*scwlb.Backend, want map[int32]*scwlb.Backend, filterPrefix string) backendOps {
	remove := make(map[int32]*scwlb.Backend)
	update := make(map[int32]*scwlb.Backend)
	create := make(map[int32]*scwlb.Backend)
	keep := make(map[int32]*scwlb.Backend)

	filteredGot := make([]*scwlb.Backend, 0, len(got))
	for _, current := range got {
		if strings.HasPrefix(current.Name, filterPrefix) {
			filteredGot = append(filteredGot, current)
		}
	}

	// Check for deletions and updates
	for _, current := range filteredGot {
		if target, ok := want[current.ForwardPort]; ok {
			if !backendEquals(current, target) {
				target.ID = current.ID
				update[target.ForwardPort] = target
			} else {
				keep[target.ForwardPort] = current
			}
		} else {
			remove[current.ForwardPort] = current
		}
	}

	// Check for additions
	for _, target := range want {
		found := false
		for _, current := range filteredGot {
			if current.ForwardPort == target.ForwardPort {
				found = true
				break
			}
		}
		if !found {
			create[target.ForwardPort] = target
		}
	}

	return backendOps{
		remove: remove,
		update: update,
		create: create,
		keep:   keep,
	}
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

// createBackend creates a backend on the load balancer
func (l *loadbalancers) createBackend(service *v1.Service, loadbalancer *scwlb.LB, backend *scwlb.Backend) (*scwlb.Backend, error) {
	b, err := l.api.CreateBackend(&scwlb.ZonedAPICreateBackendRequest{
		Zone:                     loadbalancer.Zone,
		LBID:                     loadbalancer.ID,
		Name:                     backend.Name,
		ForwardProtocol:          backend.ForwardProtocol,
		SslBridging:              backend.SslBridging,
		IgnoreSslServerVerify:    backend.IgnoreSslServerVerify,
		ForwardPort:              backend.ForwardPort,
		ForwardPortAlgorithm:     backend.ForwardPortAlgorithm,
		StickySessions:           backend.StickySessions,
		StickySessionsCookieName: backend.StickySessionsCookieName,
		HealthCheck:              backend.HealthCheck,
		ServerIP:                 backend.Pool,
		ProxyProtocol:            backend.ProxyProtocol,
		TimeoutServer:            backend.TimeoutServer,
		TimeoutConnect:           backend.TimeoutConnect,
		TimeoutTunnel:            backend.TimeoutTunnel,
		TimeoutQueue:             backend.TimeoutQueue,
		OnMarkedDownAction:       backend.OnMarkedDownAction,
		RedispatchAttemptCount:   backend.RedispatchAttemptCount,
		MaxConnections:           backend.MaxConnections,
		MaxRetries:               backend.MaxRetries,
		FailoverHost:             backend.FailoverHost,
	})
	if err != nil {
		return nil, err
	}

	return b, nil
}

// updateBackend updates a backend on the load balancer
func (l *loadbalancers) updateBackend(service *v1.Service, loadbalancer *scwlb.LB, backend *scwlb.Backend) (*scwlb.Backend, error) {
	b, err := l.api.UpdateBackend(&scwlb.ZonedAPIUpdateBackendRequest{
		Zone:                     loadbalancer.Zone,
		BackendID:                backend.ID,
		Name:                     backend.Name,
		ForwardProtocol:          backend.ForwardProtocol,
		SslBridging:              backend.SslBridging,
		IgnoreSslServerVerify:    backend.IgnoreSslServerVerify,
		ForwardPort:              backend.ForwardPort,
		ForwardPortAlgorithm:     backend.ForwardPortAlgorithm,
		StickySessions:           backend.StickySessions,
		StickySessionsCookieName: backend.StickySessionsCookieName,
		ProxyProtocol:            backend.ProxyProtocol,
		TimeoutServer:            backend.TimeoutServer,
		TimeoutConnect:           backend.TimeoutConnect,
		TimeoutTunnel:            backend.TimeoutTunnel,
		TimeoutQueue:             backend.TimeoutQueue,
		OnMarkedDownAction:       backend.OnMarkedDownAction,
		RedispatchAttemptCount:   backend.RedispatchAttemptCount,
		MaxConnections:           backend.MaxConnections,
		MaxRetries:               backend.MaxRetries,
		FailoverHost:             backend.FailoverHost,
	})
	if err != nil {
		return nil, err
	}

	if _, err := l.api.UpdateHealthCheck(&scwlb.ZonedAPIUpdateHealthCheckRequest{
		Zone:                loadbalancer.Zone,
		BackendID:           backend.ID,
		Port:                backend.ForwardPort,
		CheckDelay:          backend.HealthCheck.CheckDelay,
		CheckTimeout:        backend.HealthCheck.CheckTimeout,
		CheckMaxRetries:     backend.HealthCheck.CheckMaxRetries,
		CheckSendProxy:      backend.HealthCheck.CheckSendProxy,
		TCPConfig:           backend.HealthCheck.TCPConfig,
		MysqlConfig:         backend.HealthCheck.MysqlConfig,
		PgsqlConfig:         backend.HealthCheck.PgsqlConfig,
		LdapConfig:          backend.HealthCheck.LdapConfig,
		RedisConfig:         backend.HealthCheck.RedisConfig,
		HTTPConfig:          backend.HealthCheck.HTTPConfig,
		HTTPSConfig:         backend.HealthCheck.HTTPSConfig,
		TransientCheckDelay: backend.HealthCheck.TransientCheckDelay,
	}); err != nil {
		return nil, fmt.Errorf("failed to update healthcheck: %v", err)
	}

	return b, nil
}

// createFrontend creates a frontend on the load balancer
func (l *loadbalancers) createFrontend(service *v1.Service, loadbalancer *scwlb.LB, frontend *scwlb.Frontend, backend *scwlb.Backend) (*scwlb.Frontend, error) {
	f, err := l.api.CreateFrontend(&scwlb.ZonedAPICreateFrontendRequest{
		Zone:                loadbalancer.Zone,
		LBID:                loadbalancer.ID,
		Name:                frontend.Name,
		InboundPort:         frontend.InboundPort,
		BackendID:           backend.ID,
		TimeoutClient:       frontend.TimeoutClient,
		CertificateIDs:      &frontend.CertificateIDs,
		ConnectionRateLimit: frontend.ConnectionRateLimit,
		EnableAccessLogs:    frontend.EnableAccessLogs,
		EnableHTTP3:         frontend.EnableHTTP3,
	})

	return f, err
}

// updateFrontend updates a frontend on the load balancer
func (l *loadbalancers) updateFrontend(service *v1.Service, loadbalancer *scwlb.LB, frontend *scwlb.Frontend, backend *scwlb.Backend) (*scwlb.Frontend, error) {
	f, err := l.api.UpdateFrontend(&scwlb.ZonedAPIUpdateFrontendRequest{
		Zone:                loadbalancer.Zone,
		FrontendID:          frontend.ID,
		Name:                frontend.Name,
		InboundPort:         frontend.InboundPort,
		BackendID:           backend.ID,
		TimeoutClient:       frontend.TimeoutClient,
		CertificateIDs:      &frontend.CertificateIDs,
		ConnectionRateLimit: frontend.ConnectionRateLimit,
		EnableAccessLogs:    &frontend.EnableAccessLogs,
		EnableHTTP3:         frontend.EnableHTTP3,
	})

	return f, err
}

// stringArrayEqual returns true if both arrays contains the exact same elements regardless of the order
func stringArrayEqual(got, want []string) bool {
	slices.Sort(got)
	slices.Sort(want)
	return reflect.DeepEqual(got, want)
}

// ptrStringEqual returns true if both strings are equal
func ptrStringEqual(got, want *string) bool {
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	return *got == *want
}

// stringPtrArrayEqual returns true if both arrays contains the exact same elements regardless of the order
func stringPtrArrayEqual(got, want []*string) bool {
	slices.SortStableFunc(got, func(a, b *string) int { return strings.Compare(*a, *b) })
	slices.SortStableFunc(want, func(a, b *string) int { return strings.Compare(*a, *b) })
	return reflect.DeepEqual(got, want)
}

// durationPtrEqual returns true if both duration are equal
func durationPtrEqual(got, want *time.Duration) bool {
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	return *got == *want
}

// scwDurationPtrEqual returns true if both duration are equal
func scwDurationPtrEqual(got, want *scw.Duration) bool {
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	return *got == *want
}

// int32PtrEqual returns true if both integers are equal
func int32PtrEqual(got, want *int32) bool {
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	return *got == *want
}

// uint32PtrEqual returns true if both integers are equal
func uint32PtrEqual(got, want *uint32) bool {
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	return *got == *want
}

// chunkArray takes an array and split it in chunks of a given size
func chunkArray(array []string, maxChunkSize int) [][]string {
	result := [][]string{}

	for len(array) > 0 {
		chunkSize := maxChunkSize
		if len(array) < maxChunkSize {
			chunkSize = len(array)
		}

		result = append(result, array[:chunkSize])
		array = array[chunkSize:]
	}

	return result
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

	aclPrefix := makeACLPrefix(frontend)
	whitelist := extractNodesInternalIps(nodes)
	whitelist = append(whitelist, extractNodesExternalIps(nodes)...)
	whitelist = append(whitelist, strip32SubnetMasks(service.Spec.LoadBalancerSourceRanges)...)

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

func strip32SubnetMasks(subnets []string) []string {
	stripped := make([]string, len(subnets))
	for idx, subnet := range subnets {
		stripped[idx] = strings.TrimSuffix(subnet, "/32")
	}
	return stripped
}

func ptrInt32ToString(i *int32) string {
	if i == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *i)
}

func ptrUint32ToString(i *uint32) string {
	if i == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", *i)
}

func ptrBoolToString(b *bool) string {
	if b == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%t", *b)
}

// hasLoadBalancerStaticIPs returns true if static IPs are specified for the loadbalancer.
func hasLoadBalancerStaticIPs(service *v1.Service) bool {
	if ipIDs := getIPIDs(service); len(ipIDs) > 0 {
		return true
	}

	if service.Spec.LoadBalancerIP != "" {
		return true
	}

	return false
}

// hasEqualLoadBalancerStaticIPs returns true if the LB has the expected static IPs.
// This function returns true if no static IP is configured.
func hasEqualLoadBalancerStaticIPs(service *v1.Service, lb *scwlb.LB) bool {
	if ipIDs := getIPIDs(service); len(ipIDs) > 0 {
		if len(ipIDs) != len(lb.IP) {
			return false
		}

		// Sort IP IDs.
		sortedIPIDs := slices.Clone(ipIDs)
		slices.Sort(sortedIPIDs)

		// Sort LB IP IDs.
		sortedLBIPIDs := make([]string, 0, len(lb.IP))
		for _, ip := range lb.IP {
			sortedLBIPIDs = append(sortedLBIPIDs, ip.ID)
		}
		slices.Sort(sortedLBIPIDs)

		// Compare the sorted list.
		return reflect.DeepEqual(sortedIPIDs, sortedLBIPIDs)
	}

	if lbIP := service.Spec.LoadBalancerIP; lbIP != "" {
		return len(lb.IP) == 1 && lbIP == lb.IP[0].IPAddress
	}

	return true
}

// nodesInitialized verifies that all nodes are initialized before using them as LoadBalancer targets.
func nodesInitialized(nodes []*v1.Node) error {
	for _, node := range nodes {
		// If node was created more than 3 minutes ago, we ignore it to
		// avoid blocking callers indefinitely.
		if time.Since(node.CreationTimestamp.Time) > 3*time.Minute {
			continue
		}

		if slices.ContainsFunc(node.Spec.Taints, func(taint v1.Taint) bool {
			return taint.Key == api.TaintExternalCloudProvider
		}) {
			return fmt.Errorf("node %s is not yet initialized", node.Name)
		}
	}

	return nil
}

func ptrStringToString(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func ptrScwDurationToString(i *scw.Duration) string {
	if i == nil {
		return "<nil>"
	}
	return i.ToTimeDuration().String()
}
