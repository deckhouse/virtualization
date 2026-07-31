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

package step

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	commonvd "github.com/deckhouse/virtualization-controller/pkg/common/vd"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	servicestat "github.com/deckhouse/virtualization-controller/pkg/controller/service/stat"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type waitForDVCRImporterStatStub struct{}

func (s waitForDVCRImporterStatStub) CheckPod(_ *corev1.Pod) error {
	return servicestat.ErrNotScheduled
}

func (s waitForDVCRImporterStatStub) GetProgress(_ types.UID, _ *corev1.Pod, prevProgress string, _ ...servicestat.GetProgressOption) string {
	return prevProgress
}

func (s waitForDVCRImporterStatStub) GetDownloadSpeed(_ types.UID, _ *corev1.Pod) *v1alpha2.StatusSpeed {
	return nil
}

var _ = Describe("WaitForDVCRImporterStep", func() {
	const protectionFinalizer = "virtualization.deckhouse.io/vd-protection"

	newImporterPod := func(nodePlacement *provisioner.NodePlacement) *corev1.Pod {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "importer",
				Namespace:  "default",
				Finalizers: []string{protectionFinalizer},
			},
		}
		Expect(provisioner.KeepNodePlacementTolerations(nodePlacement, pod)).To(Succeed())
		return pod
	}

	takeStep := func(pod *corev1.Pod, objects []client.Object, cb *conditions.ConditionBuilder) client.Client {
		vd := newTestVD(nil, "vm")
		fakeClient := fake.NewClientBuilder().WithScheme(newStepScheme()).WithObjects(append(objects, pod)...).Build()

		importerService := service.NewImporterService(
			&dvcr.Settings{},
			fakeClient,
			"importer-image",
			corev1.ResourceRequirements{},
			string(corev1.PullIfNotPresent),
			"1",
			"vd-controller",
			service.NewProtectionService(fakeClient, protectionFinalizer),
		)

		result, err := NewWaitForDVCRImporterStep(pod, waitForDVCRImporterStatStub{}, importerService, fakeClient, cb).
			Take(context.Background(), vd)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())

		return fakeClient
	}

	It("keeps the unschedulable pod when tolerations have not changed", func() {
		objects := []client.Object{newTestVM(testVMToleration), newTestVMClass(testVMClassToleration)}

		nodePlacement := &provisioner.NodePlacement{
			Tolerations: []corev1.Toleration{testVMToleration, testVMClassToleration, testSystemToleration},
		}
		pod := newImporterPod(nodePlacement)

		cb := conditions.NewConditionBuilder(vdcondition.ReadyType)
		fakeClient := takeStep(pod, objects, cb)

		fetched := &corev1.Pod{}
		err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pod), fetched)
		Expect(err).ToNot(HaveOccurred())
		Expect(fetched.DeletionTimestamp).To(BeNil())
		Expect(cb.Condition().Reason).To(Equal(vdcondition.ProvisioningNotStarted.String()))
		Expect(cb.Condition().Message).ToNot(ContainSubstring("recreation"))
	})

	It("recreates the unschedulable pod and its supplements when vm tolerations have changed", func() {
		supgen := vdsupplements.NewGenerator(newTestVD(nil, "vm"))
		authSecretKey := supgen.DVCRAuthSecret()
		authSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: authSecretKey.Name, Namespace: authSecretKey.Namespace},
		}
		objects := []client.Object{newTestVM(testVMToleration), newTestVMClass(), authSecret}

		pod := newImporterPod(&provisioner.NodePlacement{Tolerations: []corev1.Toleration{testSystemToleration}})

		cb := conditions.NewConditionBuilder(vdcondition.ReadyType)
		fakeClient := takeStep(pod, objects, cb)

		err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pod), &corev1.Pod{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// The auth secret is owned by the deleted pod: it must go away with it,
		// otherwise garbage collection removes it from under the recreated pod.
		err = fakeClient.Get(context.Background(), authSecretKey, &corev1.Secret{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		Expect(cb.Condition().Message).To(ContainSubstring("recreation"))
	})

	It("does not recreate the pod created by GetDVCRNodePlacement round-trip", func() {
		objects := []client.Object{newTestVM(testVMToleration), newTestVMClass(testVMClassToleration)}
		vd := newTestVD(nil, "vm")

		roundTripClient := fake.NewClientBuilder().WithScheme(newStepScheme()).WithObjects(objects...).Build()
		nodePlacement, err := commonvd.GetDVCRNodePlacement(context.Background(), roundTripClient, vd)
		Expect(err).ToNot(HaveOccurred())
		pod := newImporterPod(nodePlacement)

		cb := conditions.NewConditionBuilder(vdcondition.ReadyType)
		fakeClient := takeStep(pod, objects, cb)

		fetched := &corev1.Pod{}
		err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(pod), fetched)
		Expect(err).ToNot(HaveOccurred())
		Expect(fetched.DeletionTimestamp).To(BeNil())
		Expect(cb.Condition().Message).ToNot(ContainSubstring("recreation"))
	})
})
