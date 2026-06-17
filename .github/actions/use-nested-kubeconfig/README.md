# use-nested-kubeconfig

Decodes the nested-cluster kubeconfig used by E2E workflows into `~/.kube/config`, fixes permissions, selects the requested context, and can wait for `kubectl get nodes` to succeed.
