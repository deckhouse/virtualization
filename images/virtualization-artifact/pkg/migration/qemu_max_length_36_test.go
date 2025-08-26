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

package migration

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestMigrationSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Migration Suite")
}

var _ = Describe("Migration Qemu Max Length 36", func() {
	newMigration := func(client client.WithWatch) (Migration, error) {
		return newQEMUMaxLength36(client, testutil.NewNoOpLogger())
	}

	DescribeTable("#MaxLength", func(expectFunc func(kvvmi *virtv1.VirtualMachineList), objs ...client.Object) {
		fakeClient, err := testutil.NewFakeClientWithObjects(objs...)
		Expect(err).NotTo(HaveOccurred())

		m, err := newMigration(fakeClient)
		Expect(err).ToNot(HaveOccurred())
		err = m.Migrate(context.Background())
		Expect(err).ToNot(HaveOccurred())

		vmList := &virtv1.VirtualMachineList{}
		err = fakeClient.List(context.TODO(), vmList)
		Expect(err).ToNot(HaveOccurred())

		expectFunc(vmList)
	},
		Entry("should replace all disks serial",
			func(kvvmList *virtv1.VirtualMachineList) {
				GinkgoHelper()
				Expect(kvvmList).NotTo(BeNil())
				Expect(kvvmList.Items).To(HaveLen(1))
				kvvm := kvvmList.Items[0]
				Expect(kvvm.Spec.Template).ToNot(BeNil())
				for _, d := range kvvm.Spec.Template.Spec.Domain.Devices.Disks {
					Expect(len(d.Serial)).To(BeNumerically("<=", 36))
					switch d.Name {
					case vdQemu36DiskName:
						Expect(d.Serial).To(Equal(vdQemu36UIDMD5))
					case viQemu36DiskName:
						Expect(d.Serial).To(Equal(viQemu36MD5))
					case cviQemu36DiskName:
						Expect(d.Serial).To(Equal(cviQemu36UIDMD5))
					default:
						Fail("unknown disk name")
					}
				}
			},
			kvvmQemu36,
			vdQemu36,
			viQemu36,
			cviQemu36,
		))
})

const (
	namespaceQemu36 = "qemu-36"

	vdQemu36Name     = "vd-qemu-36"
	vdQemu36DiskName = "vd-vd-qemu-36"
	vdQemu36UID      = types.UID("vd-qemu-36-uid")
	vdQemu36UIDMD5   = "150668731cbbc66aa0364f4a9055e496"

	viQemu36Name     = "vi-qemu-36"
	viQemu36DiskName = "vi-vi-qemu-36"
	viQemu36UID      = types.UID("vi-qemu-36-uid")
	viQemu36MD5      = "35dc821cd41a4013d11c15fb9b15f2f0"

	cviQemu36Name     = "cvi-qemu-36"
	cviQemu36DiskName = "cvi-cvi-qemu-36"
	cviQemu36UID      = types.UID("cvi-qemu-36-uid")
	cviQemu36UIDMD5   = "ccc3c60cd0e60b9edfb67cf6e1c8af54"

	kvvmTest01Name = "vm-test-01"
)

var (
	vdQemu36 = &v1alpha2.VirtualDisk{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      vdQemu36Name,
			Namespace: namespaceQemu36,
			UID:       vdQemu36UID,
		},
	}
	viQemu36 = &v1alpha2.VirtualImage{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualImageKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      viQemu36Name,
			Namespace: namespaceQemu36,
			UID:       viQemu36UID,
		},
	}
	cviQemu36 = &v1alpha2.ClusterVirtualImage{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.ClusterVirtualImageKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cviQemu36Name,
			Namespace: namespaceQemu36,
			UID:       cviQemu36UID,
		},
	}

	kvvmQemu36 = &virtv1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: virtv1.GroupVersion.String(),
			Kind:       "VirtualMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kvvmTest01Name,
			Namespace: namespaceQemu36,
		},
		Spec: virtv1.VirtualMachineSpec{
			Template: &virtv1.VirtualMachineInstanceTemplateSpec{
				Spec: virtv1.VirtualMachineInstanceSpec{
					Domain: virtv1.DomainSpec{
						Devices: virtv1.Devices{
							Disks: []virtv1.Disk{
								generateDiskWithInvalidSerial(vdQemu36DiskName),
								generateDiskWithInvalidSerial(viQemu36DiskName),
								generateDiskWithInvalidSerial(cviQemu36DiskName),
							},
						},
					},
				},
			},
		},
	}
)

func generateDiskWithInvalidSerial(name string) virtv1.Disk {
	return virtv1.Disk{
		Name: name,
		DiskDevice: virtv1.DiskDevice{
			Disk: &virtv1.DiskTarget{
				Bus: virtv1.DiskBusSCSI,
			},
		},
		Serial: "should replace because the serial is invalid",
	}
}
