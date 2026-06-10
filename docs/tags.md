# Tag management

The Scaleway CCM will also sync the tags of the Scaleway Instance to Kubernetes Labels on the nodes.

**On Scaleway's managed Kubernetes (Kapsule), this synchronization is disabled in the CCM
(`DISABLE_TAGS_SYNC`, see the [configuration reference](configuration.md#disable_tags_sync)) and
is instead handled directly by Scaleway. Tags set on a pool are transformed into tags on the
pool's instances, and applied to all Kubernetes nodes of the pool the same way as described
below.**

## Labels

In order for a tag to be synced to a label, it needs to be of the form `foo=bar`.
In this case, the Kubernetes nodes will have the label `k8s.scaleway.com/foo=bar`.

Once the tag is removed from the instance, it will also be removed as a label on the node.

### Non prefixed labels

It is possible to add labels not prefixed with `k8s.scaleway.com`. The downside, is that when you will delete the associated tag, the label won't get removed.
In order to have non prefixed labels, you should prefix the tag with `noprefix=`.

For instance the tag `noprefix=foo=bar` will yield the `foo=bar` label on the Kubernetes nodes.

This is the only way to add custom prefixed labels like `node.kubernetes.io`.

## Taints

In order for a tag to be synced to a taint, it needs to be of the form `taint=foo=bar:Effect`, where `Effect` is one of `NoExecute`, `NoSchedule` or `PreferNoSchedule`.
In this case, the Kubernetes nodes will have the taint `k8s.scaleway.com/foo=bar` with the effect `Effect`.

Once the tag is removed from the instance, it will also be removed as a taint on the node.

### Non prefixed Taints

It is possible to add taints not prefixed with `k8s.scaleway.com`. The downside, is that when you will delete the associated tag, the taint won't get removed.
In order to have non prefixed taints, you should prefix the taint with `taint=noprefix=`.

For instance the tag `taint=noprefix=foo=bar:Effect` will yield the `foo=bar` taint on the Kubernetes nodes with the `Effect` effect.

This is the only way to add custom prefixed taints like `node.kubernetes.io`.

## Special Kubernetes Labels

- `node.kubernetes.io/exclude-from-external-load-balancers` can be set on the Kubernetes nodes if this same value is set as a tag on the instance. It will have the value `managed-by-scaleway-ccm` and will be deleted if deleted from the tags.
