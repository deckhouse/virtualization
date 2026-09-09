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

package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
)

// The live collector runs for every spec, not only failed ones: the failure
// dump in dump.go reads resources after the fact, so anything the cluster has
// already deleted by then - most importantly the virt-launcher pod of a guest
// that died mid-migration and was instantly restarted - leaves no trace. The
// collector instead follows the logs of every virt-launcher pod of the spec's
// namespace while the pod is alive and journals every status transition of the
// namespace's internal VMIs, so the evidence survives the deletion.
//
// Files land under <base>/<namespace>/ on the machine running the suite:
//
//	pod_<pod>__<container>.log  - followed container logs (kubelet timestamps)
//	kvvmi_<name>.jsonl          - one JSON line per observed KVVMI transition
const (
	// liveCollectDirEnv overrides the base directory of the live collector.
	liveCollectDirEnv = "E2E_LIVE_COLLECT_DIR"
	// defaultLiveCollectDir is where the collector writes when the env is unset.
	defaultLiveCollectDir = "/tmp/e2e_live"

	// virtLauncherLabelSelector selects the virt-launcher pods of a namespace.
	virtLauncherLabelSelector = "kubevirt.internal.virtualization.deckhouse.io=virt-launcher"

	// liveWatchWindow caps a single watch pass; the loops restart the watch
	// until the spec's cleanup cancels the collector.
	liveWatchWindow = time.Hour

	// liveRetryDelay spaces out retries after a broken watch or log stream.
	liveRetryDelay = 2 * time.Second
)

// liveKVVMIGVR is the GroupVersionResource of the internal (KubeVirt) VMI.
var liveKVVMIGVR = schema.GroupVersionResource{
	Group:    "internal.virtualization.deckhouse.io",
	Version:  "v1",
	Resource: "internalvirtualizationvirtualmachineinstances",
}

// startLiveCollect launches the always-on collectors for the spec's namespace.
// They run until the spec's cleanup cancels them via DeferCleanup.
func (f *Framework) startLiveCollect() {
	if f.namespace == nil {
		return
	}

	dir := filepath.Join(liveCollectBaseDir(), f.namespace.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		GinkgoWriter.Printf("Live collect disabled: failed to create %s: %v\n", dir, err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	c := &liveCollector{
		clients:   f.Clients,
		namespace: f.namespace.Name,
		dir:       dir,
	}
	go c.collectLauncherLogs(ctx)
	go c.collectKVVMITransitions(ctx)
}

func liveCollectBaseDir() string {
	if dir := os.Getenv(liveCollectDirEnv); dir != "" {
		return dir
	}
	return defaultLiveCollectDir
}

type liveCollector struct {
	clients   Clients
	namespace string
	dir       string

	// streaming tracks the pod containers whose logs are being followed, so a
	// pod re-observed by a restarted watch does not get a second follower.
	mu        sync.Mutex
	streaming map[string]bool
}

// collectLauncherLogs discovers the namespace's virt-launcher pods and starts
// one log follower per d8v container. Discovery combines a list (pods created
// before the watch started, or during a watch gap) with a watch (pods created
// after), and restarts until ctx is cancelled.
func (c *liveCollector) collectLauncherLogs(ctx context.Context) {
	defer GinkgoRecover()

	pods := c.clients.KubeClient().CoreV1().Pods(c.namespace)
	opts := metav1.ListOptions{
		LabelSelector:  virtLauncherLabelSelector,
		TimeoutSeconds: ptr.To(int64(liveWatchWindow / time.Second)),
	}

	for ctx.Err() == nil {
		list, err := pods.List(ctx, metav1.ListOptions{LabelSelector: virtLauncherLabelSelector})
		if err == nil {
			for i := range list.Items {
				c.followPod(ctx, &list.Items[i])
			}
		}

		w, err := pods.Watch(ctx, opts)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(liveRetryDelay):
			}
			continue
		}
		for event := range w.ResultChan() {
			if pod, ok := event.Object.(*corev1.Pod); ok {
				c.followPod(ctx, pod)
			}
		}
		// The window elapsed or the API server closed the watch; relist and
		// rewatch. The loop condition handles the spec's cleanup.
	}
}

// followPod starts a follower goroutine for every d8v container of the pod
// that is not being followed yet.
func (c *liveCollector) followPod(ctx context.Context, pod *corev1.Pod) {
	for _, container := range pod.Spec.Containers {
		if !strings.HasPrefix(container.Name, d8vContainerPrefix) {
			continue
		}
		key := string(pod.UID) + "/" + container.Name

		c.mu.Lock()
		if c.streaming == nil {
			c.streaming = map[string]bool{}
		}
		if c.streaming[key] {
			c.mu.Unlock()
			continue
		}
		c.streaming[key] = true
		c.mu.Unlock()

		go c.followContainerLogs(ctx, pod.Name, container.Name)
	}
}

// followContainerLogs streams the container's logs into the collector's
// directory until the pod is gone or ctx is cancelled. A broken stream (an API
// blip, a container restart) is reopened from the time of the last received
// chunk, so at most a boundary line is duplicated, none are lost.
func (c *liveCollector) followContainerLogs(ctx context.Context, podName, containerName string) {
	defer GinkgoRecover()

	fileName := filepath.Join(c.dir, fmt.Sprintf("pod_%s__%s.log", podName, containerName))
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		GinkgoWriter.Printf("Live collect: failed to open %s: %v\n", fileName, err)
		return
	}
	defer file.Close()

	var sinceTime *metav1.Time
	for ctx.Err() == nil {
		req := c.clients.KubeClient().CoreV1().Pods(c.namespace).GetLogs(podName, &corev1.PodLogOptions{
			Container:  containerName,
			Follow:     true,
			Timestamps: true,
			SinceTime:  sinceTime,
		})
		stream, err := req.Stream(ctx)
		if err != nil {
			if c.isPodGone(ctx, podName) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(liveRetryDelay):
			}
			continue
		}

		buf := make([]byte, 32*1024)
		for {
			n, err := stream.Read(buf)
			if n > 0 {
				sinceTime = ptr.To(metav1.Now())
				if _, err := file.Write(buf[:n]); err != nil {
					GinkgoWriter.Printf("Live collect: failed to write %s: %v\n", fileName, err)
					stream.Close()
					return
				}
			}
			if err != nil {
				break
			}
		}
		stream.Close()

		// The followed container terminated or the stream broke: without this
		// check a deleted pod would keep the retry loop alive for the rest of
		// the spec.
		if c.isPodGone(ctx, podName) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(liveRetryDelay):
		}
	}
}

func (c *liveCollector) isPodGone(ctx context.Context, podName string) bool {
	_, err := c.clients.KubeClient().CoreV1().Pods(c.namespace).Get(ctx, podName, metav1.GetOptions{})
	return k8serrors.IsNotFound(err)
}

// collectKVVMITransitions journals every observed status transition of the
// namespace's internal VMIs, one JSON line per watch event, until ctx is
// cancelled.
func (c *liveCollector) collectKVVMITransitions(ctx context.Context) {
	defer GinkgoRecover()

	kvvmis := c.clients.DynamicClient().Resource(liveKVVMIGVR).Namespace(c.namespace)
	opts := metav1.ListOptions{TimeoutSeconds: ptr.To(int64(liveWatchWindow / time.Second))}

	for ctx.Err() == nil {
		w, err := kvvmis.Watch(ctx, opts)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(liveRetryDelay):
			}
			continue
		}
		for event := range w.ResultChan() {
			kvvmi, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}
			c.writeKVVMITransition(string(event.Type), kvvmi)
		}
	}
}

// writeKVVMITransition appends the event as a single JSON line to the VMI's
// journal. The full status goes in: the point of the journal is diagnosing
// failures whose shape is unknown in advance.
func (c *liveCollector) writeKVVMITransition(eventType string, kvvmi *unstructured.Unstructured) {
	status, _, _ := unstructured.NestedMap(kvvmi.Object, "status")
	line, err := json.Marshal(map[string]any{
		"time":              time.Now().Format(time.RFC3339Nano),
		"event":             eventType,
		"resourceVersion":   kvvmi.GetResourceVersion(),
		"deletionTimestamp": kvvmi.GetDeletionTimestamp(),
		"status":            status,
	})
	if err != nil {
		GinkgoWriter.Printf("Live collect: failed to marshal KVVMI %s transition: %v\n", kvvmi.GetName(), err)
		return
	}

	fileName := filepath.Join(c.dir, fmt.Sprintf("kvvmi_%s.jsonl", kvvmi.GetName()))
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		GinkgoWriter.Printf("Live collect: failed to open %s: %v\n", fileName, err)
		return
	}
	defer file.Close()

	if _, err := file.Write(append(line, '\n')); err != nil {
		GinkgoWriter.Printf("Live collect: failed to write %s: %v\n", fileName, err)
	}
}
