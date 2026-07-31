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
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"

	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

const testNodeName = "test-node"

func newTestController() *USBGatewayController {
	return &USBGatewayController{
		nodeName: testNodeName,
		queue: workqueue.NewTypedRateLimitingQueue(
			workqueue.DefaultTypedControllerRateLimiter[string](),
		),
		log: slog.Default(),
	}
}

func TestGetAttachInfoNotCollected(t *testing.T) {
	c := newTestController()

	if _, ok := c.getAttachInfo(); ok {
		t.Fatalf("getAttachInfo() ok = true before any store, want false")
	}
}

func TestGetAttachInfoAfterStore(t *testing.T) {
	c := newTestController()

	want := usbip.AttachInfo{NPorts: 8}
	c.storeAttachInfo(want)

	got, ok := c.getAttachInfo()
	if !ok {
		t.Fatalf("getAttachInfo() ok = false after store, want true")
	}
	if got.NPorts != want.NPorts {
		t.Fatalf("getAttachInfo() NPorts = %d, want %d", got.NPorts, want.NPorts)
	}
}

// A Sync running before Start collected the attach info must requeue instead of
// panicking on the empty atomic.Value.
func TestEnsureAttachInfoNotCollectedDoesNotPanic(t *testing.T) {
	c := newTestController()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: testNodeName}}

	got, err := c.ensureAttachInfo(context.Background(), node)
	if err != nil {
		t.Fatalf("ensureAttachInfo() error = %v, want nil", err)
	}
	if got != node {
		t.Fatalf("ensureAttachInfo() returned a modified node, want the original")
	}
}

func TestMarkNodeNotCollectedDoesNotPanic(t *testing.T) {
	c := newTestController()

	if err := c.markNode(context.Background()); err != nil {
		t.Fatalf("markNode() error = %v, want nil", err)
	}
}
