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

package util

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

// PhaseTransition represents a single phase change observed during tracking.
type PhaseTransition struct {
	Phase string
	At    time.Time
}

// EventPhaseWatcher watches for phase changes using Kubernetes Watch API.
type EventPhaseWatcher struct {
	objectKey   types.NamespacedName
	transitions []PhaseTransition
	stopCh      chan struct{}
	mu          sync.Mutex
	gvr         schema.GroupVersionResource
}

// WatchPhases starts watching phase changes for the given object.
// Returns nil if GVR cannot be determined for the object's kind.
func WatchPhases(ctx context.Context, obj client.Object) *EventPhaseWatcher {
	gvk := obj.GetObjectKind().GroupVersionKind()
	// If GVK is not set on the object, try to get it from the scheme
	if gvk.Empty() {
		gvks, _, err := framework.GetClients().GenericClient().Scheme().ObjectKinds(obj)
		if err != nil || len(gvks) == 0 {
			return nil
		}
		gvk = gvks[0]
	}

	gvr, err := kindToResource(gvk)
	if err != nil {
		return nil
	}

	ew := &EventPhaseWatcher{
		objectKey: types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()},
		stopCh:    make(chan struct{}),
		gvr:       gvr,
	}

	go ew.run(ctx)
	return ew
}

// kindToResource converts GVK to GVR using API constants.
func kindToResource(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	switch gvk.Kind {
	case v1alpha2.VirtualDiskKind:
		return schema.GroupVersionResource{
			Group:    v1alpha2.SchemeGroupVersion.Group,
			Version:  v1alpha2.SchemeGroupVersion.Version,
			Resource: v1alpha2.VirtualDiskResource,
		}, nil
	case v1alpha2.VirtualImageKind:
		return schema.GroupVersionResource{
			Group:    v1alpha2.SchemeGroupVersion.Group,
			Version:  v1alpha2.SchemeGroupVersion.Version,
			Resource: v1alpha2.VirtualImageResource,
		}, nil
	case v1alpha2.ClusterVirtualImageKind:
		return schema.GroupVersionResource{
			Group:    v1alpha2.SchemeGroupVersion.Group,
			Version:  v1alpha2.SchemeGroupVersion.Version,
			Resource: v1alpha2.ClusterVirtualImageResource,
		}, nil
	case v1alpha2.VirtualDiskSnapshotKind:
		return schema.GroupVersionResource{
			Group:    v1alpha2.SchemeGroupVersion.Group,
			Version:  v1alpha2.SchemeGroupVersion.Version,
			Resource: v1alpha2.VirtualDiskSnapshotResource,
		}, nil
	case v1alpha2.VirtualMachineKind:
		return schema.GroupVersionResource{
			Group:    v1alpha2.SchemeGroupVersion.Group,
			Version:  v1alpha2.SchemeGroupVersion.Version,
			Resource: v1alpha2.VirtualMachineResource,
		}, nil
	case v1alpha2.VirtualMachineBlockDeviceAttachmentKind:
		return schema.GroupVersionResource{
			Group:    v1alpha2.SchemeGroupVersion.Group,
			Version:  v1alpha2.SchemeGroupVersion.Version,
			Resource: v1alpha2.VirtualMachineBlockDeviceAttachmentResource,
		}, nil
	case v1alpha2.VirtualMachineSnapshotKind:
		return schema.GroupVersionResource{
			Group:    v1alpha2.SchemeGroupVersion.Group,
			Version:  v1alpha2.SchemeGroupVersion.Version,
			Resource: v1alpha2.VirtualMachineSnapshotResource,
		}, nil
	// KubeVirt
	case "VirtualMachineInstance":
		return schema.GroupVersionResource{
			Group:    "kubevirt.io",
			Version:  "v1",
			Resource: "virtualmachineinstances",
		}, nil
	}

	return schema.GroupVersionResource{}, fmt.Errorf("GVR not found for %s", gvk.Kind)
}

func (ew *EventPhaseWatcher) run(ctx context.Context) {
	defer GinkgoRecover()

	dynClient := framework.GetClients().DynamicClient()

	watcher, err := dynClient.Resource(ew.gvr).
		Namespace(ew.objectKey.Namespace).
		Watch(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", ew.objectKey.Name),
		})
	if err != nil {
		return
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ew.stopCh:
			return
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return
			}
			ew.handleEvent(event)
		}
	}
}

func (ew *EventPhaseWatcher) handleEvent(event watch.Event) {
	obj, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		return
	}

	if event.Type == watch.Deleted {
		return
	}

	phase, found, err := unstructured.NestedString(obj.Object, "status", "phase")
	if err != nil || !found || phase == "" {
		return
	}

	name := obj.GetName()
	if ew.objectKey.Name != "" && name != ew.objectKey.Name {
		return
	}

	ew.mu.Lock()
	defer ew.mu.Unlock()

	if len(ew.transitions) > 0 && ew.transitions[len(ew.transitions)-1].Phase == phase {
		return
	}

	ew.transitions = append(ew.transitions, PhaseTransition{
		Phase: phase,
		At:    time.Now(),
	})
}

// Stop stops the watcher and returns all observed phase transitions
func (ew *EventPhaseWatcher) Stop() []PhaseTransition {
	select {
	case <-ew.stopCh:
	default:
		close(ew.stopCh)
	}
	ew.mu.Lock()
	defer ew.mu.Unlock()
	transitions := make([]PhaseTransition, len(ew.transitions))
	copy(transitions, ew.transitions)
	return transitions
}

// GetPhases returns all observed phases in order
func (ew *EventPhaseWatcher) GetPhases() []string {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	phases := make([]string, len(ew.transitions))
	for i, t := range ew.transitions {
		phases[i] = t.Phase
	}
	return phases
}

// GetTransitions returns all phase transitions with timestamps
func (ew *EventPhaseWatcher) GetTransitions() []PhaseTransition {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	transitions := make([]PhaseTransition, len(ew.transitions))
	copy(transitions, ew.transitions)
	return transitions
}

// ContainsPhase checks if the given phase was ever observed
func (ew *EventPhaseWatcher) ContainsPhase(phase string) bool {
	phases := ew.GetPhases()
	for _, p := range phases {
		if p == phase {
			return true
		}
	}
	return false
}

// ContainsAnyOfPhases checks if any of the given phases were observed
func (ew *EventPhaseWatcher) ContainsAnyOfPhases(phasesToCheck ...string) bool {
	phases := ew.GetPhases()
	for _, p := range phases {
		for _, check := range phasesToCheck {
			if p == check {
				return true
			}
		}
	}
	return false
}

// HasSequence checks if the given sequence appears as a subsequence
func (ew *EventPhaseWatcher) HasSequence(sequence ...string) bool {
	if len(sequence) == 0 {
		return true
	}

	phases := ew.GetPhases()
	if len(phases) < len(sequence) {
		return false
	}

	for i := 0; i <= len(phases)-len(sequence); i++ {
		found := true
		for j := 0; j < len(sequence); j++ {
			if phases[i+j] != sequence[j] {
				found = false
				break
			}
		}
		if found {
			return true
		}
	}
	return false
}

// HasConsecutiveSequence checks if the observed phases exactly match the sequence
func (ew *EventPhaseWatcher) HasConsecutiveSequence(sequence ...string) bool {
	phases := ew.GetPhases()
	if len(phases) != len(sequence) {
		return false
	}
	for i, p := range phases {
		if p != sequence[i] {
			return false
		}
	}
	return true
}

// VerifyPhaseTransitions verifies that the watcher observed the expected phase sequence.
// Logs all observed transitions and fails the test if sequence is not found.
func VerifyPhaseTransitions(watcher *EventPhaseWatcher, expectedSequence ...string) {
	if watcher == nil {
		Fail("EventPhaseWatcher is nil")
	}

	transitions := watcher.Stop()

	if len(transitions) > 0 {
		By("Observed phase transitions", func() {
			for i, t := range transitions {
				GinkgoWriter.Printf("Transition %d: %s at %v\n", i+1, t.Phase, t.At)
			}
		})
	}

	Expect(watcher.HasSequence(expectedSequence...)).To(BeTrue(),
		"Expected sequence: %v, got: %v", expectedSequence, watcher.GetPhases())
}
