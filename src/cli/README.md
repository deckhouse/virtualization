# Virtualization

Subcommand for the command line client for Deckhouse.
Manages virtual machine-related operations in your Kubernetes cluster.

### Available Commands

| Command            | Description                                                            |
|--------------------|------------------------------------------------------------------------|
| ansible-inventory  | Generate ansible inventory from virtual machines                       |
| collect-debug-info | Collect debug information for VM: configuration, events, and logs      |
| console            | Connect to a console of a virtual machine.                             |
| port-forward       | Forward local ports to a virtual machine.                              |
| scp                | SCP files from/to a virtual machine.                                   |
| ssh                | Open an SSH connection to a virtual machine.                           |
| vnc                | Open a VNC connection to a virtual machine.                            |
| start              | Start a virtual machine.                                               |
| stop               | Stop a virtual machine.                                                |
| restart            | Restart a virtual machine.                                             |
| evict              | Evict a virtual machine.                                               |

### Examples

#### ansible-inventory

```shell
# Get inventory for default namespace in JSON format
d8 v ansible-inventory
d8 v ansible-inventory --list

# Get host variables
d8 v ansible-inventory --host myvm.mynamespace

# Specify namespace
d8 v ansible-inventory -n mynamespace

# Specify output format (json, ini, yaml)
d8 v ansible-inventory -o json
d8 v ansible-inventory -o yaml
d8 v ansible-inventory -o ini
```

#### console

```shell
d8 v console myvm
d8 v console myvm.mynamespace
```

#### port-forward

```shell
d8 v port-forward myvm tcp/8080:8080
d8 v port-forward --stdio=true myvm.mynamespace 22
```

#### scp

```shell
d8 v scp myfile.bin user@myvm:myfile.bin
d8 v scp user@myvm:myfile.bin ~/myfile.bin
```

#### ssh

```shell
d8 v --identity-file=/path/to/ssh_key ssh user@myvm.mynamespace
d8 v ssh --local-ssh=true --namespace=mynamespace --username=user myvm
```

#### vnc

```shell
d8 v vnc myvm.mynamespace
d8 v vnc myvm -n mynamespace
```

#### start

```shell
d8 v start myvm.mynamespace --wait
d8 v start myvm -n mynamespace
```

#### stop

```shell
d8 v stop myvm.mynamespace --force
d8 v stop myvm -n mynamespace
```

#### restart

```shell
d8 v restart myvm.mynamespace --timeout=1m
d8 v restart myvm -n mynamespace
```

#### evict

```shell
d8 v evict myvm.mynamespace
d8 v evict myvm -n mynamespace
```

#### collect-debug-info

```shell
# Collect debug info for VirtualMachine 'myvm' (output compressed archive to stdout)
d8 v collect-debug-info myvm > debug-info.tar.gz
d8 v collect-debug-info myvm.mynamespace > debug-info.tar.gz
d8 v collect-debug-info myvm -n mynamespace > debug-info.tar.gz
```
