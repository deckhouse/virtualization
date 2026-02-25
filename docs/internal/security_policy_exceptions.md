# SecurityPolicyExceptions

Deckhouse versions > 1.75 will enable strict security checks for Pods in 
"system" namespaces, so we need to add SecurityPolicyException resources
for all Pods in the d8-virtualization namespace that require more permissions
than the Pod Security Standard "Restricted".

Module has these Pods with additional permissions:

- ds/virtualization-dra
- ds/virt-handler
- ds/vm-route-forge

### virtualization-dra

This component requires access to USB devices on the host.

### virt-handler

It works extensively with KVM subsystem and needs access
to root filesystems of the Pods with VM. 

### vm-route-forge

It manipulates route tables on the host.

## Implementation details.

1. New template is added to detect if SecurityPolicyExtension kind is available. See templates/_d8_security_policy_exception.tpl.
2. New labels are added to the d8-virtualization namespace to enable stricter security checks.
3. New label is added to Pod template. Value for the label is the name of the SecurityPolicyExtension that applies to this Pod.

