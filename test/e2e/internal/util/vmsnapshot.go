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
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmscondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

func UntilVMSnapshotReady(key client.ObjectKey, timeout time.Duration) {
	GinkgoHelper()

	Eventually(func() error {
		vmSnapshot := &v1alpha2.VirtualMachineSnapshot{}
		err := framework.GetClients().GenericClient().Get(context.Background(), key, vmSnapshot)
		if err != nil {
			return err
		}

		snapshotReady, _ := conditions.GetCondition(vmscondition.VirtualMachineSnapshotReadyType, vmSnapshot.Status.Conditions)
		if snapshotReady.Status == metav1.ConditionTrue {
			return nil
		}

		return fmt.Errorf("vm snapshot %s is not ready", key.Name)
	}).WithTimeout(timeout).WithPolling(time.Second).Should(Succeed())
}
