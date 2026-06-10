# Getting started

This guide shows how to deploy `scaleway-cloud-controller-manager` (CCM) on a self-managed
Kubernetes cluster running on Scaleway Instances or Elastic Metal servers.

**WARNING**: If you are using [Scaleway Kapsule](https://www.scaleway.com/en/kubernetes-kapsule)
(Scaleway Managed Kubernetes), the CCM is already deployed for you. Deploying it yourself on a
Kapsule cluster is not needed and may cause conflicts.

## Prerequisites

You need a Kubernetes cluster running on Scaleway resources, bootstrapped with the
`--cloud-provider=external` flag so that the kubelet defers node initialization to the CCM.

If you are bootstrapping a new cluster with `kubeadm`, follow the
[official kubeadm installation guide](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/install-kubeadm/)
and make sure every node (control-plane and workers) is started with:

```bash
echo 'KUBELET_EXTRA_ARGS="--cloud-provider=external"' > /etc/default/kubelet
```

Then initialize the control plane and join your worker nodes as usual, e.g.:

```bash
kubeadm init --control-plane-endpoint=$(scw-metadata PUBLIC_IP_ADDRESS) --apiserver-cert-extra-sans=$(scw-metadata PUBLIC_IP_ADDRESS)
mkdir -p $HOME/.kube
cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
chown $(id -u):$(id -g) $HOME/.kube/config
```

Install a CNI plugin of your choice (e.g. [Cilium](https://docs.cilium.io/en/stable/gettingstarted/k8s-install-default/)
or [Calico](https://docs.tigera.io/calico/latest/getting-started/kubernetes/)) before continuing,
as nodes will stay `NotReady` until the CNI is installed and the CCM has initialized them.

## Configure your Scaleway credentials

The CCM needs a Scaleway API key (access key + secret key), and the region/zone where it should
operate. See the [configuration reference](configuration.md) for the full list of supported
environment variables.

Create a Secret with your credentials, based on
[`examples/k8s-scaleway-secret.yml`](../examples/k8s-scaleway-secret.yml):

_Hint: You can generate an API key from the [Scaleway console](https://console.scaleway.com/iam/api-keys)._

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: scaleway-secret
  namespace: kube-system
type: Opaque
stringData:
  SCW_ACCESS_KEY: "xxxxxxxxxxxxxxxx"
  SCW_SECRET_KEY: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  SCW_DEFAULT_PROJECT_ID: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxx"
  SCW_DEFAULT_REGION: "fr-par"
  SCW_DEFAULT_ZONE: "fr-par-1"
```

```bash
kubectl apply -f k8s-scaleway-secret.yml
```

## Deploy the cloud-controller-manager

Deploy the CCM using the [example manifest](../examples/k8s-scaleway-ccm-latest.yml), which
includes the Deployment, ServiceAccount and RBAC rules required by the CCM:

```bash
kubectl apply -f https://raw.githubusercontent.com/scaleway/scaleway-cloud-controller-manager/master/examples/k8s-scaleway-ccm-latest.yml
```

## Check the cloud-controller-manager is working

Check that the `cloud-controller-manager` pod is running, and that nodes become `Ready` once it
has initialized them:

```bash
kubectl get pods -n kube-system -l app=scaleway-cloud-controller-manager
kubectl get nodes
```

Once nodes are `Ready`, you can verify the LoadBalancer integration by deploying a `Service` of
`type: LoadBalancer` - see the [LoadBalancer examples](loadbalancer-examples.md) for a full
walkthrough:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: example-service
spec:
  selector:
    app: example
  ports:
    - port: 8765
      targetPort: 9376
  type: LoadBalancer
```

```bash
kubectl apply -f lb.yaml
kubectl get services
```

After a few seconds, an `EXTERNAL-IP` should be assigned to the service - this is the IP of the
Scaleway Load Balancer created by the CCM.
