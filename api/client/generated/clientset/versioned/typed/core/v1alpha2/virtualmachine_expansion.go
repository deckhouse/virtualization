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

	"github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

type VirtualMachineExpansion interface {
	// SerialConsole connects to the serial console of the virtual machine, or, with Probe set in
	// the options, reports who holds it right now and leaves them alone: connecting is exclusive,
	// so a client is expected to probe first and warn about the user it is about to disconnect.
	// Exactly one of the two is returned, the stream or the session, depending on Probe.
	SerialConsole(ctx context.Context, name string, options *SerialConsoleOptions) (StreamInterface, *v1alpha2.VirtualMachineSession, error)
	// VNC connects to the VNC of the virtual machine. See SerialConsole about Probe.
	VNC(ctx context.Context, name string, options *VNCOptions) (StreamInterface, *v1alpha2.VirtualMachineSession, error)
	PortForward(name string, opts v1alpha2.VirtualMachinePortForward) (StreamInterface, error)
	Freeze(ctx context.Context, name string, opts v1alpha2.VirtualMachineFreeze) error
	Unfreeze(ctx context.Context, name string) error
	AddVolume(ctx context.Context, name string, opts v1alpha2.VirtualMachineAddVolume) error
	RemoveVolume(ctx context.Context, name string, opts v1alpha2.VirtualMachineRemoveVolume) error
	CancelEvacuation(ctx context.Context, name string, dryRun []string) error
	AddResourceClaim(ctx context.Context, name string, opts v1alpha2.VirtualMachineAddResourceClaim) error
	RemoveResourceClaim(ctx context.Context, name string, opts v1alpha2.VirtualMachineRemoveResourceClaim) error
}

type SerialConsoleOptions struct {
	ConnectionTimeout time.Duration
	// Probe asks who holds the serial console instead of connecting to it.
	Probe bool
}

type VNCOptions struct {
	// Probe asks who holds the VNC instead of connecting to it.
	Probe bool
}

type StreamOptions struct {
	In  io.Reader
	Out io.Writer
}

type StreamInterface interface {
	Stream(options StreamOptions) error
	AsConn() net.Conn
}

func (c *virtualMachines) SerialConsole(_ context.Context, name string, options *SerialConsoleOptions) (StreamInterface, *v1alpha2.VirtualMachineSession, error) {
	return nil, nil, fmt.Errorf("not implemented")
}

func (c *virtualMachines) VNC(_ context.Context, name string, options *VNCOptions) (StreamInterface, *v1alpha2.VirtualMachineSession, error) {
	return nil, nil, fmt.Errorf("not implemented")
}

func (c *virtualMachines) PortForward(name string, opts v1alpha2.VirtualMachinePortForward) (StreamInterface, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *virtualMachines) Freeze(ctx context.Context, name string, opts v1alpha2.VirtualMachineFreeze) error {
	return fmt.Errorf("not implemented")
}

func (c *virtualMachines) Unfreeze(ctx context.Context, name string) error {
	return fmt.Errorf("not implemented")
}

func (c *virtualMachines) AddVolume(ctx context.Context, name string, opts v1alpha2.VirtualMachineAddVolume) error {
	return fmt.Errorf("not implemented")
}

func (c *virtualMachines) RemoveVolume(ctx context.Context, name string, opts v1alpha2.VirtualMachineRemoveVolume) error {
	return fmt.Errorf("not implemented")
}

func (c *virtualMachines) CancelEvacuation(ctx context.Context, name string, dryRun []string) error {
	return fmt.Errorf("not implemented")
}

func (c *virtualMachines) AddResourceClaim(ctx context.Context, name string, opts v1alpha2.VirtualMachineAddResourceClaim) error {
	return fmt.Errorf("not implemented")
}

func (c *virtualMachines) RemoveResourceClaim(ctx context.Context, name string, opts v1alpha2.VirtualMachineRemoveResourceClaim) error {
	return fmt.Errorf("not implemented")
}
