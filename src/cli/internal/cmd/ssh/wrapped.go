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

func addLocalSSHClientFlags(flagset *pflag.FlagSet, opts *SSHOptions) {
	flagset.StringArrayVar(&opts.AdditionalSSHOptions, additionalOpts, opts.AdditionalSSHOptions,
		"Additional options to be passed to the local SSH/SCP client. "+
			"May be repeated, or a single value may contain several options separated by spaces "+
			"(e.g. --ssh-args='-o X -o Y').")
	flagset.StringArrayVar(&opts.AdditionalSSHLocalOptions, additionalLocalOpts, opts.AdditionalSSHLocalOptions,
		"Deprecated: use --ssh-args instead")
	flagset.BoolVar(&opts.WrapLocalSSH, wrapLocalSSHFlag, opts.WrapLocalSSH,
		"Deprecated: local SSH/SCP client is always used")
}

func WarnDeprecatedSSHFlags(cmd *cobra.Command) {
	if cmd.Flags().Changed(wrapLocalSSHFlag) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: --%s is deprecated and has no effect; local SSH/SCP client is always used.\n", wrapLocalSSHFlag)
	}
	if cmd.Flags().Changed(additionalLocalOpts) {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: --%s is deprecated; use --%s instead.\n", additionalLocalOpts, additionalOpts)
	}
}

// flattenSSHArgs splits each entry of additionalArgs by whitespace and
// returns a single flat slice. This allows values such as
// `--ssh-args='-o X -o Y'` to be passed as a single flag occurrence.
func flattenSSHArgs(additionalArgs []string) []string {
	var flat []string
	for _, entry := range additionalArgs {
		flat = append(flat, strings.Fields(entry)...)
	}
	return flat
}

func RunLocalClient(cmd *cobra.Command, namespace, name string, options *SSHOptions, clientArgs []string) error {
	args := []string{"-o"}
	args = append(args, buildProxyCommandOption(cmd, namespace, name, options.SSHPort))

	if len(options.AdditionalSSHOptions) > 0 {
		args = append(args, flattenSSHArgs(options.AdditionalSSHOptions)...)
	}
	if len(options.AdditionalSSHLocalOptions) > 0 {
		args = append(args, flattenSSHArgs(options.AdditionalSSHLocalOptions)...)
	}
	if options.KnownHostsFilePathProvided {
		args = append(args, "-o", "UserKnownHostsFile="+options.KnownHostsFilePath)
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
	for cmd.HasParent() {
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
	fmt.Fprintf(&proxyCommand, "%s.%s", name, namespace)
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
	return opts
}
