# Example of scaleway-cloud-controller-manager deployment

The scaleway-cloud-controller-manager is designed to run in a K8S cluster but can also be run outside of a cluster.

Here are the _Kubernetes_ manifest to deploy the cloud-controller-manager on a cluster:
* k8s-scaleway-secret.yaml: example config file for scaleway components (ccm, csi).
* k8s-scaleway-ccm-latest.yaml: deployment file.

You can also execute the controller manager outside of kubernetes with different ways:
* docker-compose-latest.yaml: example deployment file.
