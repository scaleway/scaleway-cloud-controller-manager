# Example of scaleway-cloud-controller-manager deployment

The scaleway-cloud-controller-manager is designed to run in a K8S cluster but can also be run outside of a cluster.

Here are the _Kubernetes_ manifests to deploy the cloud-controller-manager on a cluster:
* k8s-scaleway-secret.yml: example Secret containing the Scaleway credentials and location
  (`SCW_ACCESS_KEY`, `SCW_SECRET_KEY`, `SCW_DEFAULT_PROJECT_ID`, `SCW_DEFAULT_REGION`,
  `SCW_DEFAULT_ZONE`). See the [configuration reference](../docs/configuration.md) for the full
  list of supported environment variables.
* k8s-scaleway-ccm-latest.yml: Deployment, ServiceAccount and RBAC manifest.

You can also execute the controller manager outside of kubernetes with different ways:
* docker-compose-latest.yml: example deployment file.
