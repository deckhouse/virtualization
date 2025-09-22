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

package v1alpha2

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/deckhouse/virtualization/api/subresources/v1alpha3"
)

type VirtualMachineExpansion interface {
	SerialConsole(name string, options *SerialConsoleOptions) (StreamInterface, error)
	VNC(name string) (StreamInterface, error)
	PortForward(name string, opts v1alpha3.VirtualMachinePortForward) (StreamInterface, error)
	Freeze(ctx context.Context, name string, opts v1alpha3.VirtualMachineFreeze) error
	Unfreeze(ctx context.Context, name string) error
	AddVolume(ctx context.Context, name string, opts v1alpha3.VirtualMachineAddVolume) error
	RemoveVolume(ctx context.Context, name string, opts v1alpha3.VirtualMachineRemoveVolume) error
	CancelEvacuation(ctx context.Context, name string, dryRun []string) error
}

type SerialConsoleOptions struct {
	ConnectionTimeout time.Duration
}
type StreamOptions struct {
	In  io.Reader
	Out io.Writer
}

type StreamInterface interface {
	Stream(options StreamOptions) error
	AsConn() net.Conn
}

func (c *virtualMachines) SerialConsole(name string, options *SerialConsoleOptions) (StreamInterface, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *virtualMachines) VNC(name string) (StreamInterface, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *virtualMachines) PortForward(name string, opts v1alpha3.VirtualMachinePortForward) (StreamInterface, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *virtualMachines) Freeze(ctx context.Context, name string, opts v1alpha3.VirtualMachineFreeze) error {
	return fmt.Errorf("not implemented")
}

func (c *virtualMachines) Unfreeze(ctx context.Context, name string) error {
	return fmt.Errorf("not implemented")
}

func (c *virtualMachines) AddVolume(ctx context.Context, name string, opts v1alpha3.VirtualMachineAddVolume) error {
	return fmt.Errorf("not implemented")
}

func (c *virtualMachines) RemoveVolume(ctx context.Context, name string, opts v1alpha3.VirtualMachineRemoveVolume) error {
	return fmt.Errorf("not implemented")
}

func (c *virtualMachines) CancelEvacuation(ctx context.Context, name string, dryRun []string) error {
	return fmt.Errorf("not implemented")
}
