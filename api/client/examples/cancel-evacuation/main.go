/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()
	if err := NewCancelEvacuationCommand().ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

}

type MigrateCancelOptions struct {
	Namespace string
}

func (o *MigrateCancelOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.Namespace, "namespace", "n", "", "namespace of virtual machine")
}

func NewCancelEvacuationCommand() *cobra.Command {
	opts := MigrateCancelOptions{}

	cmd := cobra.Command{
		Use:           "cancel-evacuation [virtual machine name]",
		Short:         "cancel evacuation command",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), args[0], opts.Namespace)
		},
	}

	flagset := cmd.Flags()
	opts.AddFlags(flagset)

	return &cmd
}

func run(ctx context.Context, name, namespace string) (err error) {
	clientConfig := kubeclient.DefaultClientConfig(&pflag.FlagSet{})

	if namespace == "" {
		namespace, _, err = clientConfig.Namespace()
		if err != nil {
			return err
		}

	}

	client, err := kubeclient.GetClientFromClientConfig(clientConfig)
	if err != nil {
		return err
	}

	return client.VirtualMachines(namespace).CancelEvacuation(ctx, name, nil)
}
