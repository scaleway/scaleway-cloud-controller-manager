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
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/durationpb"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	scwipam "github.com/scaleway/scaleway-sdk-go/api/ipam/v1alpha1"
	scwlb "github.com/scaleway/scaleway-sdk-go/api/lb/v1"
	scwvpc "github.com/scaleway/scaleway-sdk-go/api/vpc/v2"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/scaleway/scaleway-sdk-go/validation"
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
	// The default value is "5s". The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
	serviceAnnotationLoadBalancerHealthCheckDelay = "service.beta.kubernetes.io/scw-loadbalancer-health-check-delay"

	// serviceAnnotationLoadBalancerHealthTransientCheckDelay is the time between two consecutive health checks on transient state (going UP or DOWN)
	// The default value is "0.5s". The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
	serviceAnnotationLoadBalancerHealthTransientCheckDelay = "service.beta.kubernetes.io/scw-loadbalancer-health-transient-check-delay"

	// serviceAnnotationLoadBalancerHealthCheckTimeout is the additional check timeout, after the connection has been already established
	// The default value is "5s". The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
	serviceAnnotationLoadBalancerHealthCheckTimeout = "service.beta.kubernetes.io/scw-loadbalancer-health-check-timeout"

	// serviceAnnotationLoadBalancerHealthCheckMaxRetries is the number of consecutive unsuccessful health checks, after wich the server will be considered dead
	// The default value is "5".
	serviceAnnotationLoadBalancerHealthCheckMaxRetries = "service.beta.kubernetes.io/scw-loadbalancer-health-check-max-retries"

	// serviceAnnotationLoadBalancerHealthCheckHTTPURI is the URI that is used by the "http" health check
	// It is possible to set the uri per port, like "80:/;443,8443:mydomain.tld/healthz"
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

	// serviceAnnotationLoadBalancerTimeoutClient is the maximum client connection inactivity time
	// The default value is "10m". The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
	serviceAnnotationLoadBalancerTimeoutClient = "service.beta.kubernetes.io/scw-loadbalancer-timeout-client"

	// serviceAnnotationLoadBalancerTimeoutServer is the maximum server connection inactivity time
	// The default value is "10s". The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
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
	// This is useful when it is needed to not bypass the LoadBalacer for traffic coming from the cluster
	serviceAnnotationLoadBalancerUseHostname = "service.beta.kubernetes.io/scw-loadbalancer-use-hostname"

	// serviceAnnotationLoadBalancerProtocolHTTP is the annotation to set the forward protocol of the LB to HTTP
	// The possible values are "false", "true" or "*" for all ports or a comma delimited list of the service port
	// (for instance "80,443")
	serviceAnnotationLoadBalancerProtocolHTTP = "service.beta.kubernetes.io/scw-loadbalancer-protocol-http"

	// serviceAnnotationLoadBalancerCertificateIDs is the annotation to choose the certificate IDS to associate
	// with this LoadBalancer.
	// The possible format are:
	// "<certificate-id>": will use this certificate for all frontends
	// "<certificate-id>,<certificate-id>" will use these certificates for all frontends
	// "<port1>:<certificate1-id>,<certificate2-id>;<port2>,<port3>:<certificate3-id>" will use certificate 1 and 2 for frontend with port port1
	// and certificate3 for frotend with port port2 and port3
	serviceAnnotationLoadBalancerCertificateIDs = "service.beta.kubernetes.io/scw-loadbalancer-certificate-ids"

	// serviceAnnotationLoadBalancerTargetNodeLabels is the annotation to target nodes with specific label(s)
	// Expected format: "Key1=Val1,Key2=Val2"
	serviceAnnotationLoadBalancerTargetNodeLabels = "service.beta.kubernetes.io/scw-loadbalancer-target-node-labels"

	// serviceAnnotationLoadBalancerRedispatchAttemptCount is the annotation to activate redispatch on another backend server in case of failure
	// The default value is "0", which disable the redispatch
	serviceAnnotationLoadBalancerRedispatchAttemptCount = "service.beta.kubernetes.io/scw-loadbalancer-redispatch-attempt-count"

	// serviceAnnotationLoadBalancerMaxRetries is the annotation to configure the number of retry on connection failure
	// The default value is 3.
	serviceAnnotationLoadBalancerMaxRetries = "service.beta.kubernetes.io/scw-loadbalancer-max-retries"

	// serviceAnnotationLoadBalancerPrivate is the annotation to configure the LB to be private or public
	// The LB will be public if unset or false.
	serviceAnnotationLoadBalancerPrivate = "service.beta.kubernetes.io/scw-loadbalancer-private"
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

	lbPrivate, err := svcPrivate(service)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerPrivate)
		return nil, fmt.Errorf("invalid value for annotation %s: expected boolean", serviceAnnotationLoadBalancerPrivate)
	}

	if lbPrivate && l.pnID == "" {
		return nil, fmt.Errorf("scaleway-cloud-controller-manager cannot create private load balancers without a private network")
	}
	if lbPrivate && service.Spec.LoadBalancerIP != "" {
		return nil, fmt.Errorf("scaleway-cloud-controller-manager can only handle .spec.LoadBalancerIP for public load balancers. Unsetting the .spec.LoadBalancerIP can result in the loss of the IP")
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
		klog.Errorf("error getting loadbalancer for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil, err
	}

	privateModeMismatch := lbPrivate != (len(lb.IP) == 0)
	reservedIPMismatch := service.Spec.LoadBalancerIP != "" && service.Spec.LoadBalancerIP != lb.IP[0].IPAddress
	if privateModeMismatch || reservedIPMismatch {
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

	return l.deleteLoadBalancer(ctx, lb, service)
}

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
	scwCcmTagsDelimiter := os.Getenv(scwCcmTagsDelimiterEnv)
	if scwCcmTagsDelimiter == "" {
		scwCcmTagsDelimiter = ","
	}

	lbPrivate, err := svcPrivate(service)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerPrivate)
		return nil, fmt.Errorf("invalid value for annotation %s: expected boolean", serviceAnnotationLoadBalancerPrivate)
	}

	var ipID *string
	if !lbPrivate && service.Spec.LoadBalancerIP != "" {
		request := scwlb.ZonedAPIListIPsRequest{
			IPAddress: &service.Spec.LoadBalancerIP,
			Zone:      getLoadBalancerZone(service),
		}
		ipsResp, err := l.api.ListIPs(&request)
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
		Zone:             getLoadBalancerZone(service),
		Name:             lbName,
		Description:      "kubernetes service " + service.Name,
		Tags:             tags,
		IPID:             ipID,
		Type:             lbType,
		AssignFlexibleIP: scw.BoolPtr(!lbPrivate),
	}

	lb, err := l.api.CreateLB(&request)
	if err != nil {
		klog.Errorf("error creating load balancer for service %s/%s: %v", service.Namespace, service.Name, err)
		return nil, fmt.Errorf("error creating load balancer for service %s/%s: %v", service.Namespace, service.Name, err)
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
	nodes = filterNodes(service, nodes)
	if l.pnID != "" {
		respPN, err := l.api.ListLBPrivateNetworks(&scwlb.ZonedAPIListLBPrivateNetworksRequest{
			Zone: loadbalancer.Zone,
			LBID: loadbalancer.ID,
		})
		if err != nil {
			return fmt.Errorf("error listing private networks of load balancer %s: %v", loadbalancer.ID, err)
		}

		var pnNIC *scwlb.PrivateNetwork
		for _, pNIC := range respPN.PrivateNetwork {
			if pNIC.PrivateNetworkID == l.pnID {
				pnNIC = pNIC
				continue
			}

			// this PN should not be attached to this loadbalancer
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

		if pnNIC == nil {
			klog.V(3).Infof("attach private network %s to load balancer %s", l.pnID, loadbalancer.ID)
			_, err = l.api.AttachPrivateNetwork(&scwlb.ZonedAPIAttachPrivateNetworkRequest{
				Zone:             loadbalancer.Zone,
				LBID:             loadbalancer.ID,
				PrivateNetworkID: l.pnID,
				DHCPConfig:       &scwlb.PrivateNetworkDHCPConfig{},
			})
			if err != nil {
				return fmt.Errorf("unable to attach private network %s on %s: %v", l.pnID, loadbalancer.ID, err)
			}
		}
	}

	var targetIPs []string
	if getForceInternalIP(service) || l.pnID != "" {
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

	frontendsOps := compareFrontends(respFrontends.Frontends, svcFrontends)
	backendsOps := compareBackends(respBackends.Backends, svcBackends)

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

// createPrivateServiceStatus creates a LoadBalancer status for services with private load balancers
func (l *loadbalancers) createPrivateServiceStatus(service *v1.Service, lb *scwlb.LB) (*v1.LoadBalancerStatus, error) {
	if l.pnID == "" {
		return nil, fmt.Errorf("cannot make status for service %s/%s: private load balancer requires a private network", service.Namespace, service.Name)
	}

	region, err := lb.Zone.Region()
	if err != nil {
		return nil, fmt.Errorf("error making status for service %s/%s: %v", service.Namespace, service.Name, err)
	}

	status := &v1.LoadBalancerStatus{}

	if getUseHostname(service) {
		pn, err := l.vpc.GetPrivateNetwork(&scwvpc.GetPrivateNetworkRequest{
			Region:           region,
			PrivateNetworkID: l.pnID,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to query private network for lb %s: %v", lb.Name, err)
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
			return nil, fmt.Errorf("unable to query ipam for lb %s: %v", lb.Name, err)
		}

		if len(ipamRes.IPs) == 0 {
			return nil, fmt.Errorf("no private network ip for lb %s", lb.Name)
		}

		status.Ingress = make([]v1.LoadBalancerIngress, len(ipamRes.IPs))
		for idx, ip := range ipamRes.IPs {
			status.Ingress[idx].IP = ip.Address.IP.String()
		}
	}

	return status, nil
}

// createPublicServiceStatus creates a LoadBalancer status for services with public load balancers
func (l *loadbalancers) createPublicServiceStatus(service *v1.Service, lb *scwlb.LB) (*v1.LoadBalancerStatus, error) {
	status := &v1.LoadBalancerStatus{}
	status.Ingress = make([]v1.LoadBalancerIngress, 0)
	for _, ip := range lb.IP {
		// Skip ipv6 entries
		if i := net.ParseIP(ip.IPAddress); i.To4() == nil {
			continue
		}

		if getUseHostname(service) {
			status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{Hostname: ip.Reverse})
		} else {
			status.Ingress = append(status.Ingress, v1.LoadBalancerIngress{IP: ip.IPAddress})
		}
	}

	if len(status.Ingress) == 0 {
		return nil, fmt.Errorf("no ipv4 found for lb %s", lb.Name)
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

	if lbPrivate {
		return l.createPrivateServiceStatus(service, lb)
	}

	return l.createPublicServiceStatus(service, lb)
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

	if forwardPortAlgorithmValue != scwlb.ForwardPortAlgorithmRoundrobin && forwardPortAlgorithmValue != scwlb.ForwardPortAlgorithmLeastconn && forwardPortAlgorithmValue != scwlb.ForwardPortAlgorithmFirst {
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

func getTimeoutClient(service *v1.Service) (time.Duration, error) {
	timeoutClient, ok := service.Annotations[serviceAnnotationLoadBalancerTimeoutClient]
	if !ok {
		return time.ParseDuration("10m")
	}

	timeoutClientDuration, err := time.ParseDuration(timeoutClient)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerTimeoutClient)
		return time.Duration(0), errLoadBalancerInvalidAnnotation
	}

	return timeoutClientDuration, nil
}

func getTimeoutServer(service *v1.Service) (time.Duration, error) {
	timeoutServer, ok := service.Annotations[serviceAnnotationLoadBalancerTimeoutServer]
	if !ok {
		return time.ParseDuration("10s")
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

func getRedisatchAttemptCount(service *v1.Service) (*int32, error) {
	redispatchAttemptCount, ok := service.Annotations[serviceAnnotationLoadBalancerRedispatchAttemptCount]
	if !ok {
		var v int32 = 0
		return &v, nil
	}
	redispatchAttemptCountInt, err := strconv.Atoi(redispatchAttemptCount)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerRedispatchAttemptCount)
		return nil, errLoadBalancerInvalidAnnotation

	}
	redispatchAttemptCountInt32 := int32(redispatchAttemptCountInt)
	return &redispatchAttemptCountInt32, nil
}

func getMaxRetries(service *v1.Service) (*int32, error) {
	maxRetriesCount, ok := service.Annotations[serviceAnnotationLoadBalancerMaxRetries]
	if !ok {
		var v int32 = 3
		return &v, nil
	}
	maxRetriesCountInt, err := strconv.Atoi(maxRetriesCount)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerMaxRetries)
		return nil, errLoadBalancerInvalidAnnotation

	}
	maxRetriesCountInt32 := int32(maxRetriesCountInt)
	return &maxRetriesCountInt32, nil
}

func getHealthCheckDelay(service *v1.Service) (time.Duration, error) {
	healthCheckDelay, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckDelay]
	if !ok {
		return time.ParseDuration("5s")
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
		return time.ParseDuration("5s")
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
		return 5, nil
	}

	healthCheckMaxRetriesInt, err := strconv.Atoi(healthCheckMaxRetries)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerHealthCheckMaxRetries)
		return 0, errLoadBalancerInvalidAnnotation
	}

	return int32(healthCheckMaxRetriesInt), nil
}

func getHealthCheckTransientCheckDelay(service *v1.Service) (*scw.Duration, error) {
	transientCheckDelay, ok := service.Annotations[serviceAnnotationLoadBalancerHealthTransientCheckDelay]
	if !ok {
		return nil, nil
	}
	transientCheckDelayDuration, err := time.ParseDuration(transientCheckDelay)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerHealthTransientCheckDelay)
		return nil, errLoadBalancerInvalidAnnotation
	}

	durationpb := durationpb.New(transientCheckDelayDuration)

	return &scw.Duration{
		Seconds: durationpb.Seconds,
		Nanos:   durationpb.Nanos,
	}, nil
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
	ids := []string{}
	if certificates == "" {
		return ids, nil
	}

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

	uriStr, err := getHTTPHealthCheckURI(service, nodePort)
	if err != nil {
		return nil, err
	}
	uri, err := url.Parse(fmt.Sprintf("http://%s", uriStr))
	if err != nil {
		return nil, err
	}
	if uri.Path == "" {
		uri.Path = "/"
	}

	method, err := getHTTPHealthCheckMethod(service, nodePort)
	if err != nil {
		return nil, err
	}

	return &scwlb.HealthCheckHTTPConfig{
		Method:     method,
		Code:       &code,
		URI:        uri.RequestURI(),
		HostHeader: uri.Host,
	}, nil
}

func getHTTPSHealthCheck(service *v1.Service, nodePort int32) (*scwlb.HealthCheckHTTPSConfig, error) {
	code, err := getHTTPHealthCheckCode(service, nodePort)
	if err != nil {
		return nil, err
	}

	uriStr, err := getHTTPHealthCheckURI(service, nodePort)
	if err != nil {
		return nil, err
	}
	uri, err := url.Parse(fmt.Sprintf("https://%s", uriStr))
	if err != nil {
		return nil, err
	}
	if uri.Path == "" {
		uri.Path = "/"
	}

	method, err := getHTTPHealthCheckMethod(service, nodePort)
	if err != nil {
		return nil, err
	}

	return &scwlb.HealthCheckHTTPSConfig{
		Method:     method,
		Code:       &code,
		URI:        uri.Path,
		HostHeader: uri.Host,
		Sni:        uri.Host,
	}, nil
}

func svcPrivate(service *v1.Service) (bool, error) {
	isPrivate, ok := service.Annotations[serviceAnnotationLoadBalancerPrivate]
	if !ok {
		return false, nil
	}
	return strconv.ParseBool(isPrivate)
}

// Original version: https://github.com/kubernetes/legacy-cloud-providers/blob/1aa918bf227e52af6f8feb3fa065dabff251a0a3/aws/aws_loadbalancer.go#L117
func getKeyValueFromAnnotation(annotation string) map[string]string {
	additionalTags := make(map[string]string)
	additionalTagsList := strings.TrimSpace(annotation)

	// Break up list of "Key1=Val,Key2=Val2"
	tagList := strings.Split(additionalTagsList, ",")

	// Break up "Key=Val"
	for _, tagSet := range tagList {
		tag := strings.Split(strings.TrimSpace(tagSet), "=")

		// Accept "Key=val" or "Key=" or just "Key"
		if len(tag) >= 2 && len(tag[0]) != 0 {
			// There is a key and a value, so save it
			additionalTags[tag[0]] = tag[1]
		} else if len(tag) == 1 && len(tag[0]) != 0 {
			// Just "Key"
			additionalTags[tag[0]] = ""
		}
	}

	return additionalTags
}

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

	return &scwlb.Frontend{
		Name:           fmt.Sprintf("%s_tcp_%d", string(service.UID), port.Port),
		InboundPort:    port.Port,
		TimeoutClient:  &timeoutClient,
		CertificateIDs: certificateIDs,
	}, nil
}

func servicePortToBackend(service *v1.Service, loadbalancer *scwlb.LB, port v1.ServicePort, nodeIPs []string) (*scwlb.Backend, error) {
	protocol, err := getForwardProtocol(service, port.NodePort)
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

	onMarkedDownAction, err := getOnMarkedDownAction(service)
	if err != nil {
		return nil, err
	}

	redispatchAttemptCount, err := getRedisatchAttemptCount(service)
	if err != nil {
		return nil, err
	}

	maxRetries, err := getMaxRetries(service)
	if err != nil {
		return nil, err
	}

	healthCheck := &scwlb.HealthCheck{
		Port: port.NodePort,
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

	backend := &scwlb.Backend{
		Name:                   fmt.Sprintf("%s_tcp_%d", string(service.UID), port.NodePort),
		Pool:                   nodeIPs,
		ForwardPort:            port.NodePort,
		ForwardProtocol:        protocol,
		ForwardPortAlgorithm:   forwardPortAlgorithm,
		StickySessions:         stickySessions,
		ProxyProtocol:          proxyProtocol,
		TimeoutServer:          &timeoutServer,
		TimeoutConnect:         &timeoutConnect,
		TimeoutTunnel:          &timeoutTunnel,
		OnMarkedDownAction:     onMarkedDownAction,
		HealthCheck:            healthCheck,
		RedispatchAttemptCount: redispatchAttemptCount,
		MaxRetries:             maxRetries,
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

	return true
}

func backendEquals(got, want *scwlb.Backend) bool {
	if got == nil || want == nil {
		return got == want
	}

	if got.Name != want.Name {
		klog.V(3).Infof("backend.Name: %s - %s", got.Name, want.Name)
		return false
	}
	if got.ForwardPort != want.ForwardPort {
		klog.V(3).Infof("backend.ForwardPort: %d - %d", got.ForwardPort, want.ForwardPort)
		return false
	}
	if got.ForwardProtocol != want.ForwardProtocol {
		klog.V(3).Infof("backend.ForwardProtocol: %s - %s", got.ForwardProtocol, want.ForwardProtocol)
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
	if got.OnMarkedDownAction != want.OnMarkedDownAction {
		klog.V(3).Infof("backend.OnMarkedDownAction: %s - %s", got.OnMarkedDownAction, want.OnMarkedDownAction)
		return false
	}
	if !int32PtrEqual(got.RedispatchAttemptCount, want.RedispatchAttemptCount) {
		klog.V(3).Infof("backend.RedispatchAttemptCount: %s - %s", ptrInt32ToString(got.RedispatchAttemptCount), ptrInt32ToString(want.RedispatchAttemptCount))
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

func compareFrontends(got []*scwlb.Frontend, want map[int32]*scwlb.Frontend) frontendOps {
	remove := make(map[int32]*scwlb.Frontend)
	update := make(map[int32]*scwlb.Frontend)
	create := make(map[int32]*scwlb.Frontend)
	keep := make(map[int32]*scwlb.Frontend)

	// Check for deletions and updates
	for _, current := range got {
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
		for _, current := range got {
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

func compareBackends(got []*scwlb.Backend, want map[int32]*scwlb.Backend) backendOps {
	remove := make(map[int32]*scwlb.Backend)
	update := make(map[int32]*scwlb.Backend)
	create := make(map[int32]*scwlb.Backend)
	keep := make(map[int32]*scwlb.Backend)

	// Check for deletions and updates
	for _, current := range got {
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
		for _, current := range got {
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

func aclsEquals(got []*scwlb.ACL, want []*scwlb.ACLSpec) bool {
	if len(got) != len(want) {
		return false
	}

	slices.SortStableFunc(got, func(a, b *scwlb.ACL) bool { return a.Index < b.Index })
	slices.SortStableFunc(want, func(a, b *scwlb.ACLSpec) bool { return a.Index < b.Index })
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

func (l *loadbalancers) createBackend(service *v1.Service, loadbalancer *scwlb.LB, backend *scwlb.Backend) (*scwlb.Backend, error) {
	b, err := l.api.CreateBackend(&scwlb.ZonedAPICreateBackendRequest{
		Zone:                     loadbalancer.Zone,
		LBID:                     loadbalancer.ID,
		Name:                     backend.Name,
		ForwardProtocol:          backend.ForwardProtocol,
		ForwardPort:              backend.ForwardPort,
		ForwardPortAlgorithm:     backend.ForwardPortAlgorithm,
		StickySessions:           backend.StickySessions,
		StickySessionsCookieName: backend.StickySessionsCookieName,
		HealthCheck:              backend.HealthCheck,
		ServerIP:                 backend.Pool,
		TimeoutServer:            backend.TimeoutServer,
		TimeoutConnect:           backend.TimeoutConnect,
		TimeoutTunnel:            backend.TimeoutTunnel,
		OnMarkedDownAction:       backend.OnMarkedDownAction,
		ProxyProtocol:            backend.ProxyProtocol,
		RedispatchAttemptCount:   backend.RedispatchAttemptCount,
		MaxRetries:               backend.MaxRetries,
	})
	if err != nil {
		return nil, err
	}

	return b, nil
}

func (l *loadbalancers) updateBackend(service *v1.Service, loadbalancer *scwlb.LB, backend *scwlb.Backend) (*scwlb.Backend, error) {
	b, err := l.api.UpdateBackend(&scwlb.ZonedAPIUpdateBackendRequest{
		Zone:                     loadbalancer.Zone,
		BackendID:                backend.ID,
		Name:                     backend.Name,
		ForwardProtocol:          backend.ForwardProtocol,
		ForwardPort:              backend.ForwardPort,
		ForwardPortAlgorithm:     backend.ForwardPortAlgorithm,
		StickySessions:           backend.StickySessions,
		StickySessionsCookieName: backend.StickySessionsCookieName,
		TimeoutServer:            backend.TimeoutServer,
		TimeoutConnect:           backend.TimeoutConnect,
		TimeoutTunnel:            backend.TimeoutTunnel,
		OnMarkedDownAction:       backend.OnMarkedDownAction,
		ProxyProtocol:            backend.ProxyProtocol,
		RedispatchAttemptCount:   backend.RedispatchAttemptCount,
		MaxRetries:               backend.MaxRetries,
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

func (l *loadbalancers) createFrontend(service *v1.Service, loadbalancer *scwlb.LB, frontend *scwlb.Frontend, backend *scwlb.Backend) (*scwlb.Frontend, error) {
	f, err := l.api.CreateFrontend(&scwlb.ZonedAPICreateFrontendRequest{
		Zone:           loadbalancer.Zone,
		LBID:           loadbalancer.ID,
		Name:           frontend.Name,
		InboundPort:    frontend.InboundPort,
		BackendID:      backend.ID,
		TimeoutClient:  frontend.TimeoutClient,
		CertificateIDs: &frontend.CertificateIDs,
		EnableHTTP3:    frontend.EnableHTTP3,
	})

	return f, err
}

func (l *loadbalancers) updateFrontend(service *v1.Service, loadbalancer *scwlb.LB, frontend *scwlb.Frontend, backend *scwlb.Backend) (*scwlb.Frontend, error) {
	f, err := l.api.UpdateFrontend(&scwlb.ZonedAPIUpdateFrontendRequest{
		Zone:           loadbalancer.Zone,
		FrontendID:     frontend.ID,
		Name:           frontend.Name,
		InboundPort:    frontend.InboundPort,
		BackendID:      backend.ID,
		TimeoutClient:  frontend.TimeoutClient,
		CertificateIDs: &frontend.CertificateIDs,
		EnableHTTP3:    frontend.EnableHTTP3,
	})

	return f, err
}

func stringArrayEqual(got, want []string) bool {
	slices.Sort(got)
	slices.Sort(want)
	return reflect.DeepEqual(got, want)
}

func stringPtrArrayEqual(got, want []*string) bool {
	slices.SortStableFunc(got, func(a, b *string) bool { return *a < *b })
	slices.SortStableFunc(want, func(a, b *string) bool { return *a < *b })
	return reflect.DeepEqual(got, want)
}

func durationPtrEqual(got, want *time.Duration) bool {
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	return *got == *want
}

func scwDurationPtrEqual(got, want *scw.Duration) bool {
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	return *got == *want
}

func int32PtrEqual(got, want *int32) bool {
	if got == nil && want == nil {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	return *got == *want
}

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
