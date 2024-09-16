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

package vmbda

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newUnsafeIterator(reader client.Reader) *iterator {
	return &iterator{
		reader: reader,
	}
}

type iterator struct {
	reader client.Reader
}

// Iter implements iteration on objects VMBDA and create new DTO.
// DO NOT mutate VMBDA!
func (l *iterator) Iter(ctx context.Context, h handler) error {
	vmbdas := virtv2.VirtualMachineBlockDeviceAttachmentList{}
	if err := l.reader.List(ctx, &vmbdas, client.UnsafeDisableDeepCopy); err != nil {
		return err
	}
	for _, vmbda := range vmbdas.Items {
		m := newDataMetric(&vmbda)
		if stop := h(m); stop {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			continue
		}
	}
	return nil
}