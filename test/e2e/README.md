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
  defaultClusterStorageClass: replicated-storage-class
```

Additionally, the storage class in the tests can be defined by the environment variable `STORAGE_CLASS_NAME`:
```bash
STORAGE_CLASS_NAME=replicated-storage-class task run
```

The override applies to the whole run: every VirtualDisk (and every PVC-backed
VirtualImage) built through `internal/object` is pinned to `STORAGE_CLASS_NAME` unless the
test picks a StorageClass itself. Without the variable no StorageClass is written to the
spec, so the cluster default StorageClass applies.

The StorageClass (`STORAGE_CLASS_NAME` or the cluster default) may use any volume binding
mode. When the cluster has no default StorageClass, set `STORAGE_CLASS_NAME` explicitly.

Note: `task run VAR=value` style passes variables to the Taskfile templates only, not to the
environment of the test process. Environment variables consumed by the tests themselves
(`STORAGE_CLASS_NAME` aside, which the Taskfile forwards, e.g. `*_PRECHECK=no` switches)
must be passed as real environment variables: `DEFAULT_STORAGE_CLASS_PRECHECK=no task run ...`.

### E2E configuration

Temp directories, prefixes, images and ssh settings can be set in the
YAML configuration file.
Path to custom file can be set with the `E2E_CONFIG` environment variable.
Default config file is `default_config.yaml` in `test/e2e` directory.

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

Tests run in parallel by default (`--procs=24`, override with the `PROCS` env variable, timeout 1h).
To run them sequentially (timeout 3h):

```bash
SEQUENTIAL=true task run
```

`task runp` is deprecated and is just an alias for `task run`.

### Debugging options

- Use the FOCUS environment variable to run a specific test.
- Set `POST_CLEANUP=never` to disable cleanup after tests (takes precedence over `postCleanupMode` in config).
- Set LABELS to run tests with specific label(https://onsi.github.io/ginkgo/#spec-labels).
- Manage timeouts for e2e tests using env variables `E2E_SHORT_TIMEOUT`, `E2E_MIDDLE_TIMEOUT`, `E2E_LONG_TIMEOUT` and `E2E_MAX_TIMEOUT`.

For example, to run only one test and leave all created resources in the cluster, use the following command:
```bash
FOCUS="VirtualMachineConnectivity" POST_CLEANUP=never task run
```

### PostCleanUp option

`POST_CLEANUP` defines an environment variable used to explicitly request the deletion of created/used resources.

Valid values:
- `always` or "" (empty) - perform cleanup after tests (default)
- `never` - skip cleanup after tests
- `no-on-failure` - perform cleanup only when the spec passes (skip cleanup on failure, preserving resources for investigation)

You can also control cleanup behavior via the `postCleanupMode` field in `default_config.yaml`:
```yaml
postCleanupMode: always  # default: cleanup enabled
```

The `POST_CLEANUP` environment variable takes precedence over the YAML config.

For example, run a test in no-cleanup mode:
```bash
POST_CLEANUP=never task run
```

### Working with `Virtualization-controller` errors

When the Ginkgo tests suite is running, it also runs the Virtualization-controller log stream. If you see an error in the STDOUT, it is not an error of the Ginkgo contexts. You can ignore this error by adding an ignore-pattern to the configuration file (default_config.yaml). But remember that the Virtualization-controller should work without errors while the tests suite is running. If your changes may be causing errors, check the code.

Example:
```yaml
logFilter:
  - "failed to sync virtual disk data source objectref" # "err": "failed to sync virtual disk data source objectref: admission webhook \"datavolume-validate.cdi.kubevirt.io\" denied the request:  Destination PVC winwin/vd-win2022-8a136ef9-32d9-4ae3-a27f-e42e15c15f47 already exists"
  - "failed to detach: intvirtvm not found to unplug" # "err": "failed to detach: intvirtvm not found to unplug"
```

## Run release smoke tests from developer machine

The release smoke suite (`release/`) checks that VMs survive a module upgrade. In CI it
runs in a nested cluster; `release/run-local.sh` runs it against the cluster of the current
kubeconfig in four phases:

- `pre` - create the VMs, start iperf, save the upgrade context
- `upgrade` - move the VMs the way a module upgrade would (see `UPGRADE_MODE`)
- `post` - check the VMs survived: guest access, disks, no restart, iperf, VMOPs
- `cleanup` - delete the release Project with everything in it

`all` (the default) runs them in order.

The cluster needs the `generic-for-e2e` VirtualMachineClass
(`.github/scripts/bash/e2e/create-e2e-vmclass.sh`) and the multitenancy-manager `default`
ProjectTemplate. The script also requires `kubectl`, `jq` and `go`.

The examples below run all phases from the repository root. Set the StorageClasses for the
replicated, local and NFS disks to the ones your cluster has; the script lists the cluster
StorageClasses if one is missing:

```bash
cd test/e2e
export RELEASE_STORAGE_CLASS_REPLICATED=linstor-thin-r1
export RELEASE_STORAGE_CLASS_LOCAL=sds-local-thin-wffc
export RELEASE_STORAGE_CLASS_NFS=nfs-4-1-wffc
task release:local
```

```fish
cd test/e2e
set -x RELEASE_STORAGE_CLASS_REPLICATED linstor-thin-r1
set -x RELEASE_STORAGE_CLASS_LOCAL sds-local-thin-wffc
set -x RELEASE_STORAGE_CLASS_NFS nfs-4-1-wffc
task release:local
```

To run one phase, pass it after `--`, e.g. `task release:local -- pre`. A run that fails
before `cleanup` blocks the next `pre` until you run `task release:local -- cleanup`.

Options (environment variables):

- `UPGRADE_MODE`:
  - `evict` (default) - an Evict VMOP for every Running VM, the migrations a new
    virt-handler/virt-launcher triggers.
  - `none` - nothing moves; `post` skips the migration checks.
  - `manual` - pause until Enter, do the disruption yourself (restart virt-handler, roll a
    dev build, ...); set `RELEASE_UPGRADE_MIGRATES_VMS=false` if nothing migrated.
  - `mpo` - a real upgrade: patch the ModulePullOverride to `NEW_RELEASE`; needs
    `CURRENT_RELEASE`, `NEW_RELEASE`, `DEV_MODULE_SOURCE` and `crane` logged in to it. The
    cluster stays on `NEW_RELEASE` afterwards.
- `KEEP=yes` - leave the resources after `all`.
- `STATE_DIR` - where the phases hand over state (default: `${TMPDIR:-/tmp}/release-e2e-local`).
- `TIMEOUT_SECONDS` - how long `upgrade` waits for the Evict VMOPs (default: 1200).
- `GINKGO_ARGS` - extra ginkgo flags, e.g. `"--timeout=2h"`.

See the header of `release/run-local.sh` for details.
