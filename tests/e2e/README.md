# Integration tests

## Running integration tests

Integration tests require a running deckhouse cluster with the virtualization module installed.  
Once you have a running deckhouse cluster, you can use config, environment variables or flags  to
point the tests to the cluster. 
You can override the config file using the env var ```E2E_CONFIG```.
(default config - ```default_config.yaml```)

You must also have a default class declared
Mark a ReplicatedStorageClass as default:
```yaml
spec:
  isDefault: true
```
Example:
```bash
kubectl patch replicatedstorageclasses.storage.deckhouse.io linstor-thin-r3 --type=json  -p='[{"op": "replace", "path":"/spec/isDefault", "value":true}]'
```
### Configuration
To override a configuration option, create an environment variable named ```E2E_variable``` where variable is the name of the configuration option and the _ (underscore) represents indention levels. 
For example, you can configure the ```token``` of the ```kubectl``` and ```virtctl```:
```bash
clusterTransport:
  token: "your token"
```
To override this value, set an environment variable like this:
```bash
export E2E_CLUSTERTRANSPORT_TOKEN="your token"
```

### RUN

```bash
task run
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