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

package collectdebuginfo

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

func NewCommand() *cobra.Command {
	bundle := &DebugBundle{}
	cmd := &cobra.Command{
		Use:     "collect-debug-info (VirtualMachine)",
		Short:   "Collect debug information for VM: configuration, events, and logs. Output is written to stdout.",
		Example: usage(),
		Args:    templates.ExactArgs("collect-debug-info", 1),
		RunE:    bundle.Run,
	}

	cmd.Flags().BoolVar(&bundle.saveLogs, "with-logs", false, "Include pod logs in output")
	cmd.Flags().BoolVar(&bundle.debug, "debug", false, "Enable debug output for permission errors")
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

type DebugBundle struct {
	saveLogs      bool
	debug         bool
	dynamicClient dynamic.Interface
	stdout        io.Writer
	stderr        io.Writer
	resourceCount int
}

func usage() string {
	return `  # Collect debug info for VirtualMachine 'myvm' (output to stdout):
  {{ProgramName}} collect-debug-info myvm
  {{ProgramName}} collect-debug-info myvm.mynamespace
  {{ProgramName}} collect-debug-info myvm -n mynamespace

  # Include pod logs:
  {{ProgramName}} collect-debug-info --with-logs myvm

  # Save compressed output to file:
  {{ProgramName}} collect-debug-info --with-logs myvm | gzip > debug-info.yaml.gz`
}

func (b *DebugBundle) Run(cmd *cobra.Command, args []string) error {
	client, defaultNamespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}

	namespace, name, err := templates.ParseTarget(args[0])
	if err != nil {
		return err
	}
	if namespace == "" {
		namespace = defaultNamespace
	}

	config, err := clientconfig.GetRESTConfig(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get REST config: %w", err)
	}
	b.dynamicClient, err = dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	b.stdout = cmd.OutOrStdout()
	b.stderr = cmd.ErrOrStderr()

	if err := b.collectResources(cmd.Context(), client, namespace, name); err != nil {
		return err
	}

	return nil
}

func (b *DebugBundle) collectResources(ctx context.Context, client kubeclient.Client, namespace, vmName string) error {
	if err := b.collectVMResources(ctx, client, namespace, vmName); err != nil {
		return fmt.Errorf("failed to collect VM resources: %w", err)
	}

	if err := b.collectBlockDevices(ctx, client, namespace, vmName); err != nil {
		return fmt.Errorf("failed to collect block devices: %w", err)
	}

	if err := b.collectPods(ctx, client, namespace, vmName); err != nil {
		return fmt.Errorf("failed to collect pods: %w", err)
	}

	return nil
}

func (b *DebugBundle) handleError(resourceType, resourceName string, err error) bool {
	if errors.IsForbidden(err) || errors.IsUnauthorized(err) {
		if b.debug {
			fmt.Fprintf(b.stderr, "Warning: Skipping %s/%s: permission denied\n", resourceType, resourceName)
		}
		return true
	}
	return false
}
