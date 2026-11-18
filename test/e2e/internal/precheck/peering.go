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
	"encoding/json"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	peertest "github.com/deckhouse/virtualization/test/e2e/internal/peering"
)

type peeringPrecheck struct{}

func (*peeringPrecheck) Label() string { return PrecheckPeering }

func (*peeringPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	if err := checkPeeringDevBuild(ctx, f.KubeClient(), func(ctx context.Context, pod corev1.Pod) ([]byte, error) {
		result := f.Clients.Kubectl().RawCommandContext(ctx,
			fmt.Sprintf("exec --namespace %s %s --container virtualization-controller -- /app/virtualization-controller --print-build-info", pod.Namespace, pod.Name),
			framework.ShortTimeout)
		if err := result.Error(); err != nil {
			return nil, fmt.Errorf("%w; stderr: %s", err, result.StdErr())
		}

		return result.StdOutBytes(), nil
	}); err != nil {
		return err
	}

	if !isCheckEnabled(peeringCheckEnvName) {
		return nil
	}

	deployment, err := f.KubeClient().AppsV1().Deployments(framework.ModuleNamespace).Get(ctx, "virt-synchronization-controller", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("peering migration requires a commercial virtualization build with a synchronization controller: %w", err)
	}

	if deployment.Status.AvailableReplicas == 0 {
		return fmt.Errorf("wait for deployment %s/%s to become available before testing peering migration", deployment.Namespace, deployment.Name)
	}

	return nil
}

// checkPeeringDevBuild checks the actual binaries in every active controller pod,
// including old replicas during a rollout. An image tag does not prove its build tags.
func checkPeeringDevBuild(ctx context.Context, kube kubernetes.Interface, readBuildInfo func(context.Context, corev1.Pod) ([]byte, error)) error {
	pods, err := kube.CoreV1().Pods(framework.ModuleNamespace).List(ctx, metav1.ListOptions{LabelSelector: "app=virtualization-controller"})
	if err != nil {
		return fmt.Errorf("cannot verify the controller DEV build: %w", err)
	}

	checked := 0

	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}

		if !slices.ContainsFunc(pod.Status.Conditions, func(c corev1.PodCondition) bool {
			return c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue
		}) {
			return fmt.Errorf("wait for controller pod %s/%s to become ready before testing self-peer migration", pod.Namespace, pod.Name)
		}

		data, err := readBuildInfo(ctx, pod)
		if err != nil {
			return fmt.Errorf("cannot verify the build of controller pod %s/%s; deploy a controller built with DEV_BUILD=true: %w", pod.Namespace, pod.Name, err)
		}

		var info struct {
			DevBuild *bool `json:"devBuild"`
		}
		if err := json.Unmarshal(data, &info); err != nil {
			return fmt.Errorf("invalid build information from controller pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}

		if info.DevBuild == nil || !*info.DevBuild {
			return fmt.Errorf("self-peer migration requires a controller built with DEV_BUILD=true; pod %s/%s does not report a DEV build", pod.Namespace, pod.Name)
		}

		checked++
	}

	if checked == 0 {
		return fmt.Errorf("self-peer migration requires ready virtualization-controller pods in namespace %s; deploy a controller built with DEV_BUILD=true", framework.ModuleNamespace)
	}

	return nil
}

// Recovery tests restart the synchronization controller shared by all namespaces.
type peeringRecoveryPrecheck struct{}

func (*peeringRecoveryPrecheck) Label() string { return PrecheckPeeringRecovery }

func (*peeringRecoveryPrecheck) Run(ctx context.Context, f *framework.Framework) error {
	return peertest.CheckMigrationIsolation(ctx, f.RewriteClient(), "")
}

func init() {
	RegisterPrecheck(&peeringRecoveryPrecheck{}, false)
}
