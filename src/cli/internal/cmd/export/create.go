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

package export

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"

	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

const (
	createExample = `  # Create an export for VirtualDataExport 'myvdexport':
  {{ProgramName}} export create vd myvdexport -n mynamespace
  {{ProgramName}} export create vd myvdexport -n mynamespace --timeout 1h
  {{ProgramName}} export create vd myvdexport
  {{ProgramName}} export create vdsnapshot myvdexport
  {{ProgramName}} export create vi myvdexport
  {{ProgramName}} export create cvi myvdexport`
)

type create struct {
	timeout      string
	virtualdisk  bool
	virtualimage bool
}

func newExportCreateCommand() *cobra.Command {
	c := &create{}
	cmd := &cobra.Command{
		Use:     "create (VirtualDataExport)",
		Short:   "Create an export.",
		Example: createExample,
		Args:    templates.ExactArgs("create", 2),
		RunE:    c.Run,
	}

	cmd.Flags().StringVarP(&c.timeout, "timeout", "t", "", "Timeout for export")
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

func (c *create) Run(cmd *cobra.Command, args []string) error {
	client, namespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}
	kind, name := args[0], args[1]
	ref, err := parseTargetRef(kind, name)
	if err != nil {
		return err
	}
	timeout, err := parseTimeout(c.timeout)
	if err != nil {
		return err
	}

	export := newVDExport(name, namespace, ref, timeout)
	_, err = client.VirtualDataExports(namespace).Create(cmd.Context(), export, metav1.CreateOptions{})

	return err
}

func parseTargetRef(kind, name string) (virtv2.VirtualDataExportTargetRef, error) {
	targetRef := virtv2.VirtualDataExportTargetRef{
		Name: name,
	}
	switch kind {
	case "vd", "virtualdisk", "VirtualDisk":
		targetRef.Kind = virtv2.VirtualDataExportTargetVirtualDisk
	case "vdsnapshot", "virtualdisksnapshot", "VirtualDiskSnapshot":
		targetRef.Kind = virtv2.VirtualDataExportTargetVirtualDiskSnapshot
	case "vi", "virtualimage", "VirtualImage":
		targetRef.Kind = virtv2.VirtualDataExportTargetVirtualImage
	case "cvi", "clustervirtualimage", "ClusterVirtualImage":
		targetRef.Kind = virtv2.VirtualDataExportTargetClusterVirtualImage
	default:
		return targetRef, fmt.Errorf("unknown target kind %q", kind)
	}
	return targetRef, nil
}

func parseTimeout(timeout string) (*metav1.Duration, error) {
	if timeout == "" {
		return nil, nil
	}
	t, err := time.ParseDuration(timeout)
	if err != nil {
		return nil, err
	}
	return &metav1.Duration{Duration: t}, nil
}

func newVDExport(baseName, namespace string, targetRef virtv2.VirtualDataExportTargetRef, timeout *metav1.Duration) *virtv2.VirtualDataExport {
	vdExport := &virtv2.VirtualDataExport{
		TypeMeta: metav1.TypeMeta{
			Kind:       virtv2.VirtualDataExportKind,
			APIVersion: virtv2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: baseName + "-",
			Namespace:    namespace,
		},
		Spec: virtv2.VirtualDataExportSpec{
			TargetRef: targetRef,
		},
	}
	if timeout != nil {
		vdExport.Spec.IdleTimeout = *timeout
	}
	return vdExport
}
