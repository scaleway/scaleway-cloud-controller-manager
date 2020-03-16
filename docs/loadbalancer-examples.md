# Scaleway LoadBalancer Service example

This example will show you how to use the `cloud-controller-manager` to create a service of type: LoadBalancer.

## Requirements

First, you need a cluster running a `cloud-controller-manager`, this could be a Scaleway Kapsule or your own installed kubernetes.

To create a load balancer you first have to have an running application. In the example below we are going to create a webserver. This web server will then be reached from the outside by creating a LoadBalancer.

```yaml
# examples/demo-nginx-deployment.yaml
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  replicas: 2
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
demo-nginx      10.96.97.137   51.15.224.149    80:30132/TCP   3m
```

You can now access your service via the provisioned load balancer

```bash
curl -i http://51.15.224.149
```

## Deploy a loadbalancer with specific IP

Instead of creating a loadbalancer service with an ephemeral IP (which changes each time your delete/create the service), you can use a reserved loadbalancer address.

_Warning: LoadBalancer ips are different from Instance ips. You cannot use an instance ip on a load-balancer and vice versa._

To reserve a loadbalancer IP through API :
```bash
curl -X POST "https://api.scaleway.com/lb/v1/regions/$SCW_DEFAULT_REGION/ips" -H "X-Auth-Token: $SCW_SECRET_KEY" -H "Content-Type: application/json" \
-d "{\"organization_id\":\"$SCW_DEFAULT_ORGANIZATION_ID\"} | jq -r .ip_address"
```

Now specify this IP address to spec.loadBalancerIP field of the service

```yaml
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
  loadBalancerIP: SCW.LB.IP.ADDR
```

## Convert an ephemeral IP into reserved IP

It's possible to keep an ephemeral address (converting into a reserved address) by patching the associated service.

```bash
export CURRENT_EXTERNAL_IP=$(kubectl get svc $SERVICE_NAME -o json | jq -r .status.loadBalancer.ingress[0].ip)
kubectl patch svc $SERVICE_NAME --type merge --patch "{\"spec\":{\"loadBalancerIP\": \"$CURRENT_EXTERNAL_IP\"}}"
```

This way, the ephemeral ip would not be deleted when the service will be deleted and could be reused in another service.
