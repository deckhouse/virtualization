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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func main() {
	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	if err := NewResourceClaimCommand().ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func NewResourceClaimCommand() *cobra.Command {
	cmd := cobra.Command{
		Use:           "resourceclaim (VirtualMachine)",
		Short:         "add/remove resource claim to/from a VirtualMachine.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.AddCommand(
		NewAddResourceClaimCommand(),
		NewRemoveResourceClaimCommand(),
	)

	return &cmd
}

func NewAddResourceClaimCommand() *cobra.Command {
	opts := &addResourceClaimOptions{}

	cmd := cobra.Command{
		Use:           "add (VirtualMachine)",
		Short:         "add resource claim to a VirtualMachine.",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          opts.Run,
	}

	opts.AddFlags(cmd.Flags())

	return &cmd
}

type addResourceClaimOptions struct {
	Namespace                 string
	HotplugName               string
	RequestName               string
	ResourceClaimTemplateName string
	DryRun                    bool
}

func (o *addResourceClaimOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.Namespace, "namespace", "n", "", "namespace of virtual machine")
	fs.StringVar(&o.HotplugName, "hotplug-name", "", "name of the hotplug device")
	fs.StringVarP(&o.RequestName, "request-name", "r", "", "name of the resource claim request")
	fs.StringVarP(&o.ResourceClaimTemplateName, "resource-claim-template-name", "t", "", "name of the resource claim template")
	fs.BoolVarP(&o.DryRun, "dry-run", "d", false, "dry run")
}

func (o *addResourceClaimOptions) Validate() error {
	if o.HotplugName == "" {
		return fmt.Errorf("hotplug-name is required")
	}
	if o.RequestName == "" {
		return fmt.Errorf("request-name is required")
	}
	if o.ResourceClaimTemplateName == "" {
		return fmt.Errorf("resource-claim-template-name is required")
	}
	return nil
}

func (o *addResourceClaimOptions) Run(cmd *cobra.Command, args []string) error {
	err := o.Validate()
	if err != nil {
		return err
	}

	client, namespace, err := getClientAndNamespace(o.Namespace)
	if err != nil {
		return err
	}

	name := args[0]
	options := subv1alpha2.VirtualMachineAddResourceClaim{
		Name:                      o.HotplugName,
		ResourceClaimTemplateName: o.ResourceClaimTemplateName,
		RequestName:               o.RequestName,
		DryRun:                    dryRun(o.DryRun),
	}

	cmd.Println("Options:", options)

	return client.VirtualMachines(namespace).AddResourceClaim(cmd.Context(), name, options)
}

func NewRemoveResourceClaimCommand() *cobra.Command {
	opts := removeResourceClaimOptions{}

	cmd := cobra.Command{
		Use:           "remove (VirtualMachine)",
		Short:         "remove resource claim from a VirtualMachine.",
		Args:          cobra.ExactArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE:          opts.Run,
	}

	opts.AddFlags(&cmd)

	return &cmd
}

type removeResourceClaimOptions struct {
	Namespace   string
	HotplugName string
	DryRun      bool
}

func (o *removeResourceClaimOptions) AddFlags(cmd *cobra.Command) {
	fs := cmd.Flags()
	fs.StringVarP(&o.Namespace, "namespace", "n", "", "namespace of virtual machine")
	fs.StringVar(&o.HotplugName, "hotplug-name", "", "name of the hotplug device")
	fs.BoolVarP(&o.DryRun, "dry-run", "d", false, "dry run")
}

func (o *removeResourceClaimOptions) Validate() error {
	if o.HotplugName == "" {
		return fmt.Errorf("hotplug-name is required")
	}
	return nil
}

func (o *removeResourceClaimOptions) Run(cmd *cobra.Command, args []string) error {
	err := o.Validate()
	if err != nil {
		return err
	}

	client, namespace, err := getClientAndNamespace(o.Namespace)
	if err != nil {
		return err
	}

	name := args[0]
	options := subv1alpha2.VirtualMachineRemoveResourceClaim{
		Name:   o.HotplugName,
		DryRun: dryRun(o.DryRun),
	}

	return client.VirtualMachines(namespace).RemoveResourceClaim(cmd.Context(), name, options)
}

func getClientAndNamespace(defaultNamespace string) (kubeclient.Client, string, error) {
	namespace := defaultNamespace
	clientConfig := kubeclient.DefaultClientConfig(&pflag.FlagSet{})

	if namespace == "" {
		ns, _, err := clientConfig.Namespace()
		if err != nil {
			return nil, "", err
		}
		namespace = ns
	}

	client, err := kubeclient.GetClientFromClientConfig(clientConfig)
	if err != nil {
		return nil, "", err
	}

	return client, namespace, nil
}

func dryRun(should bool) []string {
	if should {
		return []string{metav1.DryRunAll}
	}
	return nil
}
