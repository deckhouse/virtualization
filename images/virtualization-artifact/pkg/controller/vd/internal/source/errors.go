/*
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
*/

package source

import (
	"errors"
	"fmt"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/source/step"
)

var (
	ErrSecretNotFound        = errors.New("container registry secret not found")
	ErrISOSourceNotSupported = step.ErrISOSourceNotSupported
)

type ImageNotReadyError struct {
	name string
}

func (e ImageNotReadyError) Error() string {
	return fmt.Sprintf("VirtualImage %q not ready", e.name)
}

func NewImageNotReadyError(name string) error {
	return ImageNotReadyError{
		name: name,
	}
}

type ImageNotFoundError struct {
	Name string
}

func (e ImageNotFoundError) Error() string {
	return fmt.Sprintf("VirtualImage %q not found", e.Name)
}

func NewImageNotFoundError(name string) error {
	return ImageNotFoundError{Name: name}
}

type ClusterImageNotReadyError struct {
	name string
}

func (e ClusterImageNotReadyError) Error() string {
	return fmt.Sprintf("ClusterVirtualImage %q not ready", e.name)
}

func NewClusterImageNotReadyError(name string) error {
	return ClusterImageNotReadyError{
		name: name,
	}
}

type ClusterImageNotFoundError struct {
	Name string
}

func (e ClusterImageNotFoundError) Error() string {
	return fmt.Sprintf("ClusterVirtualImage %q not found", e.Name)
}

func NewClusterImageNotFoundError(name string) error {
	return ClusterImageNotFoundError{Name: name}
}

type VirtualDiskSnapshotNotReadyError struct {
	name string
}

func (e VirtualDiskSnapshotNotReadyError) Error() string {
	return fmt.Sprintf("VirtualDiskSnapshot %q not ready", e.name)
}

func NewVirtualDiskSnapshotNotReadyError(name string) error {
	return VirtualDiskSnapshotNotReadyError{
		name: name,
	}
}
