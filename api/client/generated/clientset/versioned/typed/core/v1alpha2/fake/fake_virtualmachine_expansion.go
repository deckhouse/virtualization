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

package fake

import (
	"context"

	corev1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

func (c *fakeVirtualMachines) SerialConsole(name string, options *corev1alpha2.SerialConsoleOptions) (corev1alpha2.StreamInterface, error) {
	return nil, nil
}

func (c *fakeVirtualMachines) VNC(name string) (corev1alpha2.StreamInterface, error) {
	return nil, nil
}

func (c *fakeVirtualMachines) PortForward(name string, opts v1alpha2.VirtualMachinePortForward) (corev1alpha2.StreamInterface, error) {
	return nil, nil
}

func (c *fakeVirtualMachines) Freeze(ctx context.Context, name string, opts v1alpha2.VirtualMachineFreeze) error {
	return nil
}

func (c *fakeVirtualMachines) Unfreeze(ctx context.Context, name string) error {
	return nil
}

func (c *fakeVirtualMachines) AddVolume(ctx context.Context, name string, opts v1alpha2.VirtualMachineAddVolume) error {
	return nil
}

func (c *fakeVirtualMachines) RemoveVolume(ctx context.Context, name string, opts v1alpha2.VirtualMachineRemoveVolume) error {
	return nil
}

func (c *fakeVirtualMachines) CancelEvacuation(ctx context.Context, name string, dryRun []string) error {
	return nil
}

func (c *fakeVirtualMachines) USBRedir(ctx context.Context, name string) (corev1alpha2.StreamInterface, error) {
	return nil, nil
}
