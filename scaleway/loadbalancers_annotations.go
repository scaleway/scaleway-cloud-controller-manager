package scaleway

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	scwlb "github.com/scaleway/scaleway-sdk-go/api/lb/v1"
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

	// serviceAnnotationLoadBalancerHealthCheckSendProxy is the annotation to control if proxy protocol should be activated for the health check.
	// The default value is "false"
	serviceAnnotationLoadBalancerHealthCheckSendProxy = "service.beta.kubernetes.io/scw-loadbalancer-health-check-send-proxy"

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

	// serviceAnnotationLoadBalancerConnectionRateLimit is the annotation to set the incoming connection rate limit per second.
	// Set to 0 to disable the rate limit
	serviceAnnotationLoadBalancerConnectionRateLimit = "service.beta.kubernetes.io/scw-loadbalancer-connection-rate-limit"

	// serviceAnnotationLoadBalancerEnableAccessLogs is the annotation to enable access logs for the load balancer.
	// The default value is "false". The possible values are "false" or "true".
	serviceAnnotationLoadBalancerEnableAccessLogs = "service.beta.kubernetes.io/scw-loadbalancer-enable-access-logs"

	// serviceAnnotationLoadBalancerEnableHTTP3 is the annotation to enable HTTP/3 protocol for the load balancer.
	// The default value is "false". The possible values are "false" or "true".
	serviceAnnotationLoadBalancerEnableHTTP3 = "service.beta.kubernetes.io/scw-loadbalancer-enable-http3"

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

	// serviceAnnotationLoadBalancerTimeoutQueue is the maximum time for a request to be left pending in queue when max_connections is reached.
	// The duration are go's time.Duration (ex: "1s", "2m", "4h", ...)
	serviceAnnotationLoadBalancerTimeoutQueue = "service.beta.kubernetes.io/scw-loadbalancer-timeout-queue"

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

	// serviceAnnotationLoadBalancerHTTPBackendTLS is the annotation to enable tls towards the backend when using http forward protocol.
	// Default to "false". The possible values are "false", "true" or "*" for all ports or a comma delimited list of the service port
	// (for instance "80,443")
	serviceAnnotationLoadBalancerHTTPBackendTLS = "service.beta.kubernetes.io/scw-loadbalancer-http-backend-tls"

	// serviceAnnotationLoadBalancerHTTPBackendTLSSkipVerify is the annotation to skip tls verification on backends when using http forward protocol with TLS enabled
	// The possible values are "false", "true" or "*" for all ports or a comma delimited list of the service port
	// (for instance "80,443")
	serviceAnnotationLoadBalancerHTTPBackendTLSSkipVerify = "service.beta.kubernetes.io/scw-loadbalancer-http-backend-tls-skip-verify"

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

	// serviceAnnotationLoadBalancerMaxConnections is the annotation to configure the number of connections
	serviceAnnotationLoadBalancerMaxConnections = "service.beta.kubernetes.io/scw-loadbalancer-max-connections"

	// serviceAnnotationLoadBalancerMaxRetries is the annotation to configure the number of retry on connection failure
	// The default value is 3.
	serviceAnnotationLoadBalancerMaxRetries = "service.beta.kubernetes.io/scw-loadbalancer-max-retries"

	// serviceAnnotationLoadBalancerFailoverHost is the annotation to specify the Scaleway Object Storage bucket website to be served as failover if all backend servers are down, e.g. failover-website.s3-website.fr-par.scw.cloud.
	serviceAnnotationLoadBalancerFailoverHost = "service.beta.kubernetes.io/scw-loadbalancer-failover-host"

	// serviceAnnotationLoadBalancerPrivate is the annotation to configure the LB to be private or public
	// The LB will be public if unset or false.
	serviceAnnotationLoadBalancerPrivate = "service.beta.kubernetes.io/scw-loadbalancer-private"

	// serviceAnnotationLoadBalancerExternallyManaged is the annotation that makes the following changes in behavior:
	// * Won't create/delete the LB.
	// * Ignores the global configurations (such as size, private mode, IPs).
	// * Won't detach other private networks attached to the LB.
	// * won't manage extra frontends and backends not starting with the service id.
	// * Will refuse to manage a LB with a name starting with the cluster id.
	// This annotation requires `service.beta.kubernetes.io/scw-loadbalancer-id` to be set to a valid existing LB.
	serviceAnnotationLoadBalancerExternallyManaged = "service.beta.kubernetes.io/scw-loadbalancer-externally-managed"

	// serviceAnnotationLoadBalancerIPIDs is the annotation to statically set the IPs of the loadbalancer.
	// It is possible to provide a single IP ID, or a comma delimited list of IP IDs.
	// You can provide at most one IPv4 and one IPv6. You must set at least one IPv4.
	// This annotation takes priority over the deprecated spec.loadBalancerIP field.
	// Changing the IPs will result in the re-creation of the LB.
	// The possible formats are:
	// "<ip-id>": will attach a single IP to the LB.
	// "<ip-id>,<ip-id>": will attach the two IPs to the LB.
	serviceAnnotationLoadBalancerIPIDs = "service.beta.kubernetes.io/scw-loadbalancer-ip-ids"

	// serviceAnnotationLoadBalancerIPMode is the annotation to manually set the
	// .status.loadBalancer.ingress.ipMode field of the service. The accepted values
	// are "Proxy" and "VIP". Please refer to this article for more information about IPMode:
	// https://kubernetes.io/blog/2023/12/18/kubernetes-1-29-feature-loadbalancer-ip-mode-alpha/.
	// When proxy-protocol is enabled on ALL the ports of the service, the ipMode
	// is automatically set to "Proxy". You can use this annotation to override this.
	serviceAnnotationLoadBalancerIPMode = "service.beta.kubernetes.io/scw-loadbalancer-ip-mode"

	// serviceAnnotationPrivateNetworkIDs is the annotation to configure the Private Networks
	// that will be attached to the load balancer. It is possible to provide a single
	// Private Network ID, or a comma delimited list of Private Network IDs.
	// If this annotation is not set or empty, the load balancer will be attached
	// to the Private Network specified in the `PN_ID` environment variable.
	// This annotation is ignored when service.beta.kubernetes.io/scw-loadbalancer-externally-managed is enabled.
	//
	// The possible formats are:
	//	- "<pn-id>": will attach a single Private Network to the LB.
	//	- "<pn-id>,<pn-id>": will attach the two Private Networks to the LB.
	serviceAnnotationPrivateNetworkIDs = "service.beta.kubernetes.io/scw-loadbalancer-pn-ids"
)

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

func getIPIDs(service *v1.Service) []string {
	ipIDs := service.Annotations[serviceAnnotationLoadBalancerIPIDs]
	if ipIDs == "" {
		return nil
	}

	return strings.Split(ipIDs, ",")
}

func getPrivateNetworkIDs(service *v1.Service) []string {
	pnIDs := service.Annotations[serviceAnnotationPrivateNetworkIDs]
	if pnIDs == "" {
		return nil
	}

	return strings.Split(pnIDs, ",")
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

func getTimeoutQueue(service *v1.Service) (*scw.Duration, error) {
	timeoutQueue, ok := service.Annotations[serviceAnnotationLoadBalancerTimeoutQueue]
	if !ok {
		return nil, nil
	}

	timeoutQueueDuration, err := time.ParseDuration(timeoutQueue)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerTimeoutQueue)
		return nil, errLoadBalancerInvalidAnnotation
	}

	return scw.NewDurationFromTimeDuration(timeoutQueueDuration), nil
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

func getMaxConnections(service *v1.Service) (*int32, error) {
	maxConnectionsCount, ok := service.Annotations[serviceAnnotationLoadBalancerMaxConnections]
	if !ok {
		return nil, nil
	}
	maxConnectionsCountInt, err := strconv.Atoi(maxConnectionsCount)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerMaxConnections)
		return nil, errLoadBalancerInvalidAnnotation

	}
	maxConnectionsCountInt32 := int32(maxConnectionsCountInt)
	return &maxConnectionsCountInt32, nil
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

func getFailoverHost(service *v1.Service) (*string, error) {
	failoverHost, ok := service.Annotations[serviceAnnotationLoadBalancerFailoverHost]
	if !ok {
		return nil, nil
	}
	return &failoverHost, nil
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

func getHealthCheckSendProxy(service *v1.Service) (bool, error) {
	sendProxy, ok := service.Annotations[serviceAnnotationLoadBalancerHealthCheckSendProxy]
	if !ok {
		return false, nil
	}
	sendProxyBool, err := strconv.ParseBool(sendProxy)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerHealthCheckSendProxy)
		return false, errLoadBalancerInvalidAnnotation
	}

	return sendProxyBool, nil
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

func getSSLBridging(service *v1.Service, nodePort int32) (bool, error) {
	tlsEnabled, found := service.Annotations[serviceAnnotationLoadBalancerHTTPBackendTLS]
	if !found {
		return false, nil
	}

	var svcPort int32 = -1
	for _, p := range service.Spec.Ports {
		if p.NodePort == nodePort {
			svcPort = p.Port
		}
	}
	if svcPort == -1 {
		klog.Errorf("no valid port found")
		return false, errLoadBalancerInvalidAnnotation
	}

	isTLSEnabled, err := isPortInRange(tlsEnabled, svcPort)
	if err != nil {
		klog.Errorf("unable to check if port %d is in range %s", svcPort, tlsEnabled)
		return false, err
	}

	return isTLSEnabled, nil
}

func getSSLBridgingSkipVerify(service *v1.Service, nodePort int32) (*bool, error) {
	skipTLSVerify, found := service.Annotations[serviceAnnotationLoadBalancerHTTPBackendTLSSkipVerify]
	if !found {
		return nil, nil
	}

	var svcPort int32 = -1
	for _, p := range service.Spec.Ports {
		if p.NodePort == nodePort {
			svcPort = p.Port
		}
	}
	if svcPort == -1 {
		klog.Errorf("no valid port found")
		return nil, errLoadBalancerInvalidAnnotation
	}

	isSkipTLSVerify, err := isPortInRange(skipTLSVerify, svcPort)
	if err != nil {
		klog.Errorf("unable to check if port %d is in range %s", svcPort, skipTLSVerify)
		return nil, err
	}

	return scw.BoolPtr(isSkipTLSVerify), nil
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

func svcExternallyManaged(service *v1.Service) (bool, error) {
	isExternallyManaged, ok := service.Annotations[serviceAnnotationLoadBalancerExternallyManaged]
	if !ok {
		return false, nil
	}
	return strconv.ParseBool(isExternallyManaged)
}

func getLoadBalancerIPMode(service *v1.Service) *v1.LoadBalancerIPMode {
	loadBalancerIPMode, ok := service.Annotations[serviceAnnotationLoadBalancerIPMode]
	if !ok {
		return nil
	}

	return ptr.To(v1.LoadBalancerIPMode(loadBalancerIPMode))
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

func getConnectionRateLimit(service *v1.Service) (*uint32, error) {
	connectionRateLimit, ok := service.Annotations[serviceAnnotationLoadBalancerConnectionRateLimit]
	if !ok {
		return nil, nil
	}
	connectionRateLimitInt, err := strconv.Atoi(connectionRateLimit)
	if err != nil {
		klog.Errorf("invalid value for annotation %s", serviceAnnotationLoadBalancerConnectionRateLimit)
		return nil, errLoadBalancerInvalidAnnotation
	}
	connectionRateLimitUint32 := uint32(connectionRateLimitInt)
	return &connectionRateLimitUint32, nil
}

func getEnableAccessLogs(service *v1.Service) (bool, error) {
	enableAccessLogs, ok := service.Annotations[serviceAnnotationLoadBalancerEnableAccessLogs]
	if !ok {
		return false, nil
	}
	return strconv.ParseBool(enableAccessLogs)
}

func getEnableHTTP3(service *v1.Service) (bool, error) {
	enableHTTP3, ok := service.Annotations[serviceAnnotationLoadBalancerEnableHTTP3]
	if !ok {
		return false, nil
	}
	return strconv.ParseBool(enableHTTP3)
}
