/*
Copyright 2026 Flant JSC

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

package usbgateway

import (
	"context"
	"fmt"

	"k8s.io/client-go/dynamic"

	"github.com/deckhouse/virtualization-dra/internal/consts"
	"github.com/deckhouse/virtualization-dra/internal/usb-gateway/labeler"
)

type Marker struct {
	nodeName string
	labeler  labeler.NodeLabeler
}

func NewMarker(dynamicClient dynamic.Interface, nodeName string) *Marker {
	return &Marker{
		nodeName: nodeName,
		labeler:  labeler.NewNodeLabeler(dynamicClient),
	}
}

func (m Marker) Mark(ctx context.Context) error {
	err := m.labeler.Label(ctx, m.nodeName, "", map[string]string{
		consts.USBGatewayLabel: "true",
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to label node %s: %w", m.nodeName, err)
	}
	return nil
}

func (m Marker) Unmark(ctx context.Context) error {
	err := m.labeler.Label(ctx, m.nodeName, "", nil, []string{consts.USBGatewayLabel})
	if err != nil {
		return fmt.Errorf("failed to unlabel node %s: %w", m.nodeName, err)
	}
	return nil
}
