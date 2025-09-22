/*
Copyright 2018 The KubeVirt Authors
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/portforward/portforward.go
*/

package portforward

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"

	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	subv1alpha3 "github.com/deckhouse/virtualization/api/subresources/v1alpha3"
	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

const (
	forwardToStdioFlag = "stdio"
	addressFlag        = "address"
)

var (
	forwardToStdio bool
	address        = "127.0.0.1"
)

func NewCommand() *cobra.Command {
	portforward := &PortForward{}
	cmd := &cobra.Command{
		Use:     "port-forward name[.namespace] [protocol/]localPort[:targetPort]...",
		Short:   "Forward local ports to a virtual machine",
		Long:    usage(),
		Example: examples(),
		Args: func(cmd *cobra.Command, args []string) error {
			if n := len(args); n < 2 {
				klog.Errorf("fatal: Number of input parameters is incorrect, portforward requires at least 2 arg(s), received %d", n)
				// always write to stderr on failures to ensure they get printed in stdio mode
				cmd.SetOut(os.Stderr)
				err := cmd.Help()
				if err != nil {
					return err
				}
				return errors.New("argument validation failed")
			}
			return nil
		},
		RunE: portforward.Run,
	}
	cmd.Flags().BoolVar(&forwardToStdio, forwardToStdioFlag, forwardToStdio,
		fmt.Sprintf("--%s=true: Set this to true to forward the tunnel to stdout/stdin; Only works with a single port", forwardToStdioFlag))
	cmd.Flags().StringVar(&address, addressFlag, address,
		fmt.Sprintf("--%s=: Set this to the address the local ports should be opened on", addressFlag))
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

type PortForward struct {
	address  *net.IPAddr
	resource portforwardableResource
}

func (o *PortForward) Run(cmd *cobra.Command, args []string) error {
	setOutput(cmd)

	client, defaultNamespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}

	namespace, name, ports, err := o.prepareCommand(defaultNamespace, args)
	if err != nil {
		return err
	}

	o.resource = client.VirtualMachines(namespace)

	if forwardToStdio {
		if len(ports) != 1 {
			return errors.New("only one port supported when forwarding to stdout")
		}
		return o.startStdoutStream(namespace, name, ports[0])
	}

	o.address, err = net.ResolveIPAddr("", address)
	if err != nil {
		return err
	}

	if err := o.startPortForwards(namespace, name, ports); err != nil {
		return err
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	return nil
}

func (o *PortForward) prepareCommand(defaultNamespace string, args []string) (namespace, name string, ports []forwardedPort, err error) {
	namespace, name, err = templates.ParseTarget(args[0])
	if err != nil {
		return namespace, name, ports, err
	}

	ports, err = parsePorts(args[1:])
	if err != nil {
		return namespace, name, ports, err
	}

	if namespace == "" {
		namespace = defaultNamespace
	}

	return namespace, name, ports, err
}

func (o *PortForward) startStdoutStream(namespace, name string, port forwardedPort) error {
	streamer, err := o.resource.PortForward(name, subv1alpha3.VirtualMachinePortForward{Port: port.remote, Protocol: port.protocol})
	if err != nil {
		return err
	}

	klog.V(3).Infof("forwarding to %s/%s:%d", namespace, name, port.remote)
	if err := streamer.Stream(virtualizationv1alpha2.StreamOptions{
		In:  os.Stdin,
		Out: os.Stdout,
	}); err != nil {
		return err
	}

	return nil
}

func (o *PortForward) startPortForwards(namespace, name string, ports []forwardedPort) error {
	for _, port := range ports {
		forwarder := portForwarder{
			namespace: namespace,
			name:      name,
			resource:  o.resource,
		}
		if err := forwarder.startForwarding(o.address, port); err != nil {
			return err
		}
	}
	return nil
}

// setOutput to stderr if we're using stdout for traffic
func setOutput(cmd *cobra.Command) {
	if forwardToStdio {
		cmd.SetOut(os.Stderr)
		cmd.Root().SetOut(os.Stderr)
	} else {
		cmd.SetOut(os.Stdout)
	}
}

func usage() string {
	return `Forward local ports to a virtualmachine.
The port argument supports the syntax protocol/localPort:targetPort with protocol/ and :targetPort as optional fields.
Protocol supports TCP (default) and UDP.

Portforwards get established over the Kubernetes control-plane using websocket streams.
Usage can be restricted by the cluster administrator through the /portforward subresource.
`
}

func examples() string {
	return `  #Forward the local port 8080 to the vm port
  {{ProgramName}} port-forward myvm 8080

  # Forward the local port 8080 to the vm port in mynamespace
  {{ProgramName}} port-forward myvm.mynamespace 8080
  {{ProgramName}} port-forward myvm 8080 -n mynamespace

  # Note: {{ProgramName}} port-forward sends all traffic over the Kubernetes API Server.
  # This means any traffic will add additional pressure to the control plane.
  # For continuous traffic intensive connections, consider using a dedicated Kubernetes Service.

  # Open an SSH connection using PortForward and ProxyCommand:
  ssh -o 'ProxyCommand={{ProgramName}} port-forward --stdio=true myvm.mynamespace 22' user@myvm.mynamespace

  # Use as SCP ProxyCommand:
  scp -o 'ProxyCommand={{ProgramName}} port-forward --stdio=true myvm.mynamespace 22' local.file user@myvm.mynamespace`
}
