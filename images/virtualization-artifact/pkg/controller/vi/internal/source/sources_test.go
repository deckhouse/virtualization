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

package source

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	importer2 "github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
)

func TestSources(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sources")
}

func ExpectCondition(vi *virtv2.VirtualImage, status metav1.ConditionStatus, reason vicondition.ReadyReason, msgExists bool) {
	ready, _ := conditions.GetCondition(vicondition.Ready, vi.Status.Conditions)
	Expect(ready.Status).To(Equal(status))
	Expect(ready.Reason).To(Equal(reason.String()))
	Expect(ready.ObservedGeneration).To(Equal(vi.Generation))

	if msgExists {
		Expect(ready.Message).ToNot(BeEmpty())
	} else {
		Expect(ready.Message).To(BeEmpty())
	}
}

func getVirtualDisk() *virtv2.VirtualDisk {
	return &virtv2.VirtualDisk{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "vd",
			Generation: 1,
			UID:        "33333333-3333-3333-3333-333333333333",
		},
		Status: virtv2.VirtualDiskStatus{
			Capacity: "100Mi",
		},
	}
}

func getVirtualImage(storage virtv2.StorageType, ds virtv2.VirtualImageDataSource) *virtv2.VirtualImage {
	return &virtv2.VirtualImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "vi",
			Generation: 1,
			UID:        "22222222-2222-2222-2222-222222222222",
		},
		Spec: virtv2.VirtualImageSpec{
			Storage:    storage,
			DataSource: ds,
		},
	}
}

func getPod(vi *virtv2.VirtualImage) *corev1.Pod {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: supgen.ImporterPod().Name,
		},
	}
}

func getPVC(vi *virtv2.VirtualImage, scName string) *corev1.PersistentVolumeClaim {
	supgen := supplements.NewGenerator(annotations.VIShortName, vi.Name, vi.Namespace, vi.UID)

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: supgen.PersistentVolumeClaim().Name,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To(scName),
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: corev1.ClaimBound,
		},
	}
}

func getServiceMocks() (importer *ImporterMock, disk *DiskMock, stat *StatMock, recorder *eventrecord.EventRecorderLoggerMock, bounder *BounderMock) {
	importer = &ImporterMock{
		CleanUpSupplementsFunc: func(_ context.Context, _ *supplements.Generator) (bool, error) {
			return false, nil
		},
		GetPodSettingsWithPVCFunc: func(_ *metav1.OwnerReference, _ *supplements.Generator, _, _ string) *importer2.PodSettings {
			return nil
		},
	}

	disk = &DiskMock{
		CleanUpSupplementsFunc: func(_ context.Context, _ *supplements.Generator) (bool, error) {
			return false, nil
		},
		GetStorageClassFunc: func(_ context.Context, _ string) (*storagev1.StorageClass, error) {
			return &storagev1.StorageClass{}, nil
		},
	}

	stat = &StatMock{
		GetDVCRImageNameFunc: func(_ *corev1.Pod) string {
			return "image"
		},
		CheckPodFunc: func(_ *corev1.Pod) error {
			return nil
		},
		GetSizeFunc: func(_ *corev1.Pod) virtv2.ImageStatusSize {
			return virtv2.ImageStatusSize{}
		},
		GetCDROMFunc: func(_ *corev1.Pod) bool {
			return false
		},
		GetFormatFunc: func(_ *corev1.Pod) string {
			return "iso"
		},
		GetProgressFunc: func(_ types.UID, _ *corev1.Pod, _ string, _ ...service.GetProgressOption) string {
			return "N%"
		},
	}

	recorder = &eventrecord.EventRecorderLoggerMock{
		EventFunc: func(_ client.Object, _, _, _ string) {},
	}

	bounder = &BounderMock{
		CleanUpSupplementsFunc: func(_ context.Context, _ *supplements.Generator) (bool, error) {
			return false, nil
		},
	}

	return
}
