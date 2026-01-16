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
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

func NewCommand() *cobra.Command {
	bundle := &DebugBundle{}
	cmd := &cobra.Command{
		Use:     "collect-debug-info (VirtualMachine)",
		Short:   "Collect debug information for VM: configuration, events, and logs. Output is written as compressed archive to stdout.",
		Example: usage(),
		Args:    templates.ExactArgs("collect-debug-info", 1),
		RunE:    bundle.Run,
	}

	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

type DebugBundle struct {
	dynamicClient dynamic.Interface
	restConfig    *rest.Config
	stdout        io.Writer
	stderr        io.Writer
	tarWriter     *tar.Writer
	gzipWriter    *gzip.Writer
	fileCount     int
}

func usage() string {
	return `  # Collect debug info for VirtualMachine 'myvm' (output compressed archive to stdout):
  {{ProgramName}} collect-debug-info myvm > debug-info.tar.gz
  {{ProgramName}} collect-debug-info myvm.mynamespace > debug-info.tar.gz
  {{ProgramName}} collect-debug-info myvm -n mynamespace > debug-info.tar.gz`
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
	b.restConfig = config
	b.dynamicClient, err = dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	b.stdout = cmd.OutOrStdout()
	b.stderr = cmd.ErrOrStderr()

	// Check if stdout is a terminal - if so, don't output archive
	if term.IsTerminal(int(os.Stdout.Fd())) {
		_, _ = fmt.Fprintf(b.stderr, "Error: Output is being written to terminal. Please redirect to a file:\n")
		_, _ = fmt.Fprintf(b.stderr, "  %s collect-debug-info %s > debug-info.tar.gz\n", cmd.CommandPath(), args[0])
		return fmt.Errorf("output must be redirected to a file")
	}

	// Initialize archive writers
	b.gzipWriter = gzip.NewWriter(b.stdout)
	defer func() {
		_ = b.gzipWriter.Close()
	}()
	b.tarWriter = tar.NewWriter(b.gzipWriter)
	defer func() {
		_ = b.tarWriter.Close()
	}()

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
		_, _ = fmt.Fprintf(b.stderr, "Warning: Skipping %s/%s: permission denied\n", resourceType, resourceName)
		return true
	}
	return false
}
