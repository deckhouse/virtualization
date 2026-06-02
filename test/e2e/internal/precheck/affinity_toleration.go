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

package precheck

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

const (
	affinityTolerationPrecheckEnvName = "AFFINITY_TOLERATION_PRECHECK"
	nodeGroupLabelKey                 = "node.deckhouse.io/group"
	kvmLabelKey                       = "virtualization.deckhouse.io/kvm-enabled"

	minReadyKVMMasterNodes = 1
	minReadyKVMWorkerNodes = 2
)

// affinityTolerationPrecheck implements Precheck interface for VM affinity/toleration test cluster requirements.
type affinityTolerationPrecheck struct{}

func (a *affinityTolerationPrecheck) Label() string {
	return PrecheckAffinityToleration
}

func (a *affinityTolerationPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if !isCheckEnabled(affinityTolerationPrecheckEnvName) {
		_, _ = GinkgoWriter.Write([]byte("Affinity/toleration precheck is disabled.\n"))
		return nil
	}

	masterNodes, err := listReadyNodesByLabels(ctx, f, map[string]string{
		kvmLabelKey:       "true",
		nodeGroupLabelKey: "master",
	})
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to list ready KVM-enabled master nodes: %w", affinityTolerationPrecheckEnvName, err)
	}
	if len(masterNodes) < minReadyKVMMasterNodes {
		return fmt.Errorf("%s=no to disable this precheck: at least %d ready KVM-enabled master node is required, got %d", affinityTolerationPrecheckEnvName, minReadyKVMMasterNodes, len(masterNodes))
	}

	workerNodes, err := listReadyNodesByLabels(ctx, f, map[string]string{
		kvmLabelKey:       "true",
		nodeGroupLabelKey: "worker",
	})
	if err != nil {
		return fmt.Errorf("%s=no to disable this precheck: failed to list ready KVM-enabled worker nodes: %w", affinityTolerationPrecheckEnvName, err)
	}
	if len(workerNodes) < minReadyKVMWorkerNodes {
		return fmt.Errorf("%s=no to disable this precheck: at least %d ready KVM-enabled worker nodes are required, got %d", affinityTolerationPrecheckEnvName, minReadyKVMWorkerNodes, len(workerNodes))
	}

	return nil
}

func listReadyNodesByLabels(ctx context.Context, f *framework.Framework, labels map[string]string) ([]corev1.Node, error) {
	nodes := &corev1.NodeList{}
	err := f.GenericClient().List(ctx, nodes, crclient.MatchingLabels(labels))
	if err != nil {
		return nil, err
	}

	readyNodes := make([]corev1.Node, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				readyNodes = append(readyNodes, node)
				break
			}
		}
	}

	return readyNodes, nil
}

func init() {
	RegisterPrecheck(&affinityTolerationPrecheck{}, false)
}
