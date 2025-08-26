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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/ssh/wrapped.go
*/

package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/klog/v2"
)

func addAdditionalCommandlineArgs(flagset *pflag.FlagSet, opts *SSHOptions) {
	flagset.StringArrayVarP(&opts.AdditionalSSHLocalOptions, additionalOpts, additionalOptsShort, opts.AdditionalSSHLocalOptions,
		fmt.Sprintf(`--%s="-o StrictHostKeyChecking=no" : Additional options to be passed to the local ssh. This is applied only if local-ssh=true`, additionalOpts))
	flagset.BoolVar(&opts.WrapLocalSSH, wrapLocalSSHFlag, opts.WrapLocalSSH,
		fmt.Sprintf("--%s=true: Set this to true to use the SSH/SCP client available on your system by using this command as ProxyCommand; If set to false, this will establish a SSH/SCP connection with limited capabilities provided by this client", wrapLocalSSHFlag))
}

func RunLocalClient(cmd *cobra.Command, namespace, name string, options *SSHOptions, clientArgs []string) error {
	args := []string{"-o"}
	args = append(args, buildProxyCommandOption(cmd, namespace, name, options.SSHPort))

	if len(options.AdditionalSSHLocalOptions) > 0 {
		args = append(args, options.AdditionalSSHLocalOptions...)
	}
	if options.IdentityFilePathProvided {
		args = append(args, "-i", options.IdentityFilePath)
	}

	args = append(args, clientArgs...)

	command := exec.Command(options.LocalClientName, args...)
	klog.V(3).Info("running: ", command)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin

	return command.Run()
}

func buildProxyCommandOption(cmd *cobra.Command, namespace, name string, port int) string {
	parents := make([]string, 0, 2)
	for {
		if !cmd.HasParent() {
			break
		}
		cmd = cmd.Parent()
		parents = append(parents, cmd.Name())
	}
	parents[len(parents)-1] = os.Args[0]
	pcmd := strings.Builder{}

	for i := 1; i <= len(parents); i++ {
		pcmd.WriteString(parents[len(parents)-i])
		pcmd.WriteString(" ")
	}

	proxyCommand := strings.Builder{}
	proxyCommand.WriteString("ProxyCommand=")
	proxyCommand.WriteString(pcmd.String())
	proxyCommand.WriteString("port-forward --stdio=true ")
	proxyCommand.WriteString(fmt.Sprintf("%s.%s", name, namespace))
	proxyCommand.WriteString(" ")

	proxyCommand.WriteString(strconv.Itoa(port))

	return proxyCommand.String()
}

func (o *SSH) buildSSHTarget(namespace, name string) (opts []string) {
	target := strings.Builder{}
	if len(o.options.SSHUsername) > 0 {
		target.WriteString(o.options.SSHUsername)
		target.WriteRune('@')
	}
	target.WriteString(name)
	target.WriteRune('.')
	target.WriteString(namespace)

	opts = append(opts, target.String())
	if o.command != "" {
		opts = append(opts, o.command)
	}
	return
}
