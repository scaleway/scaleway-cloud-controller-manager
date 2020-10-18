# Tag management

The Scaleway CCM will also sync the tags of the Scaleway Instance to Kubernetes Labels on the nodes.

**When using Scaleway's managed Kubernetes, Kapsule, the tags on the pool will be transformed into tags on the pool's instances, so applying to all Kubernetes nodes of the pool.**

## Labels

In order for a tag to be synced to a label, it needs to be of the form `foo=bar`.
In this case, the Kubernetes nodes will have the label `k8s.scaleway.com/foo=bar`.

Once the tag is removed from the instance, it will also be removed as a label on the node.

## Taints

In order for a tag to be synced to a taint, it needs to be of the form `taint=foo=bar:Effect`, where `Effect` is one of `NoExecute`, `NoSchedule` or `PreferNoSchedule`.
In this case, the Kubernetes nodes will have the taint `k8s.scaleway.com/foo=bar` with the effect `Effect`.

Once the tag is removed from the instance, it will also be removed as a taint on the node.

## Special Kubernetes Labels

- `node.kubernetes.io/exclude-from-external-load-balancers` can be set on the Kubernetes nodes if this same value is set as a tag on the instance. It will have the value `managed-by-scaleway-ccm` and will be deleted if deleted from the tags.
