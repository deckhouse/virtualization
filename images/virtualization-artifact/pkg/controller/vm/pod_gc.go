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

package vm

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/config"
	"github.com/deckhouse/virtualization-controller/pkg/controller/gc"
)

const (
	gcPodControllerName  = "vm-pod-gc-controller"
	defaultPodGCTTL      = 24 * time.Hour
	defaultPodGCMaxCount = 2
)

func SetupPodGC(mgr manager.Manager, log *log.Logger, gcSettings config.BaseGcSettings) error {
	podGCMgr := newPodGCManager(mgr.GetClient(), gcSettings.TTL.Duration, defaultPodGCMaxCount)

	return gc.SetupGcController(gcPodControllerName,
		mgr,
		log.With("resource", "vm-pod"),
		gcSettings.Schedule,
		podGCMgr,
	)
}

func newPodGCManager(client client.Client, ttl time.Duration, max int) *podGCManager {
	if ttl == 0 {
		ttl = defaultPodGCTTL
	}
	if max == 0 {
		max = defaultPodGCMaxCount
	}
	return &podGCManager{
		client: client,
		ttl:    ttl,
		max:    max,
	}
}

var _ gc.ReconcileGCManager = &podGCManager{}

type podGCManager struct {
	client client.Client
	ttl    time.Duration
	max    int
}

func (m *podGCManager) New() client.Object {
	return &corev1.Pod{}
}

func (m *podGCManager) ShouldBeDeleted(obj client.Object) bool {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return false
	}
	if _, hasLabel := pod.Labels[virtv1.VirtualMachineNameLabel]; !hasLabel {
		return false
	}
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

func (m *podGCManager) ListForDelete(ctx context.Context, now time.Time) ([]client.Object, error) {
	podList := &corev1.PodList{}
	err := m.client.List(ctx, podList, client.HasLabels{virtv1.VirtualMachineNameLabel})
	if err != nil {
		return nil, err
	}

	objs := make([]client.Object, 0, len(podList.Items))
	for _, pod := range podList.Items {
		objs = append(objs, &pod)
	}

	return gc.DefaultFilter(objs, m.ShouldBeDeleted, m.ttl, m.getIndex, m.max, now), nil
}

func (m *podGCManager) getIndex(obj client.Object) string {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return ""
	}
	return pod.Namespace + "/" + pod.Labels[virtv1.VirtualMachineNameLabel]
}
