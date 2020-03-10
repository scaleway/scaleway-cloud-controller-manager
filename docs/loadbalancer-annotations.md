# Scaleway LoadBalancer Annotations

This link defines how LoadBalancer [services](https://kubernetes.io/docs/concepts/services-networking/service/#internal-load-balancer) (`type: LoadBalancer`) annotations are working.

For Scaleway LoadBalancers annotations are prefixed with `service.beta.kubernetes.io/`. For example:

```yaml
kind: Service
apiVersion: v1
metadata:
  name: nginx-service
  annotations:
    service.beta.kubernetes.io/scw-loadbalancer-forward-port-algorithm: "roundrobin"
    service.beta.kubernetes.io/scw-loadbalancer-health-check-delay: "10s"
spec:
  ...
```

## Load balancer properties

You can get a list of working annotation on in the Scaleway loadBalancer [documentation](https://developers.scaleway.com/en/products/lb/api/#post-db0bfe) annotations are:

| Name | Description | 
| ---- | ----------- |
| `service.beta.kubernetes.io/scw-loadbalancer-forward-port-algorithm` | annotation to choose the load balancing algorithm |
| `service.beta.kubernetes.io/scw-loadbalancer-sticky-sessions` | annotation to enable cookie-based session persistence |
| `service.beta.kubernetes.io/scw-loadbalancer-sticky-sessions-cookie-name` | annotation for the cookie name for sticky sessions |
| `service.beta.kubernetes.io/scw-loadbalancer-health-check-type` | health check used |
| `service.beta.kubernetes.io/scw-loadbalancer-health-check-delay` | time between two consecutive health checks |
| `service.beta.kubernetes.io/scw-loadbalancer-health-check-timeout` | additional check timeout, after the connection has been already established |
| `service.beta.kubernetes.io/scw-loadbalancer-health-check-max-retries` | number of consecutive unsuccessful health checks, after wich the server will be considered dead |
| `service.beta.kubernetes.io/scw-loadbalancer-health-check-http-uri` | URI that is used by the "http" health check  |
| `service.beta.kubernetes.io/scw-loadbalancer-health-check-http-method` | method used by the "http" health check |
| `service.beta.kubernetes.io/scw-loadbalancer-health-check-http-code` | HTTP code that the "http" health check will be matching against |
| `service.beta.kubernetes.io/scw-loadbalancer-health-check-mysql-user` | MySQL user used to check the MySQL connection when using the "mysql" health check |
| `service.beta.kubernetes.io/scw-loadbalancer-health-check-pgsql-user` | PgSQL user used to check the PgSQL connection when using the "pgsql" health check |
| `service.beta.kubernetes.io/scw-loadbalancer-send-proxy-v2` | DEPRECATED - annotation that enables PROXY protocol version 2 (must be supported by backend servers) |
| `service.beta.kubernetes.io/scw-loadbalancer-proxy-protocol-v1` | annotation that enables PROXY protocol version 1 (must be supported by backend servers) |
| `service.beta.kubernetes.io/scw-loadbalancer-proxy-protocol-v2` | annotation that enables PROXY protocol version 2 (must be supported by backend servers) |
| `service.beta.kubernetes.io/scw-loadbalancer-timeout-server` | maximum server connection inactivity time |
| `service.beta.kubernetes.io/scw-loadbalancer-timeout-connect` | maximum initical server connection establishment time |
| `service.beta.kubernetes.io/scw-loadbalancer-timeout-tunnel` | maximum tunnel inactivity time |
| `service.beta.kubernetes.io/scw-loadbalancer-type` | load balancer offer type (lb-s, lb-gp-m, lb-gp-l). default: lb-s |
| `service.beta.kubernetes.io/scw-loadbalancer-on-marked-down-action` | annotation that modifes what occurs when a backend server is marked down |

Note:
- If an invalid mode is passed in the annotation, the service will throw an error.
- If an annotation is not specified, the cloud controller manager will apply default configuration.  
