# Integration tests

## Prerequisites.

### Utilities

Some utilities should be installed to run e2e tests:

- task (https://taskfile.dev)
- kubectl (https://kubernetes.io/docs/tasks/tools/#kubectl)
- d8 (https://github.com/deckhouse/deckhouse-cli/releases)
- ginkgo
  - Download from https://github.com/onsi/ginkgo
  - Or just run `go install github.com/onsi/ginkgo/v2/ginkgo@$(go list -f '{{.Version}}' -m github.com/onsi/ginkgo/v2)`

### Deckhouse cluster

Integration tests require a running Deckhouse cluster with the virtualization module installed.

### Default StorageClass

Default storage class should be set in the cluster. Annotate a StorageClass with
storageclass.kubernetes.io/is-default-class to mark it as the default:

```bash

$ kubectl annotate storageclass linstor-thin-r1 storageclass.kubernetes.io/is-default-class=true

$ kubectl get storageclass linstor-thin-r1 -o yaml | less
...
metadata:
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
...
```

### E2E configuration

Temp directories, prefixes, images and ssh settings can be set in the
YAML configuration file.
Path to custom file can be set with the `E2E_CONFIG` environment variable.
Default config file is `default_config.yaml` in `tests/e2e` directory.

You can override config field with environment variables. Use E2E_ prefix and join uppercased fields with _ (underscore).

For example, to override curl image, set `E2E_HELPERIMAGES_CURLIMAGE` environment variable.

### Cluster connection settings

Connection settings priority from highest to lowest:

- Token and endpoint in E2E_CLUSTERTRANSPORT_TOKEN and E2E_CLUSTERTRANSPORT_ENDPOINT envs.
- Token and endpoint in clusterTransport field in e2e config file.
- A path to kubeconfig file in clusterTransport.kubeConfig field in e2e config file.
- A path to kubeconfig file in KUBECONFIG env.
- A path to kubeconfig file in `~/.kube/config`.


## Run tests from developer machine

Setup cluster connection in "$HOME/.kube/config" or by [switch](https://github.com/danielfoehrKn/kubeswitch)ing the `KUBECONFIG` env and run tests:

```bash
task run
```

### Debugging options

- Use FOCUS env to run one test.
- Use STOP_ON_FAILURE=yes env to stop tests on first failure without cleanup.

For example, run only "Complex text" without cleanup on failure:
```bash
FOCUS="Complex test" STOP_ON_FAILURE=yes task run
```

## Run tests in CI
```bash
task run:ci
```

### Example
Create namespace for service account
```bash
kubectl create ns e2e-tests
```
Create service account
```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: e2e-tests
  namespace: e2e-tests
EOF
```
Create secret with token for service account
```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: e2e-tests
  namespace: e2e-tests
  annotations:
    kubernetes.io/service-account.name: e2e-tests
type: kubernetes.io/service-account-token
EOF
```
Create ClusterRoleBinding 
```bash
kubectl create clusterrolebinding e2e-tests --clusterrole=cluster-admin --serviceaccount=e2e-tests:e2e-tests
```
Export envs and run
```bash
kubectl config view -o jsonpath='{"Cluster name\tServer\n"}{range .clusters[*]}{.name}{"\t"}{.cluster.server}{"\n"}{end}'
export CLUSTER_NAME="some_server_name"
export E2E_CLUSTERTRANSPORT_ENDPOINT=$(kubectl config view -o jsonpath="{.clusters[?(@.name==\"$CLUSTER_NAME\")].cluster.server}")
export E2E_CLUSTERTRANSPORT_TOKEN=$(kubectl get secret e2e-tests -n e2e-tests -ojsonpath='{.data.token}' | base64 -d)
kubectl get secret e2e-tests -n e2e-tests -ojsonpath='{.data.ca\.crt}' | base64 -d > ca.crt
export E2E_CLUSTERTRANSPORT_CERTIFICATEAUTHORITY="$PWD/ca.crt"
export E2E_CLUSTERTRANSPORT_INSECURETLS="false"

task run
```
