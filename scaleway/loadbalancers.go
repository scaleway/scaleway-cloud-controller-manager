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
	"os"
	"strconv"
	"strings"
	"time"

	scwlb "github.com/scaleway/scaleway-sdk-go/api/lb/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/scaleway/scaleway-sdk-go/validation"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
)

const (
	// serviceAnnotationLoadBalancerID is the ID of the loadbalancer
	// It has the form <zone>/<lb-id>
	serviceAnnotationLoadBalancerID = "service.beta.kubernetes.io/scw-loadbalancer-id"

	// serviceAnnotationLoadBalancerForwardPortAlgorithm is the annotation to choose the load balancing algorithm
	// The default value is "roundrobin" and the possible values are "roundrobin" or "leastconn"
	serviceAnnotationLoadBalancerForwardPortAlgorithm = "service.beta.kubernetes.io/scw-loadbalancer-forward-port-algorithm"

	// serviceAnnotationLoadBalancerStickySessions is the annotation to enable cookie-based session persistence
	// The defaut value is "none" and the possible valuea are "none", "cookie", or "table"
	// NB: If the value "cookie" is used, the annotation service.beta.kubernetes.io/scw-loadbalancer-sticky-sessions-cookie-name must be set
	serviceAnnotationLoadBalancerStickySessions = "service.beta.kubernetes.io/scw-loadbalancer-sticky-sessions"

	// serviceAnnotationLoadBalancerStickySessionsCookieName is the annotation for the cookie name for sticky sessions
	// NB: muste be set if service.beta.kubernetes.io/scw-loadbalancer-sticky-sessions is set to "cookie"
	serviceAnnotationLoadBalancerStickySessionsCookieName = "service.beta.kubernetes.io/scw-loadbalancer-sticky-sessions-cookie-name"

	// serviceAnnotationLoadBalancerHealthCheckType is the type of health check used
	// The default value is "tcp" and the possible values are "tcp", "http", "https", "mysql", "pgsql", "redis" or "ldap"
	// It is possible to set the type per port, like "80:http;443,8443:https"
	// NB: depending on the type, some other annotations are required, see below
	serviceAnnotationLoadBalancerHealthCheckType = "service.beta.kubernetes.io/scw-loadbalancer-health-check-type"

	// serviceAnnotationLoadBalancerHealthCheckDelay is the time between two consecutive health checks
	// The default value is "10s". The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
	serviceAnnotationLoadBalancerHealthCheckDelay = "service.beta.kubernetes.io/scw-loadbalancer-health-check-delay"

	// serviceAnnotationLoadBalancerHealthCheckTimeout is the additional check timeout, after the connection has been already established
	// The default value is "10s". The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
	serviceAnnotationLoadBalancerHealthCheckTimeout = "service.beta.kubernetes.io/scw-loadbalancer-health-check-timeout"

	// serviceAnnotationLoadBalancerHealthCheckMaxRetries is the number of consecutive unsuccessful health checks, after wich the server will be considered dead
	// The default value is "10".
	serviceAnnotationLoadBalancerHealthCheckMaxRetries = "service.beta.kubernetes.io/scw-loadbalancer-health-check-max-retries"

	// serviceAnnotationLoadBalancerHealthCheckHTTPURI is the URI that is used by the "http" health check
	// It is possible to set the uri per port, like "80:/;443,8443:/healthz"
	// NB: Required when setting service.beta.kubernetes.io/scw-loadbalancer-health-check-type to "http" or "https"
	serviceAnnotationLoadBalancerHealthCheckHTTPURI = "service.beta.kubernetes.io/scw-loadbalancer-health-check-http-uri"

	// serviceAnnotationLoadBalancerHealthCheckHTTPMethod is the HTTP method used by the "http" health check
	// It is possible to set the method per port, like "80:GET;443,8443:POST"
	// NB: Required when setting service.beta.kubernetes.io/scw-loadbalancer-health-check-type to "http" or "https"
	serviceAnnotationLoadBalancerHealthCheckHTTPMethod = "service.beta.kubernetes.io/scw-loadbalancer-health-check-http-method"

	// serviceAnnotationLoadBalancerHealthCheckHTTPCode is the HTTP code that the "http" health check will be matching against
	// It is possible to set the code per port, like "80:404;443,8443:204"
	// NB: Required when setting service.beta.kubernetes.io/scw-loadbalancer-health-check-type to "http" or "https"
	serviceAnnotationLoadBalancerHealthCheckHTTPCode = "service.beta.kubernetes.io/scw-loadbalancer-health-check-http-code"

	// serviceAnnotationLoadBalancerHealthCheckMysqlUser is the MySQL user used to check the MySQL connection when using the "mysql" health check
	// It is possible to set the user per port, like "1234:root;3306,3307:mysql"
	// NB: Required when setting service.beta.kubernetes.io/scw-loadbalancer-health-check-type to "mysql"
	serviceAnnotationLoadBalancerHealthCheckMysqlUser = "service.beta.kubernetes.io/scw-loadbalancer-health-check-mysql-user"

	// serviceAnnotationLoadBalancerHealthCheckPgsqlUser is the PgSQL user used to check the PgSQL connection when using the "pgsql" health check
	// It is possible to set the user per port, like "1234:root;3306,3307:mysql"
	// NB: Required when setting service.beta.kubernetes.io/scw-loadbalancer-health-check-type to "pgsql"
	serviceAnnotationLoadBalancerHealthCheckPgsqlUser = "service.beta.kubernetes.io/scw-loadbalancer-health-check-pgsql-user"

	// serviceAnnotationLoadBalancerSendProxyV2 is the annotation that enables PROXY protocol version 2 (must be supported by backend servers)
	// The default value is "false" and the possible values are "false" or "true"
	// or a comma delimited list of the service port on which to apply the proxy protocol (for instance "80,443")
	// this field is DEPRECATED
	serviceAnnotationLoadBalancerSendProxyV2 = "service.beta.kubernetes.io/scw-loadbalancer-send-proxy-v2"

	// serviceAnnotationLoadBalancerProxyProtocolV1 is the annotation that can enable the PROXY protocol V1
	// The possible values are "false", "true" or "*" for all ports or a comma delimited list of the service port
	// (for instance "80,443")
	serviceAnnotationLoadBalancerProxyProtocolV1 = "service.beta.kubernetes.io/scw-loadbalancer-proxy-protocol-v1"

	// serviceAnnotationLoadBalancerProxyProtocolV2 is the annotation that can enable the PROXY protocol V2
	// The possible values are "false", "true" or "*" for all ports or a comma delimited list of the service port
	// (for instance "80,443")
	serviceAnnotationLoadBalancerProxyProtocolV2 = "service.beta.kubernetes.io/scw-loadbalancer-proxy-protocol-v2"

	// serviceAnnotationLoadBalancerType is the load balancer offer type
	serviceAnnotationLoadBalancerType = "service.beta.kubernetes.io/scw-loadbalancer-type"

	// serviceAnnotationLoadBalancerZone is the zone to create the load balancer
	serviceAnnotationLoadBalancerZone = "service.beta.kubernetes.io/scw-loadbalancer-zone"

	// serviceAnnotationLoadBalancerTimeoutServer is the maximum server connection inactivity time
	// The default value is "10m". The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
	serviceAnnotationLoadBalancerTimeoutServer = "service.beta.kubernetes.io/scw-loadbalancer-timeout-server"

	// serviceAnnotationLoadBalancerTimeoutConnect is the maximum initical server connection establishment time
	// The default value is "10m". The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
	serviceAnnotationLoadBalancerTimeoutConnect = "service.beta.kubernetes.io/scw-loadbalancer-timeout-connect"

	// serviceAnnotationLoadBalancerTimeoutTunnel is the maximum tunnel inactivity time
	// The default value is "10m". The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
	serviceAnnotationLoadBalancerTimeoutTunnel = "service.beta.kubernetes.io/scw-loadbalancer-timeout-tunnel"

	// serviceAnnotationLoadBalancerOnMarkedDownAction is the annotation that modifes what occurs when a backend server is marked down
	// The default value is "on_marked_down_action_none" and the possible values are "on_marked_down_action_none" and "shutdown_sessions"
	serviceAnnotationLoadBalancerOnMarkedDownAction = "service.beta.kubernetes.io/scw-loadbalancer-on-marked-down-action"

	// serviceAnnotationLoadBalancerForceInternalIP is the annotation that force the usage of InternalIP inside the loadbalancer
	// Normally, the cloud controller manager use ExternalIP to be nodes region-free (or public InternalIP in case of Baremetal).
	serviceAnnotationLoadBalancerForceInternalIP = "service.beta.kubernetes.io/scw-loadbalancer-force-internal-ip"

	// serviceAnnotationLoadBalancerUseHostname is the annotation that force the use of the LB hostname instead of the public IP.
	// This is useful when it it needed to not bypass the LoadBalacer for traffic coming from the cluster
	serviceAnnotationLoadBalancerUseHostname = "service.beta.kubernetes.io/scw-loadbalancer-use-hostname"

	// serviceAnnotationLoadBalancerProtocolHTTP is the annotation to set the forward protocol of the LB to HTTP
	// The possible values are "false", "true" or "*" for all ports or a comma delimited list of the service port
	// (for instance "80,443")
	serviceAnnotationLoadBalancerProtocolHTTP = "service.beta.kubernetes.io/scw-loadbalancer-protocol-http"

	// serviceAnnotationLoadBalancerCertificateIDs is the annotation to choose the the certificate IDS to associate
	// with this LoadBalancer.
	// The possible format are:
	// "<certificate-id>": will use this certificate for all frontends
	// "<certificate-id>,<certificate-id>" will use these certificates for all frontends
	// "<port1>:<certificate1-id>,<certificate2-id>;<port2>,<port3>:<certificate3-id>" will use certificate 1 and 2 for frontend with port port1
	// and certificate3 for frotend with port port2 and port3
	serviceAnnotationLoadBalancerCertificateIDs = "service.beta.kubernetes.io/scw-loadbalancer-certificate-ids"
)

type loadbalancers struct {
	api           LoadBalancerAPI
	client        *client // for patcher
	defaultLBType string
}

type LoadBalancerAPI interface {
	ListLBs(req *scwlb.ZonedAPIListLBsRequest, opts ...scw.RequestOption) (*scwlb.ListLBsResponse, error)
	GetLB(req *scwlb.ZonedAPIGetLBRequest, opts ...scw.RequestOption) (*scwlb.LB, error)
	CreateLB(req *scwlb.ZonedAPICreateLBRequest, opts ...scw.RequestOption) (*scwlb.LB, error)
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
}

func newLoadbalancers(client *client, defaultLBType string) *loadbalancers {
	lbType := "lb-s"
	if defaultLBType != "" {
		lbType = strings.ToLower(defaultLBType)
	}
	return &loadbalancers{
		api:           scwlb.NewZonedAPI(client.scaleway),
		client:        client,
		defaultLBType: lbType,
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
			klog.Infof("no load balancer found for service %s", service.Name)
			return nil, false, nil
		}

		klog.Errorf("error getting load balancer for service %s: %v", service.Name, err)
		return nil, false, err
	}

	status := &v1.LoadBalancerStatus{}
	status.Ingress = make([]v1.LoadBalancerIngress, len(lb.IP))
	for idx, ip := range lb.IP {
		if getUseHostname(service) {
			status.Ingress[idx].Hostname = ip.Reverse
		} else {
			status.Ingress[idx].IP = ip.IPAddress
		}
	}

	return status, true, nil
}

// EnsureLoadBalancer creates a new load balancer 'name', or updates the existing one. Returns the status of the balancer
// Implementations must treat the *v1.Service and *v1.Node
// parameters as read-only and not modify them.
// Parameter 'clusterName' is the name of the cluster as presented to kube-controller-manager
func (l *loadbalancers) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	if service.Spec.LoadBalancerClass != nil {
		return nil, fmt.Errorf("scaleway-cloud-controller-manager cannot handle loadBalancerClas %s", *service.Spec.LoadBalancerClass)
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
		// any kind of Error
		klog.Errorf("error getting loadbalancer for service %s: %v", service.Name, err)
		return nil, err
	}

	if service.Spec.LoadBalancerIP != "" && service.Spec.LoadBalancerIP != lb.IP[0].IPAddress {
		err = l.deleteLoadBalancer(ctx, lb, service)
		if err != nil {
			return nil, err
		}

		lb, err = l.createLoadBalancer(ctx, clusterName, service)
		if err != nil {
			return nil, err
		}
	}

	if lb.Status != scwlb.LBStatusReady {
		return nil, LoadBalancerNotReady
	}

	err = l.updateLoadBalancer(ctx, lb, service, nodes)
	if err != nil {
		klog.Errorf("error updating loadbalancer for service %s: %v", service.Name, err)
		return nil, err
	}

	status := &v1.LoadBalancerStatus{}
	status.Ingress = make([]v1.LoadBalancerIngress, len(lb.IP))
	for idx, ip := range lb.IP {
		if getUseHostname(service) {
			status.Ingress[idx].Hostname = ip.Reverse
		} else {
			status.Ingress[idx].IP = ip.IPAddress
		}
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

	return l.deleteLoadBalancer(ctx, lb, service)
}

//
func (l *loadbalancers) deleteLoadBalancer(ctx context.Context, lb *scwlb.LB, service *v1.Service) error {
	// remove loadbalancer annotation
	if err := l.unannotateAndPatch(service); err != nil {
		return err
	}

	// if loadbalancer is renamed, do not delete it.
	if lb.Name != l.GetLoadBalancerName(ctx, "", service) {
		return nil
	}

	// if loadBalancerIP is not set, it implies an ephemeral IP
	releaseIP := service.Spec.LoadBalancerIP == ""

	request := &scwlb.ZonedAPIDeleteLBRequest{
		Zone:      lb.Zone,
		LBID:      lb.ID,
		ReleaseIP: releaseIP,
	}

	err := l.api.DeleteLB(request)
	if err != nil {
		klog.Errorf("error deleting load balancer %s: %v", lb.ID, err)
		return fmt.Errorf("error deleting load balancer %s: %v", lb.ID, err)
	}

	return nil
}

// GetLoadBalancerName returns the name of the load balancer. Implementations must treat the
// *v1.Service parameter as read-only and not modify it.
func (l *loadbalancers) GetLoadBalancerName(ctx context.Context, clusterName string, service *v1.Service) string {
	loadbalancerPrefix := os.Getenv(scwCcmPrefixEnv)
	kubelbName := string(service.UID)

	return loadbalancerPrefix + kubelbName
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
	if zone, loadBalancerID, err := getLoadBalancerID(service); loadBalancerID != "" {
		if err != nil {
			return nil, err
		}

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

		return resp, nil
	}

	// fetch LoadBalancer by using name.
	return l.getLoadbalancerByName(ctx, service)
}

func (l *loadbalancers) getLoadbalancerByName(ctx context.Context, service *v1.Service) (*scwlb.LB, error) {
	name := l.GetLoadBalancerName(ctx, "", service)

	var loadbalancer *scwlb.LB
	resp, err := l.api.ListLBs(&scwlb.ZonedAPIListLBsRequest{
		//Zone: Use default zone from SDK
		Name: &name,
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
	scwCcmTagsDelimiter := os.Getenv(scwCcmTagsDelimiterEnv)
	if scwCcmTagsDelimiter == "" {
		scwCcmTagsDelimiter = ","
	}

	var ipID *string
	if service.Spec.LoadBalancerIP != "" {
		request := scwlb.ZonedAPIListIPsRequest{
			IPAddress: &service.Spec.LoadBalancerIP,
			Zone:      getLoadBalancerZone(service),
		}
		ipsResp, err := l.api.ListIPs(&request)
		if err != nil {
			klog.Errorf("error getting ip for service %s: %v", service.Name, err)
			return nil, fmt.Errorf("createLoadBalancer: error getting ip for service %s: %s", service.Name, err.Error())
		}

		if len(ipsResp.IPs) == 0 {
			return nil, IPAddressNotFound
		}

		if ipsResp.IPs[0].LBID != nil && *ipsResp.IPs[0].LBID != "" {
			return nil, IPAddressInUse
		}

		ipID = &ipsResp.IPs[0].ID
	}

	scwCcmTags := os.Getenv(scwCcmTagsEnv)
	tags := []string{}
	if scwCcmTags != "" {
		tags = strings.Split(scwCcmTags, scwCcmTagsDelimiter)
	}
	tags = append(tags, "managed-by-scaleway-cloud-controller-manager")
	lbName := l.GetLoadBalancerName(ctx, clusterName, service)

	lbType := getLoadBalancerType(service)
	if lbType == "" {
		lbType = l.defaultLBType
	}

	request := scwlb.ZonedAPICreateLBRequest{
		Zone:        getLoadBalancerZone(service),
		Name:        lbName,
		Description: "kubernetes service " + service.Name,
		Tags:        tags,
		IPID:        ipID,
		Type:        lbType,
	}

	lb, err := l.api.CreateLB(&request)
	if err != nil {
		klog.Errorf("error creating load balancer for service %s: %v", service.Name, err)
		return nil, fmt.Errorf("error creating load balancer for service %s: %v", service.Name, err)
	}

	// annotate newly created loadBalancer
	if err := l.annotateAndPatch(service, lb); err != nil {
		return nil, err
	}

	return lb, nil
}

func (l *loadbalancers) annotateAndPatch(service *v1.Service, loadbalancer *scwlb.LB) error {
	service = service.DeepCopy()
	patcher := NewServicePatcher(l.client.kubernetes, service)

	if service.ObjectMeta.Annotations == nil {
		service.ObjectMeta.Annotations = map[string]string{}
	}
	service.ObjectMeta.Annotations[serviceAnnotationLoadBalancerID] = loadbalancer.Zone.String() + "/" + loadbalancer.ID

	return patcher.Patch()
}

func (l *loadbalancers) unannotateAndPatch(service *v1.Service) error {
	service = service.DeepCopy()
	patcher := NewServicePatcher(l.client.kubernetes, service)

	if service.ObjectMeta.Annotations != nil {
		delete(service.ObjectMeta.Annotations, serviceAnnotationLoadBalancerID)
	}

	return patcher.Patch()
}

func (l *loadbalancers) updateLoadBalancer(ctx context.Context, loadbalancer *scwlb.LB, service *v1.Service, nodes []*v1.Node) error {
	// List all frontends associated with the LB
	respFrontends, err := l.api.ListFrontends(&scwlb.ZonedAPIListFrontendsRequest{
		Zone: loadbalancer.Zone,
		LBID: loadbalancer.ID,
	}, scw.WithAllPages())

	if err != nil {
		return fmt.Errorf("error updating load balancer %s: %v", loadbalancer.ID, err)
	}

	frontends := respFrontends.Frontends

	portFrontends := make(map[int32]*scwlb.Frontend)
	for _, frontend := range frontends {
		keep := false
		for _, port := range service.Spec.Ports {
			// if the frontend is still valid keep it
			if port.Port == frontend.InboundPort && port.NodePort == frontend.Backend.ForwardPort {
				keep = true
				break
			}
		}

		if !keep {
			// if the frontend is not valid anymore, delete it
			klog.Infof("deleting frontend: %s", frontend.ID)

			err := l.api.DeleteFrontend(&scwlb.ZonedAPIDeleteFrontendRequest{
				Zone:       loadbalancer.Zone,
				FrontendID: frontend.ID,
			})

			if err != nil {
				return fmt.Errorf("error deleting frontend %s: %v", frontend.ID, err)
			}
		} else {
			portFrontends[frontend.InboundPort] = frontend
		}
	}

	// List all backends associated with the LB
	respBackends, err := l.api.ListBackends(&scwlb.ZonedAPIListBackendsRequest{
		Zone: loadbalancer.Zone,
		LBID: loadbalancer.ID,
	}, scw.WithAllPages())

	if err != nil {
		return fmt.Errorf("error listing backend for load balancer %s: %v", loadbalancer.ID, err)
	}

	backends := respBackends.Backends

	portBackends := make(map[int32]*scwlb.Backend)
	for _, backend := range backends {
		keep := false
		for _, port := range service.Spec.Ports {
			// if the backend is still valid, keep it
			if port.NodePort == backend.ForwardPort {
				keep = true
				break
			}
		}

		if !keep {
			// if the backend is not valid, delete it
			err := l.api.DeleteBackend(&scwlb.ZonedAPIDeleteBackendRequest{
				Zone:      loadbalancer.Zone,
				BackendID: backend.ID,
			})

			if err != nil {
				return fmt.Errorf("error deleing backend %s: %v", backend.ID, err)
			}
		} else {
			portBackends[backend.ForwardPort] = backend
		}
	}

	// loop through all the service ports
	for _, port := range service.Spec.Ports {
		// if the corresponding backend exists for the node port, update it
		if backend, ok := portBackends[port.NodePort]; ok {
			updateBackendRequest, err := l.makeUpdateBackendRequest(backend, service, nodes)
			if err != nil {
				klog.Errorf("error making UpdateBackendRequest: %v", err)
				return err
			}

			updateBackendRequest.Zone = loadbalancer.Zone
			updateBackendRequest.ForwardPort = port.NodePort
			_, err = l.api.UpdateBackend(updateBackendRequest)
			if err != nil {
				klog.Errorf("error updating backend %s: %v", backend.ID, err)
				return fmt.Errorf("error updating backend %s: %v", backend.ID, err)
			}

			updateHealthCheckRequest, err := l.makeUpdateHealthCheckRequest(backend, port.NodePort, service, nodes)
			if err != nil {
				klog.Errorf("error making UpdateHealthCheckRequest: %v", err)
				return err
			}

			updateHealthCheckRequest.Zone = loadbalancer.Zone
			_, err = l.api.UpdateHealthCheck(updateHealthCheckRequest)
			if err != nil {
				klog.Errorf("error updating healthcheck for backend %s: %v", backend.ID, err)
				return fmt.Errorf("error updating healthcheck for backend %s: %v", backend.ID, err)
			}

			var serverIPs []string
			if getForceInternalIP(service) {
				serverIPs = extractNodesInternalIps(nodes)
			} else {
				serverIPs = extractNodesExternalIps(nodes)
			}

			setBackendServersRequest := &scwlb.ZonedAPISetBackendServersRequest{
				Zone:      loadbalancer.Zone,
				BackendID: backend.ID,
				ServerIP:  serverIPs,
			}

			respBackend, err := l.api.SetBackendServers(setBackendServersRequest)
			if err != nil {
				klog.Errorf("error setting backend servers for backend %s: %v", backend.ID, err)
				return fmt.Errorf("error setting backend servers for backend %s: %v", backend.ID, err)
			}

			portBackends[backend.ForwardPort] = respBackend
		} else { // if a backend does not exists for the node port, create it
			request, err := l.makeCreateBackendRequest(loadbalancer, port.NodePort, service, nodes)
			if err != nil {
				klog.Errorf("error making CreateBackendRequest: %v", err)
				return err
			}

			respBackend, err := l.api.CreateBackend(request)
			if err != nil {
				klog.Errorf("error creating backend on load balancer %s: %v", loadbalancer.ID, err)
				return fmt.Errorf("error creating backend on load balancer %s: %v", loadbalancer.ID, err)
			}

			portBackends[port.NodePort] = respBackend
		}
	}

	for _, port := range service.Spec.Ports {
		var frontendID string
		certificateIDs, err := getCertificateIDs(service, port.Port)
		if err != nil {
			return fmt.Errorf("error getting certificate IDs for loadbalancer %s: %v", loadbalancer.ID, err)
		}
		// if the frontend exists for the port, update it
		if frontend, ok := portFrontends[port.Port]; ok {
			_, err := l.api.UpdateFrontend(&scwlb.ZonedAPIUpdateFrontendRequest{
				Zone:           loadbalancer.Zone,
				FrontendID:     frontend.ID,
				Name:           frontend.Name,
				InboundPort:    frontend.InboundPort,
				BackendID:      portBackends[port.NodePort].ID,
				TimeoutClient:  frontend.TimeoutClient,
				CertificateIDs: scw.StringsPtr(certificateIDs),
			})

			if err != nil {
				klog.Errorf("error updating frontend %s: %v", frontend.ID, err)
				return fmt.Errorf("error updating frontend %s: %v", frontend.ID, err)
			}

			frontendID = frontend.ID
		} else { // if the frontend for this port does not exist, create it
			timeoutClient := time.Minute * 10
			resp, err := l.api.CreateFrontend(&scwlb.ZonedAPICreateFrontendRequest{
				Zone:           loadbalancer.Zone,
				LBID:           loadbalancer.ID,
				Name:           fmt.Sprintf("%s_tcp_%d", string(service.UID), port.Port),
				InboundPort:    port.Port,
				BackendID:      portBackends[port.NodePort].ID,
				TimeoutClient:  &timeoutClient, // TODO use annotation?
				CertificateIDs: scw.StringsPtr(certificateIDs),
			})

			if err != nil {
				klog.Errorf("error creating frontend on load balancer %s: %v", loadbalancer.ID, err)
				return fmt.Errorf("error creating frontend on load balancer %s: %v", loadbalancer.ID, err)
			}

			frontendID = resp.ID
		}

		aclName := frontendID + "-lb-source-range"

		acls, err := l.api.ListACLs(&scwlb.ZonedAPIListACLsRequest{
			Zone:       loadbalancer.Zone,
			FrontendID: frontendID,
			Name:       &aclName,
		}, scw.WithAllPages())
		if err != nil {
			return err
		}

		if len(service.Spec.LoadBalancerSourceRanges) == 0 || len(acls.ACLs) != 1 {
			for _, acl := range acls.ACLs {
				err = l.api.DeleteACL(&scwlb.ZonedAPIDeleteACLRequest{
					Zone:  loadbalancer.Zone,
					ACLID: acl.ID,
				})
				if err != nil {
					return err
				}
			}
		}

		if len(service.Spec.LoadBalancerSourceRanges) != 0 {
			aclIPs := extractNodesInternalIps(nodes)
			aclIPs = append(aclIPs, extractNodesExternalIps(nodes)...)
			aclIPs = append(aclIPs, service.Spec.LoadBalancerSourceRanges...)
			aclIPsPtr := make([]*string, len(aclIPs))
			for i := range aclIPs {
				aclIPsPtr[i] = &aclIPs[i]
			}

			if len(acls.ACLs) != 1 {
				_, err := l.api.CreateACL(&scwlb.ZonedAPICreateACLRequest{
					Zone:       loadbalancer.Zone,
					FrontendID: frontendID,
					Name:       aclName,
					Action: &scwlb.ACLAction{
						Type: scwlb.ACLActionTypeDeny,
					},
					Index: 0,
					Match: &scwlb.ACLMatch{
						IPSubnet: aclIPsPtr,
						Invert:   true,
					},
				})
				if err != nil {
					return err
				}
			} else if len(acls.ACLs) == 1 {
				_, err := l.api.UpdateACL(&scwlb.ZonedAPIUpdateACLRequest{
					Zone:   loadbalancer.Zone,
					ACLID:  acls.ACLs[0].ID,
					Action: &scwlb.ACLAction{
						Type: scwlb.ACLActionTypeDeny,
					},
					Index: 0,
					Match: &scwlb.ACLMatch{
						Invert:   true,
						IPSubnet: aclIPsPtr,
					},
					Name: aclName,
				})
				if err != nil {
					return err
				}

			}

		}
	}

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

	return nil
}

func (l *loadbalancers) makeUpdateBackendRequest(backend *scwlb.Backend, service *v1.Service, nodes []*v1.Node) (*scwlb.ZonedAPIUpdateBackendRequest, error) {
	protocol, err := getForwardProtocol(service, backend.ForwardPort)
	if err != nil {
		return nil, err
	}

	request := &scwlb.ZonedAPIUpdateBackendRequest{
		BackendID:       backend.ID,
		Name:            backend.Name,
		ForwardProtocol: protocol,
	}

	forwardPortAlgorithm, err := getForwardPortAlgorithm(service)
	if err != nil {
		return nil, err
	}
	request.ForwardPortAlgorithm = forwardPortAlgorithm

	stickySessions, err := getStickySessions(service)
	if err != nil {
		return nil, err
	}

	request.StickySessions = stickySessions

	if stickySessions == scwlb.StickySessionsTypeCookie {
		stickySessionsCookieName, err := getStickySessionsCookieName(service)
		if err != nil {
			return nil, err
		}
		if stickySessionsCookieName == "" {
			klog.Errorf("missing annotation %s", serviceAnnotationLoadBalancerStickySessionsCookieName)
			return nil, NewAnnorationError(serviceAnnotationLoadBalancerStickySessionsCookieName, stickySessionsCookieName)
		}
		request.StickySessionsCookieName = stickySessionsCookieName
	}

	proxyProtocol, err := getProxyProtocol(service, backend.ForwardPort)
	if err != nil {
		return nil, err
	}
	request.ProxyProtocol = proxyProtocol

	timeoutServer, err := getTimeoutServer(service)
	if err != nil {
		return nil, err
	}

	request.TimeoutServer = &timeoutServer

	timeoutConnect, err := getTimeoutConnect(service)
	if err != nil {
		return nil, err
	}

	request.TimeoutConnect = &timeoutConnect

	timeoutTunnel, err := getTimeoutTunnel(service)
	if err != nil {
		return nil, err
	}

	request.TimeoutTunnel = &timeoutTunnel

	onMarkedDownAction, err := getOnMarkedDownAction(service)
	if err != nil {
		return nil, err
	}

	request.OnMarkedDownAction = onMarkedDownAction

	return request, nil
}

func (l *loadbalancers) makeUpdateHealthCheckRequest(backend *scwlb.Backend, nodePort int32, service *v1.Service, nodes []*v1.Node) (*scwlb.ZonedAPIUpdateHealthCheckRequest, error) {
	request := &scwlb.ZonedAPIUpdateHealthCheckRequest{
		BackendID: backend.ID,
		Port:      nodePort,
	}

	healthCheckDelay, err := getHealthCheckDelay(service)
	if err != nil {
		return nil, err
	}

	request.CheckDelay = &healthCheckDelay

	healthCheckTimeout, err := getHealthCheckTimeout(service)
	if err != nil {
		return nil, err
	}

	request.CheckTimeout = &healthCheckTimeout

	healthCheckMaxRetries, err := getHealthCheckMaxRetries(service)
	if err != nil {
		return nil, err
	}

	request.CheckMaxRetries = healthCheckMaxRetries

	healthCheckType, err := getHealthCheckType(service, nodePort)
	if err != nil {
		return nil, err
	}

	switch healthCheckType {
	case "mysql":
		hc, err := getMysqlHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		request.MysqlConfig = hc
	case "ldap":
		hc, err := getLdapHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		request.LdapConfig = hc
	case "redis":
		hc, err := getRedisHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		request.RedisConfig = hc
	case "pgsql":
		hc, err := getPgsqlHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		request.PgsqlConfig = hc
	case "tcp":
		hc, err := getTCPHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		request.TCPConfig = hc
	case "http":
		hc, err := getHTTPHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		request.HTTPConfig = hc
	case "https":
		hc, err := getHTTPSHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		request.HTTPSConfig = hc
	default:
		klog.Errorf("wrong value for healthCheckType")
		return nil, NewAnnorationError(serviceAnnotationLoadBalancerHealthCheckType, healthCheckType)
	}

	return request, nil
}

func (l *loadbalancers) makeCreateBackendRequest(loadbalancer *scwlb.LB, nodePort int32, service *v1.Service, nodes []*v1.Node) (*scwlb.ZonedAPICreateBackendRequest, error) {
	protocol, err := getForwardProtocol(service, nodePort)
	if err != nil {
		return nil, err
	}
	var serverIPs []string
	if getForceInternalIP(service) {
		serverIPs = extractNodesInternalIps(nodes)
	} else {
		serverIPs = extractNodesExternalIps(nodes)
	}
	request := &scwlb.ZonedAPICreateBackendRequest{
		Zone:            loadbalancer.Zone,
		LBID:            loadbalancer.ID,
		Name:            fmt.Sprintf("%s_tcp_%d", string(service.UID), nodePort),
		ServerIP:        serverIPs,
		ForwardProtocol: protocol,
		ForwardPort:     nodePort,
	}

	forwardPortAlgorithm, err := getForwardPortAlgorithm(service)
	if err != nil {
		return nil, err
	}
	request.ForwardPortAlgorithm = forwardPortAlgorithm

	stickySessions, err := getStickySessions(service)
	if err != nil {
		return nil, err
	}

	request.StickySessions = stickySessions

	if stickySessions == scwlb.StickySessionsTypeCookie {
		stickySessionsCookieName, err := getStickySessionsCookieName(service)
		if err != nil {
			return nil, err
		}
		if stickySessionsCookieName == "" {
			klog.Errorf("missing annotation %s", serviceAnnotationLoadBalancerStickySessionsCookieName)
			return nil, NewAnnorationError(serviceAnnotationLoadBalancerStickySessionsCookieName, stickySessionsCookieName)
		}
		request.StickySessionsCookieName = stickySessionsCookieName
	}

	proxyProtocol, err := getProxyProtocol(service, nodePort)
	if err != nil {
		return nil, err
	}
	request.ProxyProtocol = proxyProtocol

	timeoutServer, err := getTimeoutServer(service)
	if err != nil {
		return nil, err
	}

	request.TimeoutServer = &timeoutServer

	timeoutConnect, err := getTimeoutConnect(service)
	if err != nil {
		return nil, err
	}

	request.TimeoutConnect = &timeoutConnect

	timeoutTunnel, err := getTimeoutTunnel(service)
	if err != nil {
		return nil, err
	}

	request.TimeoutTunnel = &timeoutTunnel

	onMarkedDownAction, err := getOnMarkedDownAction(service)
	if err != nil {
		return nil, err
	}

	request.OnMarkedDownAction = onMarkedDownAction

	healthCheck := &scwlb.HealthCheck{
		Port: nodePort,
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

	healthCheckType, err := getHealthCheckType(service, nodePort)
	if err != nil {
		return nil, err
	}

	switch healthCheckType {
	case "mysql":
		hc, err := getMysqlHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		healthCheck.MysqlConfig = hc
	case "ldap":
		hc, err := getLdapHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		healthCheck.LdapConfig = hc
	case "redis":
		hc, err := getRedisHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		healthCheck.RedisConfig = hc
	case "pgsql":
		hc, err := getPgsqlHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		healthCheck.PgsqlConfig = hc
	case "tcp":
		hc, err := getTCPHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		healthCheck.TCPConfig = hc
	case "http":
		hc, err := getHTTPHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		healthCheck.HTTPConfig = hc
	case "https":
		hc, err := getHTTPSHealthCheck(service, nodePort)
		if err != nil {
			return nil, err
		}
		healthCheck.HTTPSConfig = hc
	default:
		klog.Errorf("wrong value for healthCheckType")
		return nil, errLoadBalancerInvalidAnnotation
	}

	request.HealthCheck = healthCheck

	return request, nil
}

func getLoadBalancerID(service *v1.Service) (scw.Zone, string, error) {
	annoLoadBalancerID, ok := service.Annotations[serviceAnnotationLoadBalancerID]
	if !ok {
		return "", "", errLoadBalancerInvalidAnnotation
	}

	splitLoadBalancerID := strings.Split(strings.ToLower(annoLoadBalancerID), "/")
	if len(splitLoadBalancerID) != 2 {
		return "", "", errLoadBalancerInvalidLoadBalancerID
	}

	if validation.IsRegion(splitLoadBalancerID[0]) {
		zone := splitLoadBalancerID[0] + "-1"
		return scw.Zone(zone), splitLoadBalancerID[1], nil
	}

	return scw.Zone(splitLoadBalancerID[0]), splitLoadBalancerID[1], nil
}

func getForwardPortAlgorithm(service *v1.Service) (scwlb.ForwardPortAlgorithm, error) {
	forwardPortAlgorithm, ok := service.Annotations[serviceAnnotationLoadBalancerForwardPortAlgorithm]
	if !ok {
		return scwlb.ForwardPortAlgorithmRoundrobin, nil
	}

	forwardPortAlgorithmValue := scwlb.ForwardPortAlgorithm(forwardPortAlgorithm)

	if forwardPortAlgorithmValue != scwlb.ForwardPortAlgorithmRoundrobin && forwardPortAlgorithmValue != scwlb.ForwardPortAlgorithmLeastconn {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerForwardPortAlgorithm)
		return "", errLoadBalancerInvalidAnnotation
	}

	return forwardPortAlgorithmValue, nil
}

func getStickySessions(service *v1.Service) (scwlb.StickySessionsType, error) {
	stickySessions, ok := service.Annotations[serviceAnnotationLoadBalancerStickySessions]
	if !ok {
		return scwlb.StickySessionsTypeNone, nil
	}

	stickySessionsValue := scwlb.StickySessionsType(stickySessions)

	if stickySessionsValue != scwlb.StickySessionsTypeNone && stickySessionsValue != scwlb.StickySessionsTypeCookie && stickySessionsValue != scwlb.StickySessionsTypeTable {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerStickySessions)
		return "", errLoadBalancerInvalidAnnotation
	}

	return stickySessionsValue, nil
}

func getStickySessionsCookieName(service *v1.Service) (string, error) {
	stickySessionsCookieName, ok := service.Annotations[serviceAnnotationLoadBalancerStickySessionsCookieName]
	if !ok {
		return "", nil
	}

	return stickySessionsCookieName, nil
}

func getSendProxyV2(service *v1.Service, nodePort int32) (scwlb.ProxyProtocol, error) {
	sendProxyV2, ok := service.Annotations[serviceAnnotationLoadBalancerSendProxyV2]
	if !ok {
		return scwlb.ProxyProtocolProxyProtocolNone, nil
	}

	sendProxyV2Value, err := strconv.ParseBool(sendProxyV2)
	if err != nil {
		var svcPort int32 = -1
		for _, p := range service.Spec.Ports {
			if p.NodePort == nodePort {
				svcPort = p.Port
			}
		}
		if svcPort == -1 {
			klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerSendProxyV2)
			return "", errLoadBalancerInvalidAnnotation
		}

		ports := strings.Split(strings.ReplaceAll(sendProxyV2, " ", ""), ",")
		for _, port := range ports {
			intPort, err := strconv.ParseInt(port, 0, 64)
			if err != nil {
				klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerSendProxyV2)
				return "", errLoadBalancerInvalidAnnotation
			}
			if int64(svcPort) == intPort {
				return scwlb.ProxyProtocolProxyProtocolV2, nil
			}
		}
		return scwlb.ProxyProtocolProxyProtocolNone, nil
	}

	if sendProxyV2Value {
		return scwlb.ProxyProtocolProxyProtocolV2, nil
	}

	return scwlb.ProxyProtocolProxyProtocolNone, nil
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
		if int64(p) == intPort {
			return true, nil
		}
	}
	return false, nil
}

func getLoadBalancerType(service *v1.Service) string {
	return strings.ToLower(service.Annotations[serviceAnnotationLoadBalancerType])
}

func getLoadBalancerZone(service *v1.Service) scw.Zone {
	return scw.Zone(strings.ToLower(service.Annotations[serviceAnnotationLoadBalancerZone]))
}

func getProxyProtocol(service *v1.Service, nodePort int32) (scwlb.ProxyProtocol, error) {
	proxyProtocolV1 := service.Annotations[serviceAnnotationLoadBalancerProxyProtocolV1]
	proxyProtocolV2 := service.Annotations[serviceAnnotationLoadBalancerProxyProtocolV2]

	var svcPort int32 = -1
	for _, p := range service.Spec.Ports {
		if p.NodePort == nodePort {
			svcPort = p.Port
		}
	}
	if svcPort == -1 {
		klog.Errorf("no valid port found")
		return "", errLoadBalancerInvalidAnnotation
	}

	isV1, err := isPortInRange(proxyProtocolV1, svcPort)
	if err != nil {
		klog.Errorf("unable to check if port %d is in range %s", svcPort, proxyProtocolV1)
		return "", err
	}
	isV2, err := isPortInRange(proxyProtocolV2, svcPort)
	if err != nil {
		klog.Errorf("unable to check if port %d is in range %s", svcPort, proxyProtocolV2)
		return "", err
	}

	if isV1 && isV2 {
		klog.Errorf("port %d is in both v1 and v2 proxy protocols", svcPort)
		return "", fmt.Errorf("port %d is in both v1 and v2 proxy protocols", svcPort)
	}

	if isV1 {
		return scwlb.ProxyProtocolProxyProtocolV1, nil
	}
	if isV2 {
		return scwlb.ProxyProtocolProxyProtocolV2, nil
	}

	return getSendProxyV2(service, nodePort)
}

func getTimeoutServer(service *v1.Service) (time.Duration, error) {
	timeoutServer, ok := service.Annotations[serviceAnnotationLoadBalancerTimeoutServer]
	if !ok {
		return time.ParseDuration("10m")
	}

	timeoutServerDuration, err := time.ParseDuration(timeoutServer)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerTimeoutServer)
		return time.Duration(0), errLoadBalancerInvalidAnnotation
	}

	return timeoutServerDuration, nil
}

func getTimeoutConnect(service *v1.Service) (time.Duration, error) {
	timeoutConnect, ok := service.Annotations[serviceAnnotationLoadBalancerTimeoutConnect]
	if !ok {
		return time.ParseDuration("10m")
	}

	timeoutConnectDuration, err := time.ParseDuration(timeoutConnect)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerTimeoutConnect)
		return time.Duration(0), errLoadBalancerInvalidAnnotation
	}

	return timeoutConnectDuration, nil
}

func getTimeoutTunnel(service *v1.Service) (time.Duration, error) {
	timeoutTunnel, ok := service.Annotations[serviceAnnotationLoadBalancerTimeoutTunnel]
	if !ok {
		return time.ParseDuration("10m")
	}

	timeoutTunnelDuration, err := time.ParseDuration(timeoutTunnel)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerTimeoutTunnel)
		return time.Duration(0), errLoadBalancerInvalidAnnotation
	}

	return timeoutTunnelDuration, nil
}

func getOnMarkedDownAction(service *v1.Service) (scwlb.OnMarkedDownAction, error) {
	onMarkedDownAction, ok := service.Annotations[serviceAnnotationLoadBalancerOnMarkedDownAction]
	if !ok {
		return scwlb.OnMarkedDownActionOnMarkedDownActionNone, nil
	}

	onMarkedDownActionValue := scwlb.OnMarkedDownAction(onMarkedDownAction)

	if onMarkedDownActionValue != scwlb.OnMarkedDownActionOnMarkedDownActionNone && onMarkedDownActionValue != scwlb.OnMarkedDownActionShutdownSessions {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerOnMarkedDownAction)
		return "", errLoadBalancerInvalidAnnotation
	}

	return onMarkedDownActionValue, nil
}

func getHealthCheckDelay(service *v1.Service) (time.Duration, error) {
	healthCheckDelay, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckDelay]
	if !ok {
		return time.ParseDuration("10s")
	}

	healthCheckDelayDuration, err := time.ParseDuration(healthCheckDelay)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerHealthCheckDelay)
		return time.Duration(0), errLoadBalancerInvalidAnnotation
	}

	return healthCheckDelayDuration, nil
}

func getHealthCheckTimeout(service *v1.Service) (time.Duration, error) {
	healthCheckTimeout, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckTimeout]
	if !ok {
		return time.ParseDuration("10s")
	}

	healthCheckTimeoutDuration, err := time.ParseDuration(healthCheckTimeout)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerHealthCheckTimeout)
		return time.Duration(0), errLoadBalancerInvalidAnnotation
	}

	return healthCheckTimeoutDuration, nil
}

func getHealthCheckMaxRetries(service *v1.Service) (int32, error) {
	healthCheckMaxRetries, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckMaxRetries]
	if !ok {
		return 10, nil
	}

	healthCheckMaxRetriesInt, err := strconv.Atoi(healthCheckMaxRetries)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerHealthCheckMaxRetries)
		return 0, errLoadBalancerInvalidAnnotation
	}

	return int32(healthCheckMaxRetriesInt), nil
}

func getForceInternalIP(service *v1.Service) bool {
	forceInternalIP, ok := service.Annotations[serviceAnnotationLoadBalancerForceInternalIP]
	if !ok {
		return false
	}
	value, err := strconv.ParseBool(forceInternalIP)
	if err != nil {
		return false
	}
	return value
}

func getUseHostname(service *v1.Service) bool {
	useHostname, ok := service.Annotations[serviceAnnotationLoadBalancerUseHostname]
	if !ok {
		return false
	}
	value, err := strconv.ParseBool(useHostname)
	if err != nil {
		return false
	}
	return value
}

func getForwardProtocol(service *v1.Service, nodePort int32) (scwlb.Protocol, error) {
	httpProtocol := service.Annotations[serviceAnnotationLoadBalancerProtocolHTTP]

	var svcPort int32 = -1
	for _, p := range service.Spec.Ports {
		if p.NodePort == nodePort {
			svcPort = p.Port
		}
	}
	if svcPort == -1 {
		klog.Errorf("no valid port found")
		return "", errLoadBalancerInvalidAnnotation
	}

	isHTTP, err := isPortInRange(httpProtocol, svcPort)
	if err != nil {
		klog.Errorf("unable to check if port %d is in range %s", svcPort, httpProtocol)
		return "", err
	}

	if isHTTP {
		return scwlb.ProtocolHTTP, nil
	}

	return scwlb.ProtocolTCP, nil
}

func getCertificateIDs(service *v1.Service, port int32) ([]string, error) {
	certificates := service.Annotations[serviceAnnotationLoadBalancerCertificateIDs]
	if certificates == "" {
		return nil, nil
	}

	ids := []string{}

	for _, perPortCertificate := range strings.Split(certificates, ";") {
		split := strings.Split(perPortCertificate, ":")
		if len(split) == 1 {
			ids = append(ids, strings.Split(split[0], ",")...)
			continue
		}
		inRange, err := isPortInRange(split[0], port)
		if err != nil {
			klog.Errorf("unable to check if port %d is in range %s", port, split[0])
			return nil, err
		}
		if inRange {
			ids = append(ids, strings.Split(split[1], ",")...)
		}
	}
	// normalize the ids (ie strip the region prefix if any)
	for i := range ids {
		if strings.Contains(ids[i], "/") {
			splitID := strings.Split(ids[i], "/")
			if len(splitID) != 2 {
				klog.Errorf("unable to get certificate ID from %s", ids[i])
				return nil, fmt.Errorf("unable to get certificate ID from %s", ids[i])
			}
			ids[i] = splitID[1]
		}
	}

	return ids, nil
}

func getValueForPort(service *v1.Service, nodePort int32, fullValue string) (string, error) {
	var svcPort int32 = -1
	for _, p := range service.Spec.Ports {
		if p.NodePort == nodePort {
			svcPort = p.Port
		}
	}

	value := ""

	for _, perPort := range strings.Split(fullValue, ";") {
		split := strings.Split(perPort, ":")
		if len(split) == 1 {
			if value == "" {
				value = split[0]
			}
			continue
		}
		if len(split) > 2 {
			return "", fmt.Errorf("annotation with value %s is wrongly formatted, should be `port1:value1;port2,port3:value2`", fullValue)
		}
		inRange, err := isPortInRange(split[0], svcPort)
		if err != nil {
			klog.Errorf("unable to check if port %d is in range %s", svcPort, split[0])
			return "", err
		}
		if inRange {
			value = split[1]
		}
	}

	return value, nil
}

func getHealthCheckType(service *v1.Service, nodePort int32) (string, error) {
	annotation, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckType]
	if !ok {
		return "tcp", nil
	}

	hcValue, err := getValueForPort(service, nodePort, annotation)
	if err != nil {
		klog.Errorf("could not get value for annotation %s and port %d", serviceAnnotationLoadBalancerHealthCheckType, nodePort)
		return "", err
	}

	return hcValue, nil
}

func getRedisHealthCheck(service *v1.Service, nodePort int32) (*scwlb.HealthCheckRedisConfig, error) {
	return &scwlb.HealthCheckRedisConfig{}, nil
}

func getLdapHealthCheck(service *v1.Service, nodePort int32) (*scwlb.HealthCheckLdapConfig, error) {
	return &scwlb.HealthCheckLdapConfig{}, nil
}

func getTCPHealthCheck(service *v1.Service, nodePort int32) (*scwlb.HealthCheckTCPConfig, error) {
	return &scwlb.HealthCheckTCPConfig{}, nil
}

func getPgsqlHealthCheck(service *v1.Service, nodePort int32) (*scwlb.HealthCheckPgsqlConfig, error) {
	annotation, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckPgsqlUser]
	if !ok {
		return nil, nil
	}

	user, err := getValueForPort(service, nodePort, annotation)
	if err != nil {
		klog.Errorf("could not get value for annotation %s and port %d", serviceAnnotationLoadBalancerHealthCheckPgsqlUser, nodePort)
		return nil, err
	}

	return &scwlb.HealthCheckPgsqlConfig{
		User: user,
	}, nil
}

func getMysqlHealthCheck(service *v1.Service, nodePort int32) (*scwlb.HealthCheckMysqlConfig, error) {
	annotation, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckMysqlUser]
	if !ok {
		return nil, nil
	}

	user, err := getValueForPort(service, nodePort, annotation)
	if err != nil {
		klog.Errorf("could not get value for annotation %s and port %d", serviceAnnotationLoadBalancerHealthCheckMysqlUser, nodePort)
		return nil, err
	}

	return &scwlb.HealthCheckMysqlConfig{
		User: user,
	}, nil
}

func getHTTPHealthCheckCode(service *v1.Service, nodePort int32) (int32, error) {
	annotation, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckHTTPCode]
	if !ok {
		return 200, nil
	}

	stringCode, err := getValueForPort(service, nodePort, annotation)
	if err != nil {
		klog.Errorf("could not get value for annotation %s and port %d", serviceAnnotationLoadBalancerHealthCheckHTTPCode, nodePort)
		return 0, err
	}

	code, err := strconv.Atoi(stringCode)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerHealthCheckHTTPCode)
		return 0, errLoadBalancerInvalidAnnotation
	}

	return int32(code), nil
}

func getHTTPHealthCheckURI(service *v1.Service, nodePort int32) (string, error) {
	annotation, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckHTTPURI]
	if !ok {
		return "/", nil
	}

	uri, err := getValueForPort(service, nodePort, annotation)
	if err != nil {
		klog.Errorf("could not get value for annotation %s and port %d", serviceAnnotationLoadBalancerHealthCheckHTTPURI, nodePort)
		return "", err
	}

	return uri, nil
}

func getHTTPHealthCheckMethod(service *v1.Service, nodePort int32) (string, error) {
	annotation, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckHTTPMethod]
	if !ok {
		return "GET", nil
	}

	method, err := getValueForPort(service, nodePort, annotation)
	if err != nil {
		klog.Errorf("could not get value for annotation %s and port %d", serviceAnnotationLoadBalancerHealthCheckHTTPMethod, nodePort)
		return "", err
	}

	return method, nil
}

func getHTTPHealthCheck(service *v1.Service, nodePort int32) (*scwlb.HealthCheckHTTPConfig, error) {
	code, err := getHTTPHealthCheckCode(service, nodePort)
	if err != nil {
		return nil, err
	}
	uri, err := getHTTPHealthCheckURI(service, nodePort)
	if err != nil {
		return nil, err
	}
	method, err := getHTTPHealthCheckMethod(service, nodePort)
	if err != nil {
		return nil, err
	}

	return &scwlb.HealthCheckHTTPConfig{
		Method: method,
		Code:   &code,
		URI:    uri,
	}, nil
}

func getHTTPSHealthCheck(service *v1.Service, nodePort int32) (*scwlb.HealthCheckHTTPSConfig, error) {
	code, err := getHTTPHealthCheckCode(service, nodePort)
	if err != nil {
		return nil, err
	}
	uri, err := getHTTPHealthCheckURI(service, nodePort)
	if err != nil {
		return nil, err
	}
	method, err := getHTTPHealthCheckMethod(service, nodePort)
	if err != nil {
		return nil, err
	}

	return &scwlb.HealthCheckHTTPSConfig{
		Method: method,
		Code:   &code,
		URI:    uri,
	}, nil
}
