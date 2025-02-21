# Number of CPU Cores

## Problem
In the current virtual machine (VM) configuration, the user can only specify the number of cores. This results in a VM with the number of sockets equal to the number of cores, where each socket contains one core. This causes issues for some operating systems, such as Windows, which support a limited number of sockets.

## Goal
Automatically calculate the number of sockets based on the number of cores to avoid OS limitations and improve VM configuration flexibility.

## Logic for Calculating the Number of Sockets and Cores
For .spec.cpu.cores <= 16:
- One socket is created with the number of cores equal to the specified value.
- Core increment step: 1
- Allowed values: any number from 1 to 16 inclusive.

For 16 < .spec.cpu.cores <= 32:
- Two sockets are created with the same number of cores in each.
- Core increment step: 2
- Allowed values: 18, 20, 22, ..., 32.
- Minimum cores per socket: 9
- Maximum cores per socket: 16

For 32 < .spec.cpu.cores <= 64:
- Four sockets are created with the same number of cores in each.
- Core increment step: 4
- Allowed values: 36, 40, 44, ..., 64.
- Minimum cores per socket: 9
- Maximum cores per socket: 16

For .spec.cpu.cores > 64:
- Eight sockets are created with the same number of cores in each.
- Core increment step: 8
- Allowed values: 72, 80, ...
- Minimum cores per socket: 8

## Value Validation

Validation of .spec.cpu.cores values is performed via a webhook.

## Displaying VM Topology

The current VM topology (actual number of sockets and cores) is displayed in the VM status in the following format:

```yaml
status:
  resources:
    cpu:
      coreFraction: 100%
      cores: 18
      requestedCores: "18"
      runtimeOverhead: "0"
      topology:
        sockets: 2
        coresPerSocket: 9
```
