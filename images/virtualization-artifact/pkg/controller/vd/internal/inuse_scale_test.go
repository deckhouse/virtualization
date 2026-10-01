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

package internal

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// scaleNamespace returns a namespace of vms running virtual machines unrelated to the disk, and one that owns it.
func scaleNamespace(vms int) []client.Object {
	const ns = "default"
	objs := []client.Object{
		&v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-scale", Namespace: ns},
			Status:     v1alpha2.VirtualDiskStatus{AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{{Name: "owner", Mounted: true}}},
		},
	}
	running := func(name string, disks ...string) {
		vm := &v1alpha2.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Status:     v1alpha2.VirtualMachineStatus{Phase: v1alpha2.MachineRunning},
		}
		for _, d := range disks {
			vm.Status.BlockDeviceRefs = append(vm.Status.BlockDeviceRefs, v1alpha2.BlockDeviceStatusRef{Kind: v1alpha2.DiskDevice, Name: d})
		}
		kvvmi := &virtv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		for _, d := range disks {
			kvvmi.Status.VolumeStatus = append(kvvmi.Status.VolumeStatus, virtv1.VolumeStatus{Name: "vd-" + d})
		}
		objs = append(objs, vm, kvvmi, &virtv1.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}})
	}
	running("owner", "vd-scale")
	for i := range vms {
		running(fmt.Sprintf("vm-%d", i), fmt.Sprintf("vd-other-%d", i))
	}
	return objs
}

// fataler is what both GinkgoT() and *testing.B provide.
type fataler interface {
	Helper()
	Fatal(args ...any)
}

// internalVMReads counts the reads of the internal virtual machines and their instances one reconcile makes.
func internalVMReads(tb fataler, vms int) int64 {
	tb.Helper()
	var reads atomic.Int64
	count := func(obj client.Object) {
		switch obj.(type) {
		case *virtv1.VirtualMachine, *virtv1.VirtualMachineInstance:
			reads.Add(1)
		}
	}
	c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
		Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			count(obj)
			return cl.Get(ctx, key, obj, opts...)
		},
		List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			switch list.(type) {
			case *virtv1.VirtualMachineList, *virtv1.VirtualMachineInstanceList:
				reads.Add(1)
			}
			return cl.List(ctx, list, opts...)
		},
	}, scaleNamespace(vms)...)
	if err != nil {
		tb.Fatal(err)
	}

	vd := &v1alpha2.VirtualDisk{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "vd-scale"}, vd); err != nil {
		tb.Fatal(err)
	}
	reads.Store(0)
	if _, err := NewInUseHandler(c).Handle(context.Background(), vd); err != nil {
		tb.Fatal(err)
	}
	return reads.Load()
}

// The owner election reads the internal virtual machines related to the disk only: a namespace of a nested
// cluster holds all its nodes and all their disks, and every disk reconcile would otherwise read them all.
var _ = Describe("InUseHandler in a namespace with many virtual machines", func() {
	It("reads as many internal virtual machines per disk reconcile with 10 and with 200 virtual machines", func() {
		Expect(internalVMReads(GinkgoT(), 200)).To(Equal(internalVMReads(GinkgoT(), 10)))
	})
})

func BenchmarkInUseHandler(b *testing.B) {
	for _, vms := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("vms=%d", vms), func(b *testing.B) {
			c, err := testutil.NewFakeClientWithObjects(scaleNamespace(vms)...)
			if err != nil {
				b.Fatal(err)
			}
			vd := &v1alpha2.VirtualDisk{}
			if err := c.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "vd-scale"}, vd); err != nil {
				b.Fatal(err)
			}
			h := NewInUseHandler(c)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := h.Handle(context.Background(), vd.DeepCopy()); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(internalVMReads(b, vms)), "internal-vm-reads/op")
		})
	}
}
