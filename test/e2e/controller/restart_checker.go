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

package controller

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

type ControllerRestartChecker struct {
	startedAt metav1.Time
}

func (c *ControllerRestartChecker) Check() error {
	kubeClient := framework.GetClients().KubeClient()
	pods, err := kubeClient.CoreV1().Pods(VirtualizationNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"app": VirtualizationController}).String(),
	})
	if err != nil {
		return err
	}

	var errs error
	for _, pod := range pods.Items {
		foundContainer := false
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.Name == VirtualizationController && containerStatus.State.Running != nil {
				foundContainer = true
				if containerStatus.State.Running.StartedAt.After(c.startedAt.Time) {
					errs = errors.Join(errs, fmt.Errorf("the container %q was restarted: %s", VirtualizationController, pod.Name))
				}
			}
		}
		if !foundContainer {
			errs = errors.Join(errs, fmt.Errorf("the container %q was not found: %s", VirtualizationController, pod.Name))
		}
	}

	return errs
}
