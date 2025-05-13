# Virtualization
Subcommand for the command line client for Deckhouse.
Manages virtual machine-related operations in your Kubernetes cluster.

### Available Commands:
* console      - Connect to a console of a virtual machine.
* port-forward - Forward local ports to a virtual machine
* scp          - SCP files from/to a virtual machine.
* ssh          - Open an ssh connection to a virtual machine.
* vnc          - Open a vnc connection to a virtual machine.
* start        - Start a virtual machine.
* stop         - Stop a virtual machine.
* restart      - Restart a virtual machine.
* evict        - Evict a virtual machine.

### Examples
#### console
```shell
d8 virtualization console myvm
d8 virtualization console myvm.mynamespace
```
#### port-forward
```shell
d8 virtualization port-forward myvm tcp/8080:8080
d8 virtualization port-forward --stdio=true myvm.mynamespace 22
```
#### scp
```shell
d8 virtualization scp myfile.bin user@myvm:myfile.bin
d8 virtualization scp user@myvm:myfile.bin ~/myfile.bin
```
#### ssh
```shell
d8 virtualization --identity-file=/path/to/ssh_key ssh user@myvm.mynamespace
d8 virtualization ssh --local-ssh=true --namespace=mynamespace --username=user myvm
```
#### vnc
```shell
d8 virtualization vnc myvm.mynamespace
d8 virtualization vnc myvm -n mynamespace
```
#### start
```shell
d8 virtualization start myvm.mynamespace --wait 
d8 virtualization start myvm -n mynamespace
```
#### stop
```shell
d8 virtualization stop myvm.mynamespace --force 
d8 virtualization stop myvm -n mynamespace 
```
#### restart
```shell
d8 virtualization restart myvm.mynamespace --timeout=1m
d8 virtualization restart myvm -n mynamespace
```
#### evict
```shell
d8 virtualization evict myvm.mynamespace
d8 virtualization evict myvm -n mynamespace
```