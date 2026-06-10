# Kubernetes Cloud Controller Manager for Scaleway

`scaleway-cloud-controller-manager` is the Kubernetes cloud controller manager implementation
(or out-of-tree cloud-provider) for Scaleway.

External cloud providers were introduced as an _Alpha_ feature in Kubernetes 1.6.
They are Kubernetes (master) controllers that implement the
cloud-provider specific control loops required for Kubernetes to function.
Read more about kubernetes cloud controller managers in the
[kubernetes documentation](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/).

This is the cloud controller manager that is deployed by default on
[Scaleway Kapsule](https://www.scaleway.com/en/kubernetes-kapsule) (Scaleway Managed Kubernetes)
clusters, but it can also be deployed on self-managed Kubernetes clusters running on Scaleway
Instances or Elastic Metal servers.

## Features

`scaleway-cloud-controller-manager` implements:

 - Instances/InstancesV2 interface - updates nodes with cloud provider specific labels and
   addresses, also deletes kubernetes nodes when deleted from the
   cloud-provider. Supports both Scaleway Instances and Elastic Metal (baremetal) servers.
 - LoadBalancer interface - responsible for creating load balancers when a service
   of `type: LoadBalancer` is created in Kubernetes. See the available
   [annotations](docs/loadbalancer-annotations.md) and [examples](docs/loadbalancer-examples.md).
 - Zone interface - makes Kubernetes aware of the failure domain of each node.

It also synchronizes [Scaleway tags to Kubernetes labels and taints](docs/tags.md) on nodes.

## Branches and releases

There are two types of branches:
- `master` which is the main branch
- `release-x.y` which are the release branches. Each branch is built against Kubernetes 1.y

Most of the PRs on `master` will be backported to the supported `release-x.y` branches.

Each release will be cut from the `release-x.y` branch, and will be in the form of `x.y.z` where `z` will be the Scaleway Cloud Controller patch version.

## Getting Started

Learn more about running `scaleway-cloud-controller-manager` [here](docs/getting-started.md)!

**WARNING**: This cloud controller manager is installed by default on [Scaleway Kapsule](https://www.scaleway.com/en/kubernetes-kapsule) (Scaleway Managed Kubernetes), you don't have to do it yourself or it might cause conflicts.

### Configuration

The CCM is configured entirely through environment variables (Scaleway credentials, region/zone,
and behavior tuning). See the [configuration reference](docs/configuration.md) for the full list.

### Documentation

- [Getting started](docs/getting-started.md)
- [Configuration (environment variables)](docs/configuration.md)
- [LoadBalancer annotations](docs/loadbalancer-annotations.md)
- [LoadBalancer examples](docs/loadbalancer-examples.md)
- [Tag, label and taint synchronization](docs/tags.md)
- [Development setup](docs/development-setup.md)

## Build

You can build the CCM executable using the following commands:

```bash
make compile
```

You can build a local docker image for your current architecture using the following command.
Set `REGISTRY` to your own registry/namespace to avoid building into the `scaleway` namespace:

```bash
REGISTRY=[registry] make docker-build
```

## Test

The unit tests do not require a Kubernetes cluster or Scaleway credentials:

```bash
make test
```

## Development

If you are looking for a way to contribute please read [CONTRIBUTING](CONTRIBUTING.md).

### Code of conduct

Participation in the Kubernetes community is governed by the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).

## Reach us

We love feedback. Feel free to reach us on [Scaleway Slack community](https://slack.scaleway.com), we are waiting for you on #k8s.

You can also join the official Kubernetes slack on #scaleway-k8s channel

You can also [raise an issue](https://github.com/scaleway/scaleway-cloud-controller-manager/issues/new) if you think you've found a bug.
