# Integration tests

## Running integration tests

Integration tests require a running deckhouse cluster with the virtualization module installed.  
Once you have a running deckhouse cluster, you can use environment variables to
point the tests to the cluster.

### Envs

1) ```KUBECONFIG="/path/to/your/kubeconfig"```
2) ```TOKEN="token from your service account"``` use as ```kubectl --token```
3) ```ENDPOINT="your cluster API endpoint"``` use as ```kubectl --server```
4) ```CA_CRT="/path/to/your/certificate-authority"``` use as ```kubectl --certificate-authority```
5) ```INSECURE_TLS="true/false"``` use as ```kubectl  --insecure-skip-tls-verify```

### RUN

```bash
cd tests/e2e
ginkgo
```
