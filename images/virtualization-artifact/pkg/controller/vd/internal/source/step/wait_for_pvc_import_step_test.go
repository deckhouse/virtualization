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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	servicestat "github.com/deckhouse/virtualization-controller/pkg/controller/service/stat"
	vdsupplements "github.com/deckhouse/virtualization-controller/pkg/controller/vd/internal/supplements"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

var _ = Describe("WaitForPVCImportStep", func() {
	// The importer pod runs with RestartPolicy: OnFailure, so a pod that keeps failing is
	// Running or Waiting with its last exit recorded in LastTerminationState. Whether the
	// disk is declared failed must follow the reported reason, not the restart itself.
	newCrashingImporterPod := func(vd *v1alpha2.VirtualDisk, terminationMessage string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vdsupplements.NewGenerator(vd).PVCImporterPod().Name,
				Namespace: vd.Namespace,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{Message: terminationMessage},
						},
					},
				},
			},
		}
	}

	takeStepWithStat := func(pod *corev1.Pod, vd *v1alpha2.VirtualDisk, stat WaitForPVCImportStepStatService) *v1alpha2.VirtualDisk {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "vd-pvc", Namespace: vd.Namespace},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		}
		objects := []client.Object{pvc}
		if pod != nil {
			objects = append(objects, pod)
		}
		fakeClient := fake.NewClientBuilder().WithScheme(newStepScheme()).WithObjects(objects...).Build()

		cb := conditions.NewConditionBuilder(vdcondition.ReadyType)
		_, err := NewWaitForPVCImportStep(pvc, nil, nil, stat, nil, fakeClient, cb).
			Take(context.Background(), vd)
		Expect(err).ToNot(HaveOccurred())
		// The data source commits the condition the step filled in; do the same so a spec can
		// assert on what the user is told.
		conditions.SetCondition(cb, &vd.Status.Conditions)
		return vd
	}

	takeStepWithPod := func(pod *corev1.Pod, vd *v1alpha2.VirtualDisk) *v1alpha2.VirtualDisk {
		return takeStepWithStat(pod, vd, nil)
	}

	takeStep := func(terminationMessage string) *v1alpha2.VirtualDisk {
		vd := newTestVD(nil)
		vd.Status.Target.PersistentVolumeClaim = "vd-pvc"
		return takeStepWithPod(newCrashingImporterPod(vd, terminationMessage), vd)
	}

	It("fails the disk when the importer reports a permanent failure", func() {
		vd := takeStep(`{"error-message":"Unable to process data: virtual image size 43631247360 is larger than the reported available storage 10081009664. A larger PVC is required","permanent":true}`)
		Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskFailed))
	})

	It("keeps provisioning while the failure may still go away", func() {
		// A registry hiccup or DVCR restarted mid-import: the kubelet restarts the pod and
		// the import carries on, so the disk must not be painted Failed.
		vd := takeStep(`{"error-message":"Unable to process data: failed to pull image: connection refused"}`)
		Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
	})

	It("waits instead of failing while a new attempt is running", func() {
		// The share was extended between two restarts: the previous attempt failed for good,
		// but the one in flight may finish, so the disk must not be declared failed yet.
		vd := newTestVD(nil)
		vd.Status.Target.PersistentVolumeClaim = "vd-pvc"
		pod := newCrashingImporterPod(vd, `{"error-message":"Unable to process data: no space","permanent":true}`)
		pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}

		Expect(takeStepWithPod(pod, vd).Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
	})

	It("keeps provisioning when the importer pod is not there yet", func() {
		// The step reads the pod once and hands it to every branch: none of them may assume
		// there is one. The pod appears a moment after the PVC, and it is gone again once the
		// import is over.
		vd := newTestVD(nil)
		vd.Status.Target.PersistentVolumeClaim = "vd-pvc"

		Expect(takeStepWithPod(nil, vd).Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
	})

	It("tells the user why a failed importer pod failed", func() {
		vd := newTestVD(nil)
		vd.Status.Target.PersistentVolumeClaim = "vd-pvc"
		pod := newCrashingImporterPod(vd, "")
		pod.Status.Phase = corev1.PodFailed
		pod.Status.ContainerStatuses[0].State = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				Message: `{"error-message":"Unable to process data: no space left on device"}`,
			},
		}

		Expect(takeStepWithPod(pod, vd).Status.Phase).To(Equal(v1alpha2.DiskFailed))
		ready, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		Expect(ready.Message).To(Equal("Unable to process data: no space left on device"))
	})

	It("republishes the progress of the pod it read", func() {
		// The pod is fetched once and handed to every branch; the progress must still be
		// taken from that same pod.
		vd := newTestVD(nil)
		vd.Status.Target.PersistentVolumeClaim = "vd-pvc"
		pod := newCrashingImporterPod(vd, "")
		pod.Status.ContainerStatuses[0] = corev1.ContainerStatus{State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}}
		stat := &fakeProgressStat{progress: "42%"}

		vd = takeStepWithStat(pod, vd, stat)

		Expect(vd.Status.Phase).To(Equal(v1alpha2.DiskProvisioning))
		Expect(vd.Status.Progress).To(Equal("42%"))
		Expect(stat.seenPod).To(Equal(pod.Name))
	})
})

// fakeProgressStat stands in for the stat service and records the pod it was asked about.
type fakeProgressStat struct {
	progress string
	seenPod  string
}

func (f *fakeProgressStat) GetProgress(_ types.UID, pod *corev1.Pod, _ string, _ ...servicestat.GetProgressOption) string {
	if pod != nil {
		f.seenPod = pod.Name
	}
	return f.progress
}
