# Integration tests

## Prerequisites

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

### Permissions

When adding a new set of integration tests, ensure that the test user has the necessary RBAC permissions to access and manipulate the required resources. The test user should have permissions to:

- Create, read, update and delete test resources
- Access cluster-wide resources if needed
- Execute commands in pods
- Access node resources if required by the tests

Add appropriate ClusterRole/Role and ClusterRoleBinding/RoleBinding resources to grant the required permissions.

You can check the permissions for the corresponding service account using `kubectl auth can-i` commands, for example:

```
kubectl auth can-i --as=virt-e2e-sa ...
```

### Default Storage Class

Default storage class should be set in the cluster. You can set a default storage class in the `global` module config:

```bash
$ kubectl get moduleconfigs.deckhouse.io global --output yaml | yq .spec
```
```yaml
settings:
  defaultClusterStorageClass: linstor-thin-r1
```

Additionally, the storage class in the tests can be defined by the environment variable `STORAGE_CLASS_NAME`:
```bash
STORAGE_CLASS_NAME=linstor-thin-r1 task run
```

### Immediate Storage Class
Some test cases depend on an immediate storage class. You can skip the immediate storage class check if a test case does not require it.

```bash
FOCUS="VirtualMachineVersions" SKIP_IMMEDIATE_SC_CHECK="yes" task e2e:run
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

- Use the FOCUS environment variable to run a specific test.
- Set CONTINUE_ON_FAILURE=yes to continue running tests despite any failures.
- Set POST_CLEANUP=no to disable cleanup after tests.
- Set LABELS to run tests with specific label(https://onsi.github.io/ginkgo/#spec-labels).
     

For example, to run only the "ComplexTest" ignoring failed suites and leave all created resources in the cluster, use the following command: 
```bash
FOCUS="ComplexTest" CONTINUE_ON_FAILURE=yes POST_CLEANUP=no task run
```

### PostCleanUp option

POST_CLEANUP defines an environment variable used to explicitly request the deletion of created/used resources.

For example, run a test in no-cleanup mode:
```bash
POST_CLEANUP=no task run
```

### Working with `Virtualization-controller` errors

When the Ginkgo tests suite is running, it also runs the Virtualization-controller log stream. If you see an error in the STDOUT, it is not an error of the Ginkgo contexts. You can ignore this error by adding an ignore-pattern to the configuration file (default_config.yaml). But remember that the Virtualization-controller should work without errors while the tests suite is running. If your changes may be causing errors, check the code.

Example:
```yaml
logFilter:
  - "failed to sync virtual disk data source objectref" # "err": "failed to sync virtual disk data source objectref: admission webhook \"datavolume-validate.cdi.kubevirt.io\" denied the request:  Destination PVC winwin/vd-win2022-8a136ef9-32d9-4ae3-a27f-e42e15c15f47 already exists"
  - "failed to detach: intvirtvm not found to unplug" # "err": "failed to detach: intvirtvm not found to unplug"
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
