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

package handler

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

func TestVolumeMigrationHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VolumeMigration Handlers Suite")
}

func setupEnvironment(vd *v1alpha2.VirtualDisk, objs ...client.Object) client.Client {
	GinkgoHelper()
	Expect(vd).ToNot(BeNil())
	allObjects := []client.Object{vd}
	allObjects = append(allObjects, objs...)

	fakeClient, err := testutil.NewFakeClientWithObjects(allObjects...)
	Expect(err).NotTo(HaveOccurred())

	key := types.NamespacedName{
		Name:      vd.GetName(),
		Namespace: vd.GetNamespace(),
	}
	resource := reconciler.NewResource(key, fakeClient,
		func() *v1alpha2.VirtualDisk {
			return &v1alpha2.VirtualDisk{}
		},
		func(obj *v1alpha2.VirtualDisk) v1alpha2.VirtualDiskStatus {
			return obj.Status
		})
	err = resource.Fetch(context.Background())
	Expect(err).NotTo(HaveOccurred())

	return fakeClient
}

func newTestVD(name, namespace, vmName string, storageClassChanged, ready, migrating bool) *v1alpha2.VirtualDisk {
	vd := vdbuilder.NewEmpty(name, namespace)
	oldStorageClass := "old-storage-class"
	vd.Spec.PersistentVolumeClaim.StorageClass = &oldStorageClass

	vd.Status.AttachedToVirtualMachines = []v1alpha2.AttachedVirtualMachine{
		{
			Name:    vmName,
			Mounted: true,
		},
	}

	if storageClassChanged {
		vd.Status.StorageClassName = "new-storage-class"
	} else {
		vd.Status.StorageClassName = "old-storage-class"
	}

	if ready {
		vd.Status.Conditions = append(vd.Status.Conditions, metav1.Condition{
			Type:   vdcondition.ReadyType.String(),
			Status: metav1.ConditionTrue,
		})
	}
	if migrating {
		vd.Status.Conditions = append(vd.Status.Conditions, metav1.Condition{
			Type:   vdcondition.MigratingType.String(),
			Status: metav1.ConditionTrue,
		})
	}
	return vd
}

func newTestVMOP(name, namespace, vmName string, phase v1alpha2.VMOPPhase) *v1alpha2.VirtualMachineOperation {
	vmop := vmopbuilder.New(
		vmopbuilder.WithName(name),
		vmopbuilder.WithNamespace(namespace),
		vmopbuilder.WithAnnotation(annotations.AnnVMOPVolumeMigration, "true"),
		vmopbuilder.WithType(v1alpha2.VMOPTypeEvict),
		vmopbuilder.WithVirtualMachine(vmName),
	)
	vmop.Status.Phase = phase
	return vmop
}
