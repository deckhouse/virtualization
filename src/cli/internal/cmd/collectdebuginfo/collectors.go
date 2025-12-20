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

package collectdebuginfo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var coreKinds = map[string]bool{
	"Pod":                   true,
	"PersistentVolumeClaim": true,
	"PersistentVolume":      true,
	"Event":                 true,
}

// Resource collection functions

func (b *DebugBundle) collectVMResources(ctx context.Context, client kubeclient.Client, namespace, vmName string) error {
	// Get VM
	vm, err := client.VirtualMachines(namespace).Get(ctx, vmName, metav1.GetOptions{})
	if err != nil {
		if b.handleError("VirtualMachine", vmName, err) {
			return nil
		}
		return err
	}
	if err := b.outputResource("VirtualMachine", vmName, namespace, vm); err != nil {
		return fmt.Errorf("failed to output VirtualMachine: %w", err)
	}

	// Get IVVM
	ivvm, err := b.getInternalResource(ctx, "internalvirtualizationvirtualmachines", namespace, vmName)
	if err == nil {
		if err := b.outputResource("InternalVirtualizationVirtualMachine", vmName, namespace, ivvm); err != nil {
			return fmt.Errorf("failed to output InternalVirtualizationVirtualMachine: %w", err)
		}
	} else if !b.handleError("InternalVirtualizationVirtualMachine", vmName, err) {
		return err
	}

	// Get IVVMI
	ivvmi, err := b.getInternalResource(ctx, "internalvirtualizationvirtualmachineinstances", namespace, vmName)
	if err == nil {
		if err := b.outputResource("InternalVirtualizationVirtualMachineInstance", vmName, namespace, ivvmi); err != nil {
			return fmt.Errorf("failed to output InternalVirtualizationVirtualMachineInstance: %w", err)
		}
	} else if !b.handleError("InternalVirtualizationVirtualMachineInstance", vmName, err) {
		return err
	}

	// Get VM operations
	vmUID := string(vm.UID)
	vmops, err := client.VirtualMachineOperations(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("virtualization.deckhouse.io/virtual-machine-uid=%s", vmUID),
	})
	if err == nil {
		for _, vmop := range vmops.Items {
			if err := b.outputResource("VirtualMachineOperation", vmop.Name, namespace, &vmop); err != nil {
				return fmt.Errorf("failed to output VirtualMachineOperation: %w", err)
			}
		}
	} else if !b.handleError("VirtualMachineOperation", "", err) {
		return err
	}

	// Get migrations
	migrations, err := b.getInternalResourceList(ctx, "internalvirtualizationvirtualmachineinstancemigrations", namespace)
	if err == nil {
		for _, item := range migrations {
			vmiName, found, _ := unstructured.NestedString(item.Object, "spec", "vmiName")
			if found && vmiName == vmName {
				name, _, _ := unstructured.NestedString(item.Object, "metadata", "name")
				extraInfo := fmt.Sprintf(" (for VMI: %s)", vmiName)
				if err := b.outputResourceWithExtraInfo("InternalVirtualizationVirtualMachineInstanceMigration", name, namespace, item, extraInfo); err != nil {
					return fmt.Errorf("failed to output InternalVirtualizationVirtualMachineInstanceMigration: %w", err)
				}
			}
		}
	} else if !b.handleError("InternalVirtualizationVirtualMachineInstanceMigration", "", err) {
		return err
	}

	// Get events for VM
	b.collectEvents(ctx, client, namespace, "VirtualMachine", vmName)

	return nil
}

func (b *DebugBundle) collectBlockDevices(ctx context.Context, client kubeclient.Client, namespace, vmName string) error {
	vm, err := client.VirtualMachines(namespace).Get(ctx, vmName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Track collected block devices to avoid duplicates
	collectedBlockDevices := make(map[string]bool)

	// Helper function to create block device key
	bdKey := func(kind v1alpha2.BlockDeviceKind, name string) string {
		return string(kind) + ":" + name
	}

	// Static block devices
	// Note: blockDeviceRefs can only contain block devices (VirtualDisk, VirtualImage, ClusterVirtualImage),
	// not VMBDA. VMBDA are collected separately below.
	for _, bdRef := range vm.Spec.BlockDeviceRefs {
		key := bdKey(bdRef.Kind, bdRef.Name)
		if !collectedBlockDevices[key] {
			if err := b.collectBlockDevice(ctx, client, namespace, bdRef.Kind, bdRef.Name); err != nil {
				if !b.handleError(string(bdRef.Kind), bdRef.Name, err) {
					return err
				}
			} else {
				collectedBlockDevices[key] = true
			}
		}
	}

	// Get all VMBDA that reference this VM
	// Note: Hotplugged block devices are collected through VMBDA, not from vm.Status.BlockDeviceRefs,
	// to avoid duplication. All hotplugged devices have corresponding VMBDA resources.
	vmbdas, err := client.VirtualMachineBlockDeviceAttachments(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, vmbda := range vmbdas.Items {
			if vmbda.Spec.VirtualMachineName == vmName {
				if err := b.outputResource("VirtualMachineBlockDeviceAttachment", vmbda.Name, namespace, &vmbda); err != nil {
					return fmt.Errorf("failed to output VirtualMachineBlockDeviceAttachment: %w", err)
				}
				b.collectEvents(ctx, client, namespace, "VirtualMachineBlockDeviceAttachment", vmbda.Name)

				// Get associated block device
				if vmbda.Spec.BlockDeviceRef.Kind != "" && vmbda.Spec.BlockDeviceRef.Name != "" {
					// Convert VMBDAObjectRefKind to BlockDeviceKind
					var bdKind v1alpha2.BlockDeviceKind
					switch vmbda.Spec.BlockDeviceRef.Kind {
					case v1alpha2.VMBDAObjectRefKindVirtualDisk:
						bdKind = v1alpha2.VirtualDiskKind
					case v1alpha2.VMBDAObjectRefKindVirtualImage:
						bdKind = v1alpha2.VirtualImageKind
					case v1alpha2.VMBDAObjectRefKindClusterVirtualImage:
						bdKind = v1alpha2.ClusterVirtualImageKind
					default:
						continue
					}
					key := bdKey(bdKind, vmbda.Spec.BlockDeviceRef.Name)
					if !collectedBlockDevices[key] {
						if err := b.collectBlockDevice(ctx, client, namespace, bdKind, vmbda.Spec.BlockDeviceRef.Name); err != nil {
							if !b.handleError(string(bdKind), vmbda.Spec.BlockDeviceRef.Name, err) {
								return err
							}
						} else {
							collectedBlockDevices[key] = true
						}
					}
				}
			}
		}
	} else if !b.handleError("VirtualMachineBlockDeviceAttachment", "", err) {
		return err
	}

	return nil
}

func (b *DebugBundle) collectBlockDevice(ctx context.Context, client kubeclient.Client, namespace string, kind v1alpha2.BlockDeviceKind, name string) error {
	switch kind {
	case v1alpha2.VirtualDiskKind:
		vd, err := client.VirtualDisks(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if err := b.outputResource("VirtualDisk", name, namespace, vd); err != nil {
			return fmt.Errorf("failed to output VirtualDisk: %w", err)
		}
		b.collectEvents(ctx, client, namespace, "VirtualDisk", name)

		// Get PVC
		if vd.Status.Target.PersistentVolumeClaim != "" {
			pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, vd.Status.Target.PersistentVolumeClaim, metav1.GetOptions{})
			if err == nil {
				if err := b.outputResource("PersistentVolumeClaim", pvc.Name, namespace, pvc); err != nil {
					return fmt.Errorf("failed to output PersistentVolumeClaim: %w", err)
				}
				b.collectEvents(ctx, client, namespace, "PersistentVolumeClaim", pvc.Name)

				// Get PV
				if pvc.Spec.VolumeName != "" {
					pv, err := client.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
					if err == nil {
						if err := b.outputResource("PersistentVolume", pv.Name, "", pv); err != nil {
							return fmt.Errorf("failed to output PersistentVolume: %w", err)
						}
					} else if !b.handleError("PersistentVolume", pvc.Spec.VolumeName, err) {
						return err
					}
				}
			} else if !b.handleError("PersistentVolumeClaim", vd.Status.Target.PersistentVolumeClaim, err) {
				return err
			}
		}

	case v1alpha2.VirtualImageKind:
		vi, err := client.VirtualImages(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if err := b.outputResource("VirtualImage", name, namespace, vi); err != nil {
			return fmt.Errorf("failed to output VirtualImage: %w", err)
		}
		b.collectEvents(ctx, client, namespace, "VirtualImage", name)

	case v1alpha2.ClusterVirtualImageKind:
		cvi, err := client.ClusterVirtualImages().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if err := b.outputResource("ClusterVirtualImage", name, "", cvi); err != nil {
			return fmt.Errorf("failed to output ClusterVirtualImage: %w", err)
		}

	default:
		return fmt.Errorf("unknown block device kind: %s", kind)
	}

	return nil
}

func (b *DebugBundle) collectPods(ctx context.Context, client kubeclient.Client, namespace, vmName string) error {
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("vm.kubevirt.internal.virtualization.deckhouse.io/name=%s", vmName),
	})
	if err != nil {
		if b.handleError("Pod", "", err) {
			return nil
		}
		return err
	}

	// Collect VM pods and their UIDs for finding dependent pods
	vmPodUIDs := make(map[string]bool)
	for _, pod := range pods.Items {
		vmPodUIDs[string(pod.UID)] = true
		if err := b.outputResource("Pod", pod.Name, namespace, &pod); err != nil {
			return fmt.Errorf("failed to output Pod: %w", err)
		}
		b.collectEvents(ctx, client, namespace, "Pod", pod.Name)

		if b.saveLogs {
			b.collectSinglePodLogs(ctx, client, namespace, pod.Name)
		}
	}

	// Collect pods that have ownerReference to VM pods (e.g., hotplug volume pods)
	if len(vmPodUIDs) > 0 {
		allPods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			// If we can't list all pods, continue without dependent pods
			if !b.handleError("Pod", namespace, err) {
				return err
			}
		} else {
			for _, pod := range allPods.Items {
				// Skip VM pods we already collected
				if vmPodUIDs[string(pod.UID)] {
					continue
				}
				// Check if this pod has ownerReference to any VM pod
				for _, ownerRef := range pod.OwnerReferences {
					if ownerRef.Kind == "Pod" && vmPodUIDs[string(ownerRef.UID)] {
						if err := b.outputResource("Pod", pod.Name, namespace, &pod); err != nil {
							return fmt.Errorf("failed to output Pod: %w", err)
						}
						b.collectEvents(ctx, client, namespace, "Pod", pod.Name)
						if b.saveLogs {
							b.collectSinglePodLogs(ctx, client, namespace, pod.Name)
						}
						break
					}
				}
			}
		}
	}

	return nil
}

// Event collection functions

func (b *DebugBundle) collectEvents(ctx context.Context, client kubeclient.Client, namespace, resourceType, resourceName string) {
	events, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", resourceName),
	})
	if err != nil {
		if b.handleError("Event", resourceName, err) {
			return
		}
		return
	}

	// Add each event individually to preserve TypeMeta
	for i := range events.Items {
		if err := b.outputResource("Event", fmt.Sprintf("%s-%s-%d", strings.ToLower(resourceType), resourceName, i), namespace, &events.Items[i]); err != nil {
			// Log error but continue processing other events
			_, _ = fmt.Fprintf(b.stderr, "Warning: failed to output Event: %v\n", err)
		}
	}
}

// Log collection functions

const (
	// logReadTimeout is the maximum time to wait for reading pod logs
	logReadTimeout = 30 * time.Second
	// maxLogLines limits the number of log lines to prevent hanging on very large logs
	maxLogLines = int64(10000)
)

func (b *DebugBundle) collectSinglePodLogs(ctx context.Context, client kubeclient.Client, namespace, podName string) {
	logPrefix := fmt.Sprintf("logs %s/%s", namespace, podName)
	tailLines := maxLogLines

	// Get current logs with timeout and line limit
	logCtx, cancel := context.WithTimeout(ctx, logReadTimeout)
	defer cancel()

	req := client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		TailLines: &tailLines,
	})
	logStream, err := req.Stream(logCtx)
	if err == nil && logStream != nil {
		stream := logStream // Capture in closure
		defer func() {
			if stream != nil {
				_ = stream.Close()
			}
		}()
		logContent, err := b.readLogsWithTimeout(logCtx, logStream)
		if err == nil {
			_, _ = fmt.Fprintf(b.stdout, "\n# %s\n", logPrefix)
			_, _ = fmt.Fprintf(b.stdout, "%s\n", string(logContent))
		}
	}

	// Get previous logs with timeout and line limit
	logCtx, cancel = context.WithTimeout(ctx, logReadTimeout)
	defer cancel()

	req = client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Previous:  true,
		TailLines: &tailLines,
	})
	logStream, err = req.Stream(logCtx)
	if err == nil && logStream != nil {
		stream := logStream // Capture in closure
		defer func() {
			if stream != nil {
				_ = stream.Close()
			}
		}()
		logContent, err := b.readLogsWithTimeout(logCtx, logStream)
		if err == nil {
			_, _ = fmt.Fprintf(b.stdout, "\n# %s (previous)\n", logPrefix)
			_, _ = fmt.Fprintf(b.stdout, "%s\n", string(logContent))
		}
	}
}

// readLogsWithTimeout reads logs from stream with timeout protection
func (b *DebugBundle) readLogsWithTimeout(ctx context.Context, stream io.ReadCloser) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	resultChan := make(chan result, 1)

	go func() {
		data, err := io.ReadAll(stream)
		resultChan <- result{data: data, err: err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		return res.data, res.err
	}
}

// Helper functions

func (b *DebugBundle) getInternalResource(ctx context.Context, resource, namespace, name string) (*unstructured.Unstructured, error) {
	obj, err := b.dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "internal.virtualization.deckhouse.io",
		Version:  "v1",
		Resource: resource,
	}).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (b *DebugBundle) getInternalResourceList(ctx context.Context, resource, namespace string) ([]*unstructured.Unstructured, error) {
	list, err := b.dynamicClient.Resource(schema.GroupVersionResource{
		Group:    "internal.virtualization.deckhouse.io",
		Version:  "v1",
		Resource: resource,
	}).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	result := make([]*unstructured.Unstructured, len(list.Items))
	for i := range list.Items {
		result[i] = &list.Items[i]
	}
	return result, nil
}

func (b *DebugBundle) outputResource(kind, name, namespace string, obj runtime.Object) error {
	return b.outputResourceWithExtraInfo(kind, name, namespace, obj, "")
}

func (b *DebugBundle) outputResourceWithExtraInfo(kind, name, namespace string, obj runtime.Object, extraInfo string) error {
	// Output separator if not first resource
	if b.resourceCount > 0 {
		_, _ = fmt.Fprintf(b.stdout, "\n---\n")
	}
	b.resourceCount++

	// Ensure Kind is set from input if missing
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Kind == "" {
		gvk.Kind = kind
		obj.GetObjectKind().SetGroupVersionKind(gvk)
	}

	// If GroupVersion is missing/empty, try to get from scheme
	if gvk.GroupVersion().Empty() {
		gvks, _, err := kubeclient.Scheme.ObjectKinds(obj)
		if err == nil && len(gvks) > 0 {
			gvk = gvks[0]
			obj.GetObjectKind().SetGroupVersionKind(gvk)
		} else if coreKinds[kind] {
			// Fallback for core Kubernetes resources if scheme doesn't know about them
			gvk = schema.GroupVersionKind{Group: "", Version: "v1", Kind: kind}
			obj.GetObjectKind().SetGroupVersionKind(gvk)
		}
	}

	// Marshal to JSON (now with TypeMeta if set)
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal %s/%s (namespace: %s) to JSON: %w", kind, name, namespace, err)
	}

	// Convert to YAML
	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return fmt.Errorf("failed to convert %s/%s (namespace: %s) to YAML: %w", kind, name, namespace, err)
	}

	// Output with optional extra info
	_, _ = fmt.Fprintf(b.stdout, "# %d. %s: %s%s\n%s", b.resourceCount, kind, name, extraInfo, string(yamlBytes))

	return nil
}
