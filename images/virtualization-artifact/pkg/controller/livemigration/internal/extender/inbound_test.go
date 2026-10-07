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

package extender

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

const namespace = "default"

var _ = Describe("InboundMigrationExtender", func() {
	var (
		ctx   context.Context
		nodes []string
	)

	BeforeEach(func() {
		ctx = context.Background()
		nodes = []string{"node-a", "node-b", "node-c"}
	})

	newExtenderWith := func(settings Settings, objs ...client.Object) *InboundMigrationExtender {
		GinkgoHelper()
		fakeClient, err := testutil.NewFakeClientWithObjects(objs...)
		Expect(err).NotTo(HaveOccurred())
		return NewInboundMigrationExtender(fakeClient, settings, testutil.NewNoOpLogger())
	}

	newExtender := func(objs ...client.Object) *InboundMigrationExtender {
		GinkgoHelper()
		return newExtenderWith(Settings{}, objs...)
	}

	scores := func(e *InboundMigrationExtender, pod *corev1.Pod) map[string]int64 {
		GinkgoHelper()
		result := make(map[string]int64)
		for _, p := range e.Prioritize(ctx, Args{Pod: pod, NodeNames: &nodes}) {
			result[p.Host] = p.Score
		}
		return result
	}

	It("gives no bonus to a pod that is not a migration target", func() {
		e := newExtender(arrivingMigration("vm-1", "node-a", false)...)

		Expect(scores(e, newPod("vm-new", nil))).To(Equal(map[string]int64{"node-a": 0, "node-b": 0, "node-c": 0}))
	})

	It("gives no bonus to a hotplug attachment pod of a migration", func() {
		e := newExtender(arrivingMigration("vm-1", "node-a", false)...)
		pod := newPod("hp-vm-2", map[string]string{
			launcherAppLabel:  "hotplug-disk",
			migrationJobLabel: "uid-vm-2",
		})

		Expect(scores(e, pod)).To(HaveEach(BeZero()))
	})

	It("prefers the nodes no migration target heads to", func() {
		e := newExtender(arrivingMigration("vm-1", "node-a", false)...)

		Expect(scores(e, schedulingTarget("vm-2"))).To(Equal(map[string]int64{
			"node-a": 0,
			"node-b": freeNodeScore,
			"node-c": freeNodeScore,
		}))
	})

	It("gives no bonus to any node a migration target already heads to", func() {
		objs := append(arrivingMigration("vm-1", "node-a", false), arrivingMigration("vm-2", "node-b", false)...)
		e := newExtender(objs...)

		Expect(scores(e, schedulingTarget("vm-3"))).To(Equal(map[string]int64{
			"node-a": 0,
			"node-b": 0,
			"node-c": freeNodeScore,
		}))
	})

	It("keeps the bonus well below the resource scoring", func() {
		// kube-scheduler scales a score of 10 to 100 points, the maximum of one resource plugin.
		Expect(freeNodeScore).To(BeNumerically("<=", 1))
	})

	It("does not count finished migrations", func() {
		e := newExtender(arrivingMigration("vm-1", "node-a", true)...)

		Expect(scores(e, schedulingTarget("vm-2"))).To(HaveKeyWithValue("node-a", freeNodeScore))
	})

	It("does not count a pod of a deleted migration", func() {
		objs := arrivingMigration("vm-1", "node-a", false)
		e := newExtender(objs[1:]...)

		Expect(scores(e, schedulingTarget("vm-2"))).To(HaveKeyWithValue("node-a", freeNodeScore))
	})

	It("does not count a pod whose migration name was reused by a newer migration", func() {
		objs := arrivingMigration("vm-1", "node-a", false)
		objs[0].(*virtv1.VirtualMachineInstanceMigration).UID = "uid-newer"
		e := newExtender(objs...)

		Expect(scores(e, schedulingTarget("vm-2"))).To(HaveKeyWithValue("node-a", freeNodeScore))
	})

	It("keeps every candidate on the filter verb", func() {
		e := newExtender(arrivingMigration("vm-1", "node-a", false)...)

		Expect(*e.Filter(ctx, Args{Pod: schedulingTarget("vm-2"), NodeNames: &nodes}).NodeNames).To(Equal(nodes))
	})

	It("reports an error on the filter verb when node names are not sent", func() {
		e := newExtender()

		result := e.Filter(ctx, Args{Pod: schedulingTarget("vm-1")})

		Expect(result.NodeNames).To(BeNil())
		Expect(result.Error).NotTo(BeEmpty())
	})

	It("returns nothing when node names are not sent", func() {
		e := newExtender()

		Expect(e.Prioritize(ctx, Args{Pod: schedulingTarget("vm-1")})).To(BeNil())
	})

	Context("with a budget shared by incoming and outgoing migrations", func() {
		It("counts the outgoing migrations of a node as its load", func() {
			e := newExtenderWith(Settings{CountOutgoing: true}, departingMigration("vm-1", "node-a")...)

			Expect(scores(e, schedulingTarget("vm-2"))).To(Equal(map[string]int64{
				"node-a": 0,
				"node-b": freeNodeScore,
				"node-c": freeNodeScore,
			}))
		})

		It("does not count outgoing migrations with the separate limits", func() {
			e := newExtender(departingMigration("vm-1", "node-a")...)

			Expect(scores(e, schedulingTarget("vm-2"))).To(HaveKeyWithValue("node-a", freeNodeScore))
		})
	})
})

// schedulingTarget returns a migration target pod as kube-scheduler sends it: not bound yet and
// with the labels in the form kube-api-rewriter stores them.
func schedulingTarget(vmName string) *corev1.Pod {
	return newPod("virt-launcher-"+vmName+"-target", map[string]string{
		launcherAppLabel:  launcherAppName,
		migrationJobLabel: "uid-" + vmName,
	})
}

// arrivingMigration returns a migration and its target pod bound to the node, as the controller
// cache shows them.
func arrivingMigration(vmName, node string, finished bool) []client.Object {
	vmim := &virtv1.VirtualMachineInstanceMigration{
		ObjectMeta: metav1.ObjectMeta{Name: "migration-" + vmName, Namespace: namespace, UID: types.UID("uid-" + vmName)},
		Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: vmName},
		Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationScheduled},
	}
	if finished {
		vmim.Status.Phase = virtv1.MigrationSucceeded
	}

	pod := newPod("virt-launcher-"+vmName+"-target", map[string]string{
		virtv1.AppLabel:          "virt-launcher",
		virtv1.MigrationJobLabel: string(vmim.UID),
	})
	pod.Annotations = map[string]string{virtv1.MigrationJobNameAnnotation: vmim.Name}
	pod.Spec.NodeName = node
	pod.Status.Phase = corev1.PodRunning

	return []client.Object{vmim, pod}
}

// departingMigration returns a migration transferring memory from the node and its VMI.
func departingMigration(vmName, node string) []client.Object {
	vmim := &virtv1.VirtualMachineInstanceMigration{
		ObjectMeta: metav1.ObjectMeta{Name: "migration-" + vmName, Namespace: namespace, UID: types.UID("uid-" + vmName)},
		Spec:       virtv1.VirtualMachineInstanceMigrationSpec{VMIName: vmName},
		Status:     virtv1.VirtualMachineInstanceMigrationStatus{Phase: virtv1.MigrationRunning},
	}
	kvvmi := &virtv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{Name: vmName, Namespace: namespace},
		Status: virtv1.VirtualMachineInstanceStatus{MigrationState: &virtv1.VirtualMachineInstanceMigrationState{
			MigrationUID: vmim.UID,
			SourceNode:   node,
		}},
	}
	return []client.Object{vmim, kvvmi}
}

func newPod(name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels}}
}
