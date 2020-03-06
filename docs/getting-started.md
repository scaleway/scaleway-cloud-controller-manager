# Getting started

## Make the cloud controller image

The cloud-controller-manager has to be deployed in a `kubernetes` cluster. To do so you need an image which can be deployed as a `kubernetes` object.

Using the `Makefile` compile the `cloud-controller-manager` and make an image out of it

```bash
make docker-build
```

It will :

- Fetch the `go` dependencies
- Build the `cloud-controller-manager`
- Make a `docker` image out of it

(Optional) You can push own fresh image of the controller to a registry :

```
# docker tag scaleway/scaleway-cloud-controller-manager:d1e51ce3 rg.fr-par.scw.cloud/test/scaleway-cloud-controller-manager:dev
docker push rg.fr-par.scw.cloud/test/scaleway-cloud-controller-manager:dev
The push refers to repository [rg.fr-par.scw.cloud/test/scaleway-cloud-controller-manager]
c6ff308811cb: Pushed 
cd099fd7777d: Pushed 
8dfad2055603: Pushed 
dev: digest: sha256:13ab072a17eec9694b40bb978f4ef75cb964ae65938a8b1c8bd7d6a8f2c1bd58 size: 950
```

## Create a Kubernetes cluster on Scaleway using kubeadm

Create a `Kubernetes` cluster using `kubeadm`.

For the purpose on this example, you will need to create 3 ubuntu bionic instances.

- master1
- node1
- node2

Run the following commands on each instances :

```bash
apt-get update && apt-get install -y \
    iptables \
    arptables \
    ebtables \
    apt-transport-https \
    ca-certificates \
    curl \
    gnupg-agent \
    software-properties-common
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
add-apt-repository \
   "deb [arch=amd64] https://apt.kubernetes.io kubernetes-xenial main"
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
add-apt-repository \
   "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
   $(lsb_release -cs) \
   stable"
apt-get update && apt-get install -y \
    docker-ce docker-ce-cli containerd.io kubelet kubeadm kubectl
apt-mark hold \
    docker-ce docker-ce-cli containerd.io kubelet kubeadm kubectl
echo KUBELET_EXTRA_ARGS=\"--cloud-provider=external\" > /etc/default/kubelet
```

Intialize the master:

```bash
kubeadm init --control-plane-endpoint=$(scw-metadata PUBLIC_IP_ADDRESS) --apiserver-cert-extra-sans=$(scw-metadata PUBLIC_IP_ADDRESS)
mkdir -p ~/.kube
cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
chown $(id -u):$(id -g) $HOME/.kube/config
kubectl apply -f https://docs.projectcalico.org/v3.11/manifests/calico.yaml
```

Write the kubeadm join command, you will need it to join worker nodes. Also, you can also copy the kubeconfig file to your computer.

Execute the join command on your nodes, repeat this operation on each worker nodes:

```bash
kubeadm join 10.68.34.145:6443 --token itvo0b.kwoao79ptlj22gno \
    --discovery-token-ca-cert-hash sha256:07bc3f9601f1659771a7a6fd696c2969cbc757b088ec752ba95d5a42c06ed91f 
```

And then, execute on the master (or your computer if you copied the kubeconfig):

```bash
master1# kubectl get nodes
NAME      STATUS     ROLES    AGE     VERSION
master1   NotReady   master   5m43s   v1.17.2
node1     NotReady   <none>   2m17s   v1.17.2
node2     NotReady   <none>   77s     v1.17.2
```

The cluster is ready, you now have a working cluster, you can now deploy the `cloud-controller-manager`

## Deploy the cloud-controller-manager on this cluster

To deploy the `cloud-controller-manager` you will need :
* Your access key.
* Your secret key.
* Your organization id.
* The Scaleway region.

Create a `k8s-scaleway-secret.yml` file containing these informations.

_Hint: You can find those information on [https://console.scaleway.com/account/credentials](https://console.scaleway.com/account/credential)_

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: scaleway-secret
  namespace: kube-system
stringData:
  SCW_ACCESS_KEY: "xxxxxxxxxxxxxxxx"
  SCW_SECRET_KEY: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
  SCW_DEFAULT_REGION: "fr-par"
  SCW_DEFAULT_ORGANIZATION_ID: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxx"
```

Create the `secret` and deploy the controller

```bash
kubectl create -f k8s-scaleway-secret.yml
kubectl apply -f https://raw.githubusercontent.com/scaleway/scaleway-cloud-controller-manager/master/examples/k8s-scaleway-ccm-latest.yml
```

## Check the cloud-controller-manager is working

Check the `cloud-controller-manager` is running.

```bash
root@master1:~# kubectl get pods -n kube-system -l app=scaleway-cloud-controller-manager
NAME                                                 READY   STATUS    RESTARTS   AGE
scaleway-cloud-controller-manager-774f5487d5-7z5dd   1/1     Running   7          3m
root@master1:~# kubectl get nodes
NAME      STATUS     ROLES    AGE     VERSION
master1   NotReady   master   5m43s   v1.17.2
node1     NotReady   <none>   2m17s   v1.17.2
node2     NotReady   <none>   77s     v1.17.2
```

Deploy a `LoadBalancer` service and verify a public ip is assigned to this service. If it is the case you're all good, you have deployed a cluster with `kubeadm` and the `scaleway-cloud-controlle-manager

Create a lb.yaml that contain the following object:

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

```
# kubectl create -f lb.yml
# kubectl get services
root@master1:~# kubectl get services
NAME              TYPE           CLUSTER-IP      EXTERNAL-IP    PORT(S)          AGE
example-service   LoadBalancer   10.110.17.115   51.159.26.63   8765:32370/TCP   6d23h
kubernetes        ClusterIP      10.96.0.1       <none>         443/TCP          7d
```
