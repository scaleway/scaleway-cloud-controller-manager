# Scaleway LoadBalancer Annotations

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

You can get a list of working annotation on in the Scaleway loadBalancer [documentation](https://www.scaleway.com/en/developers/api/load-balancer/zoned-api/) annotations are:

Note:
- If an invalid mode is passed in the annotation, the service will throw an error.
 If an annotation is not specified, the cloud controller manager will apply default configuration.

### `service.beta.kubernetes.io/scw-loadbalancer-id`
This annotation is the ID of the loadbalancer to use. It is populated by the CCM with the new LB ID if the annotation does not exist.
It has the form `<zone>/<lb-id>`.

### `service.beta.kubernetes.io/scw-loadbalancer-forward-port-algorithm`
This is the annotation to choose the load balancing algorithm.
The default value is `roundrobin` and the possible values are `roundrobin`, `leastconn` and `first`.

### `service.beta.kubernetes.io/scw-loadbalancer-sticky-sessions`
This is the annotation to enable cookie-based session persistence.
The defaut value is `none` and the possible valuea are `none`, `cookie`, or `table`.
NB: If the value `cookie` is used, the annotation `service.beta.kubernetes.io/scw-loadbalancer-sticky-sessions-cookie-name` must be set.

### `service.beta.kubernetes.io/scw-loadbalancer-sticky-sessions-cookie-name`
This is the annotation for the cookie name for sticky sessions.
NB: muste be set if `service.beta.kubernetes.io/scw-loadbalancer-sticky-sessions` is set to `cookie`.

### `service.beta.kubernetes.io/scw-loadbalancer-health-check-type`
This is the type of health check used.
The default value is `tcp` and the possible values are `tcp`, `http`, `https`, `mysql`, `pgsql`, `redis` or `ldap`.
It is possible to set the type per port, like `80:http;443,8443:https`.
NB: depending on the type, some other annotations are required, see below.

### `service.beta.kubernetes.io/scw-loadbalancer-health-check-delay`
This is the annotation to set the time between two consecutive health checks.
The default value is `5s`. The duration are go's time.Duration (ex: `1s`, `2m`, `4h`, ...).

### `service.beta.kubernetes.io/scw-loadbalancer-health-transient-check-delay`
This is the annotation to set the time between two consecutive health checks in a transient state (going UP or DOWN).
The default value is `0.5s`. The duration are go's time.Duration (ex: `1s`, `2m`, `4h`, ...).

### `service.beta.kubernetes.io/scw-loadbalancer-health-check-timeout`
This is the annotaton to set the additional check timeout, after the connection has been already established.
The default value is `5s`. The duration are go's time.Duration (ex: `1s`, `2m`, `4h`, ...).

### `service.beta.kubernetes.io/scw-loadbalancer-health-check-max-retries`
This is the annotation to set the number of consecutive unsuccessful health checks, after wich the server will be considered dead.
The default value is `5`.

### `service.beta.kubernetes.io/scw-loadbalancer-health-check-http-uri`
This is the annotation to set the URI that is used by the `http` health check.
It is possible to set the uri per port, like `80:/;443,8443:/healthz`.
NB: Required when setting service.beta.kubernetes.io/scw-loadbalancer-health-check-type to `http` or `https`.

### `service.beta.kubernetes.io/scw-loadbalancer-health-check-http-method`
This is the annotation to set the HTTP method used by the `http` health check.
It is possible to set the method per port, like `80:GET;443,8443:POST`.
NB: Required when setting service.beta.kubernetes.io/scw-loadbalancer-health-check-type to `http` or `https`.

### `service.beta.kubernetes.io/scw-loadbalancer-health-check-http-code`
This is the annotation to set the HTTP code that the `http` health check will be matching against.
It is possible to set the code per port, like `80:404;443,8443:204`.
NB: Required when setting service.beta.kubernetes.io/scw-loadbalancer-health-check-type to `http` or `https`.

### `service.beta.kubernetes.io/scw-loadbalancer-health-check-mysql-user`
This is the annotation to set the MySQL user used to check the MySQL connection when using the `mysql` health check,
It is possible to set the user per port, like `1234:root;3306,3307:mysql`.
NB: Required when setting service.beta.kubernetes.io/scw-loadbalancer-health-check-type to `mysql`.

### `service.beta.kubernetes.io/scw-loadbalancer-health-check-pgsql-user`
This is the annotation to set the PgSQL user used to check the PgSQL connection when using the `pgsql` health check.
It is possible to set the user per port, like `1234:root;3306,3307:mysql`.
NB: Required when setting service.beta.kubernetes.io/scw-loadbalancer-health-check-type to `pgsql`.

### `service.beta.kubernetes.io/scw-loadbalancer-proxy-protocol-v1`
This is the annotation that can enable the PROXY protocol V1.
The possible values are `false`, `true` or `*` for all ports or a comma delimited list of the service port (for instance `80,443`).

### `service.beta.kubernetes.io/scw-loadbalancer-proxy-protocol-v2`
This is the annotation that can enable the PROXY protocol V2.
The possible values are `false`, `true` or `*` for all ports or a comma delimited list of the service port (for instance `80,443`).

### `service.beta.kubernetes.io/scw-loadbalancer-type`
This is the annotation to set the load balancer offer type.

### `service.beta.kubernetes.io/scw-loadbalancer-timeout-client`
This is the annotation to set the maximum client connection inactivity time.
The default value is `10m`. The duration are go's time.Duration (ex: `1s`, `2m`, `4h`, ...).

### `service.beta.kubernetes.io/scw-loadbalancer-timeout-server`
This is the annotation to set the maximum server connection inactivity time.
The default value is `10s`. The duration are go's time.Duration (ex: `1s`, `2m`, `4h`, ...).

### `service.beta.kubernetes.io/scw-loadbalancer-timeout-connect`
This is the annotation to set the maximum initial server connection establishment time.
The default value is `10m`. The duration are go's time.Duration (ex: `1s`, `2m`, `4h`, ...).

### `service.beta.kubernetes.io/scw-loadbalancer-timeout-tunnel`
This is the annotation to set the maximum tunnel inactivity time.
The default value is `10m`. The duration are go's time.Duration (ex: `1s`, `2m`, `4h`, ...).

### `service.beta.kubernetes.io/scw-loadbalancer-on-marked-down-action`
This is the annotation that modifes what occurs when a backend server is marked down.
The default value is `on_marked_down_action_none` and the possible values are `on_marked_down_action_none` and `shutdown_sessions`.

### `service.beta.kubernetes.io/scw-loadbalancer-force-internal-ip`
This is the annotation that force the usage of InternalIP inside the loadbalancer.
Normally, the cloud controller manager use ExternalIP to be nodes region-free (or public InternalIP in case of Baremetal).

### `service.beta.kubernetes.io/scw-loadbalancer-use-hostname`
This is the annotation that force the use of the LB hostname instead of the public IP.
This is useful when it is needed to not bypass the LoadBalacer for traffic coming from the cluster.

### `service.beta.kubernetes.io/scw-loadbalancer-protocol-http`
This is the annotation to set the forward protocol of the LB to HTTP.
The possible values are `false`, `true` or `*` for all ports or a comma delimited list of the service port (for instance `80,443`).
NB: forwarding HTTPS traffic with HTTP protocol enabled will work only if using a certificate, and the LB will send HTTP traffic to the backend.

### `service.beta.kubernetes.io/scw-loadbalancer-certificate-ids`
This is the annotation to choose the the certificate IDs to associate with this LoadBalancer.
The possible format are:
 - `<certificate-id>`: will use this certificate for all frontends
 - `<certificate-id>,<certificate-id>` will use these certificates for all frontends
 - `<port1>:<certificate1-id>,<certificate2-id>;<port2>,<port3>:<certificate3-id>` will use certificate 1 and 2 for frontend with port port1 and certificate3 for frotend with port port2 and port3

### `service.beta.kubernetes.io/scw-loadbalancer-redispatch-attempt-count`
This is the annotation to activate redispatch on another backend server in case of failure
The default value is 0, which disable the redispatch.

### `service.beta.kubernetes.io/scw-loadbalancer-max-retries`
This is the annotation to configure the number of retry on connection failure
The default value is 2.

### `service.beta.kubernetes.io/scw-loadbalancer-private`
This is the annotation to configure the LB to be private or public
The LB will be public if unset or false.
