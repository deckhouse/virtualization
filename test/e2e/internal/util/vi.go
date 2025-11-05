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

package util

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/cvicondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdscondition"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vicondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

func UntilVIReady(key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func() error {
		vi, err := framework.GetClients().VirtClient().VirtualImages(key.Namespace).Get(context.Background(), key.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		if vi.Status.Phase != v1alpha2.ImageReady {
			return fmt.Errorf("virtual image %s is not ready, phase: %s", key.Name, vi.Status.Phase)
		}

		readyCondition, _ := conditions.GetCondition(vicondition.ReadyType, vi.Status.Conditions)
		if readyCondition.Status != metav1.ConditionTrue {
			return fmt.Errorf("virtual image %s ready condition is not true, status: %s", key.Name, readyCondition.Status)
		}

		return nil
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func UntilCVIReady(name string, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func() error {
		cvi, err := framework.GetClients().VirtClient().ClusterVirtualImages().Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		if cvi.Status.Phase != v1alpha2.ImageReady {
			return fmt.Errorf("cluster virtual image %s is not ready, phase: %s", name, cvi.Status.Phase)
		}

		readyCondition, _ := conditions.GetCondition(cvicondition.ReadyType, cvi.Status.Conditions)
		if readyCondition.Status != metav1.ConditionTrue {
			return fmt.Errorf("cluster virtual image %s ready condition is not true, status: %s", name, readyCondition.Status)
		}

		return nil
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func UntilVDReady(key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func() error {
		vd, err := framework.GetClients().VirtClient().VirtualDisks(key.Namespace).Get(context.Background(), key.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		if vd.Status.Phase != v1alpha2.DiskReady {
			return fmt.Errorf("virtual disk %s is not ready, phase: %s", key.Name, vd.Status.Phase)
		}

		readyCondition, _ := conditions.GetCondition(vdcondition.ReadyType, vd.Status.Conditions)
		if readyCondition.Status != metav1.ConditionTrue {
			return fmt.Errorf("virtual disk %s ready condition is not true, status: %s", key.Name, readyCondition.Status)
		}

		return nil
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}

func UntilVDSnapshotReady(key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func() error {
		vds := &v1alpha2.VirtualDiskSnapshot{}
		err := framework.GetClients().GenericClient().Get(context.Background(), key, vds)
		if err != nil {
			return err
		}

		if vds.Status.Phase != v1alpha2.VirtualDiskSnapshotPhaseReady {
			return fmt.Errorf("virtual disk snapshot %s is not ready, phase: %s", key.Name, vds.Status.Phase)
		}

		readyCondition, _ := conditions.GetCondition(vdscondition.VirtualDiskSnapshotReadyType, vds.Status.Conditions)
		if readyCondition.Status != metav1.ConditionTrue {
			return fmt.Errorf("virtual disk snapshot %s ready condition is not true, status: %s", key.Name, readyCondition.Status)
		}

		return nil
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}
