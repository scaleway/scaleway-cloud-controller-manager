# Configuration

`scaleway-cloud-controller-manager` is configured entirely through environment variables.
The example [Secret manifest](../examples/k8s-scaleway-secret.yml) shows how to provide these
values when running on Kubernetes.

## Scaleway credentials and location

These are the standard Scaleway SDK environment variables, used to authenticate and to determine
where resources (load balancers, etc.) are managed.

### `SCW_ACCESS_KEY`
**Required.** The access key of the [API key](https://www.scaleway.com/en/docs/generate-an-api-token/) used by the CCM.

### `SCW_SECRET_KEY`
**Required.** The secret key of the API key used by the CCM.

### `SCW_DEFAULT_REGION`
**Required.** The default region (e.g. `fr-par`, `nl-ams`, `pl-waw`) used for regional resources
such as load balancers. The CCM will fail to start if no region can be determined.

### `SCW_DEFAULT_ZONE`
The default zone (e.g. `fr-par-1`) used for zonal resources such as Instances and Elastic Metal
servers. If not set, the first zone of `SCW_DEFAULT_REGION` is generally used.

### `SCW_DEFAULT_PROJECT_ID`
The default Project ID used when creating resources (e.g. load balancers, IPs).

### `SCW_DEFAULT_ORGANIZATION_ID`
The default Organization ID. Can be used instead of, or in addition to, `SCW_DEFAULT_PROJECT_ID`.

## CCM behavior

### `PN_ID`
The ID of the Private Network to attach managed load balancers to, and to use for routing
traffic to nodes. Can be overridden per-service with the
[`service.beta.kubernetes.io/scw-loadbalancer-pn-ids`](loadbalancer-annotations.md#servicebetakubernetesioscw-loadbalancer-pn-ids)
annotation.

### `LB_DEFAULT_TYPE`
The default load balancer commercial offer type (e.g. `LB-S`, `LB-GP-M`) used when creating new
load balancers. Can be overridden per-service with the
[`service.beta.kubernetes.io/scw-loadbalancer-type`](loadbalancer-annotations.md#servicebetakubernetesioscw-loadbalancer-type)
annotation. If unset, Scaleway's default load balancer type is used.

### `SCW_CCM_PREFIX`
A prefix prepended to the name of load balancers created by the CCM. Useful to distinguish
load balancers managed by different clusters or environments.

### `SCW_CCM_TAGS`
A list of additional tags to apply to every load balancer created by the CCM. Tags are split
using the delimiter configured by `SCW_CCM_TAGS_DELIMITER`.

### `SCW_CCM_TAGS_DELIMITER`
The delimiter used to split `SCW_CCM_TAGS` into individual tags. Defaults to `,`.

### `EXTRA_USER_AGENT`
A string appended to the user agent sent to the Scaleway API
(`scaleway/ccm <version> (<git-commit>) <EXTRA_USER_AGENT>`). Useful for tracking requests made
by a specific deployment.

### `DISABLE_INTERFACES`
A comma-separated list of cloud-provider interfaces to disable. Possible values are:
- `instances` - disables the Instances/InstancesV2 interface (node lifecycle, addresses, types).
- `loadbalancer` - disables the LoadBalancer interface (no LB creation/management).
- `zones` - disables the Zones interface (no failure-domain labeling).

### `DISABLE_TAGS_SYNC`
If set to any non-empty value, disables synchronization of Scaleway Instance/server tags to
Kubernetes node labels and taints. See [tag synchronization](tags.md) for details.
