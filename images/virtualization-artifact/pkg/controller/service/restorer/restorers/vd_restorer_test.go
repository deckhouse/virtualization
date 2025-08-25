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

package restorer

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service/restorer/common"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualDiskTestArgs struct {
	mode common.OperationMode

	diskExists       bool
	diskUsedByDiffVM bool

	failValidation bool
	failProcess    bool

	shouldBeDeleted bool
	shouldBeCreated bool
}

var _ = Describe("VirtualDiskRestorer", func() {
	var (
		ctx context.Context
		err error

		uid       string
		vm        string
		name      string
		namespace string

		intercept   interceptor.Funcs
		diskDeleted bool
		diskCreated bool

		objects    []client.Object
		disk       v1alpha2.VirtualDisk
		handler    *VirtualDiskHandler
		fakeClient client.WithWatch
	)

	BeforeEach(func() {
		ctx = context.Background()
		uid = "0000-1111-2222-4444"
		name = "test-disk"
		namespace = "default"
		vm = "test-vm"
		diskDeleted = false
		diskCreated = false

		objects = []client.Object{}

		disk = v1alpha2.VirtualDisk{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: v1alpha2.VirtualDiskSpec{
				DataSource: &v1alpha2.VirtualDiskDataSource{
					Type: v1alpha2.DataSourceTypeObjectRef,
					ObjectRef: &v1alpha2.VirtualDiskObjectRef{
						Kind: v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot,
						Name: "test-vdsnapshot",
					},
				},
			},
			Status: v1alpha2.VirtualDiskStatus{
				AttachedToVirtualMachines: []v1alpha2.AttachedVirtualMachine{
					{Name: vm, Mounted: true},
				},
			},
		}

		intercept = interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				if obj.GetName() == disk.Name {
					_, ok := obj.(*v1alpha2.VirtualDisk)
					Expect(ok).To(BeTrue())
					diskDeleted = true
				}
				return nil
			},
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				if obj.GetName() == disk.Name {
					_, ok := obj.(*v1alpha2.VirtualDisk)
					Expect(ok).To(BeTrue())
					diskCreated = true
				}

				return nil
			},
		}
	})

	DescribeTable("restore",
		func(args VirtualDiskTestArgs) {
			if args.diskUsedByDiffVM {
				disk.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
					{Name: vm, Mounted: true},
					{Name: vm + "-2", Mounted: true},
				}
			}

			if args.diskExists {
				objects = append(objects, &disk)
			}

			fakeClient, err = testutil.NewFakeClientWithInterceptorWithObjects(intercept, objects...)
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeClient).ToNot(BeNil())

			disk.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{{Name: vm, Mounted: true}}
			handler = NewVirtualDiskHandler(fakeClient, args.mode, disk, uid)
			Expect(handler).ToNot(BeNil())

			err = handler.ValidateRestore(ctx)
			if args.failValidation {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			err = handler.ProcessRestore(ctx)
			if args.failProcess {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).ToNot(HaveOccurred())
			}

			Expect(diskDeleted).To(Equal(args.shouldBeDeleted))
			Expect(diskCreated).To(Equal(args.shouldBeCreated))
		},
		Entry("disk exists; used by different VM", VirtualDiskTestArgs{
			mode: common.StrictRestoreMode,

			diskExists:       true,
			diskUsedByDiffVM: true,

			failValidation: true,
			failProcess:    true,

			shouldBeDeleted: false,
			shouldBeCreated: false,
		}),
		Entry("disk exists; not used by different VM", VirtualDiskTestArgs{
			mode: common.StrictRestoreMode,

			diskExists:       true,
			diskUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: true,
			shouldBeCreated: true,
		}),
		Entry("disk doesn't exist", VirtualDiskTestArgs{
			mode: common.StrictRestoreMode,

			diskExists:       false,
			diskUsedByDiffVM: false,

			failValidation: false,
			failProcess:    false,

			shouldBeDeleted: false,
			shouldBeCreated: true,
		}),
	)
})
