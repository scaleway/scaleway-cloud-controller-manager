# Scaleway LoadBalancer Service example

This example will show you how to use the `cloud-controller-manager` to create a service of type: LoadBalancer.

## Requirements

First, you need a cluster running a `cloud-controller-manager`, this could be a Scaleway Kapsule or your own installed kubernetes.

To create a load balancer you first have to have an running application. In the example below we are going to create a webserver. This web server will then be reached from the outside by creating a LoadBalancer.

```yaml
# examples/demo-nginx-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  replicas: 2
  selector:
    matchLabels:
      app: demo-nginx
  template:
    metadata:
      labels:
        app: demo-nginx
    spec:
      containers:
      - name: nginx
        image: nginx
        ports:
        - containerPort: 80
```

Execute it

```bash
kubectl create -f examples/demo-nginx-deployment.yaml
```

## Deploy a TCP Loadbalancer

The example below will expose the deployment and via a loadbalancer.
Note that the service **type** is set to **LoadBalancer**.

```yaml
# examples/demo-nginx-svc.yaml
kind: Service
apiVersion: v1
metadata:
  name: demo-nginx
spec:
  selector:
    app: demo-nginx
  type: LoadBalancer
  ports:
  - name: http
    port: 80
    targetPort: 80
```

Execute it

```bash
kubectl create -f examples/demo-nginx-svc.yaml
```

Deploying a loadbalancer takes few seconds, wait until `EXTERNAL-IP` address appear. This will be the load balancer
IP which you can use to connect to your service.

_Note: you can append `--watch` to the command to automaticaly refresh the result._

```bash
$ kubectl get svc
NAME            CLUSTER-IP     EXTERNAL-IP      PORT(S)        AGE
demo-nginx      10.96.97.137   203.0.113.10     80:30132/TCP   3m
```

You can now access your service via the provisioned load balancer

```bash
curl -i http://203.0.113.10
```

## Deploy a loadbalancer with specific IP

Instead of creating a loadbalancer service with an ephemeral IP (which changes each time your delete/create the service), you can use a reserved loadbalancer address.

_Warning: LoadBalancer ips are different from Instance ips. You cannot use an instance ip on a load-balancer and vice versa._

To reserve a loadbalancer IP using the [Scaleway CLI](https://github.com/scaleway/scaleway-cli):
```bash
scw lb ip create zone=$SCW_DEFAULT_ZONE project-id=$SCW_DEFAULT_PROJECT_ID -o json | jq -r .id
```

Now specify the ID of this IP address in the `service.beta.kubernetes.io/scw-loadbalancer-ip-ids` annotation.

```yaml
kind: Service
apiVersion: v1
metadata:
  name: demo-nginx
  annotations:
    service.beta.kubernetes.io/scw-loadbalancer-ip-ids: IP_ID
spec:
  selector:
    app: demo-nginx
  type: LoadBalancer
  ports:
  - name: http
    port: 80
    targetPort: 80
```

## Convert an ephemeral IP into reserved IP

It's possible to keep an ephemeral address (converting into a reserved address) by patching the associated service.

```bash
export CURRENT_EXTERNAL_IP=$(kubectl get svc $SERVICE_NAME -o json | jq -r .status.loadBalancer.ingress[0].ip)
export CURRENT_EXTERNAL_IP_ID=$(scw lb ip list zone=$SCW_DEFAULT_ZONE ip-address=$CURRENT_EXTERNAL_IP -o json | jq -r '.[0].id')
kubectl patch svc $SERVICE_NAME --type merge --patch "{\"metadata\":{\"annotations\": {\"service.beta.kubernetes.io/scw-loadbalancer-ip-ids\": \"$CURRENT_EXTERNAL_IP_ID\"}}}"
```

This way, the ephemeral ip would not be deleted when the service will be deleted and could be reused in another service.
