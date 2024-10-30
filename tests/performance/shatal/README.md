# Shatal - the tool for virtual machine wobbling.

## Available operations

- Node draining with migration of virtual machines
- Update core fraction from 10% to 25% and vice versa
- Creation of virtual machines
- Deletion of virtual machines
## How to run

```shell
KUBECONFIG=$(cat ~/.kube/config | base64 -w 0)
KUBECONFIG_BASE64=$KUBECONFIG task run
```

## How to configure

The binary expects a `config.yaml` file to configure its operation. The description of the expected configuration is given below.

```yaml
# Base64 of kubeconfig to interact with k8s API.
# Corresponds to the KUBECONFIG_BASE64 environment variable.
kubeconfigBase64: "XXX="
# The resource prefix for virtual machine wobbling.
# Corresponds to the RESOURCES_PREFIX environment variable.
resourcesPrefix: "performance"
# The namespace of virtual machines to wobble.
# Corresponds to the NAMESPACE environment variable.
namespace: "default"
# The interval of wobbling iterations.
# Corresponds to the INTERVAL environment variable.
interval: "5s"
# The count of virtual machines to wobble.
# Corresponds to the COUNT environment variable.
count: 100
# The flag to show debug level logs. 
# Corresponds to the DEBUG environment variable.
debug: true
# Flag to enable forced interrupt mode
# If `true` - shatal stops immediately, all affected resources remain in the state at the moment of shatal interruption.
# If `false` - after shatal interruption all affected resources return to their initial state.
forceInterruption: false
drainer:
  # The flag to enable node draining.
  # Corresponds to the DRAINER_ENABLED environment variable.
  enabled: true
  # Flag to drain the node only once.
  # Corresponds to the DRAINER_ONCE environment variable.
  once: true
  # The selector to specify nodes to drain
  # Corresponds to the DRAINER_LABEL_SELECTOR environment variable.
  labelSelector: true
  # The interval between draining operations.
  # Corresponds to the DRAINER_INTERVAL environment variable.
  interval: "10s"
creator:
  # The flag is enabled to initiate the generation of missing virtual machines until the target quantity is reached (where the target quantity equals the 'count' value).
  # Corresponds to the CREATOR_ENABLED environment variable.
  enabled: true
  # The interval of creation iterations.
  # Corresponds to the CREATOR_INTERVAL environment variable.
  interval: "5s"
deleter:
  # The flag to enable deletion of virtual machines.
  # Corresponds to the DELETER_ENABLED environment variable.
  enabled: true
  # The weight of this operation in comparison to others (determines the probability of triggering the deletion scenario).
  # Corresponds to the DELETER_WIGHT environment variable.
  weight: 1
modifier:
  # The flag to enable update of virtual machines (core fraction from 10% to 25% and vice versa).
  # Corresponds to the MODIFIER_ENABLED environment variable.
  enabled: true
  # The weight of this operation in comparison to others (determines the probability of triggering the deletion scenario).
  # Corresponds to the MODIFIER_WIGHT environment variable.
  weight: 1
nothing:
  # Just a flag to do nothing with virtual machines compared other operations.
  # Maybe you want 10% of virtual machines to be updated, 10% deleted, and 80% to continue working as usual.
  # Corresponds to the NOTHING_ENABLED environment variable.
  enabled: true
  # The weight of the operation determines the probability of taking no action with the virtual machine.
  # Corresponds to the NOTHING_WIGHT environment variable.
  weight: 8
```
