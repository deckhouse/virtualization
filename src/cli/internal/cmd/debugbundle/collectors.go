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

package debugbundle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	internalAPIGroup   = "internal.virtualization.deckhouse.io"
	internalAPIVersion = "v1"
	systemNamespace    = "d8-virtualization"
	coreAPIVersion     = "v1"
)

func (b *DebugBundle) collectVMResources(ctx context.Context, client kubeclient.Client, namespace, vmName string) error {
	// Get VM
	vm, err := client.VirtualMachines(namespace).Get(ctx, vmName, metav1.GetOptions{})
	if err != nil {
		if b.handleError("VirtualMachine", vmName, err) {
			return nil
		}
		return err
	}
	b.outputResource("VirtualMachine", vmName, namespace, vm)

	// Get IVVM
	ivvm, err := b.getInternalResource(ctx, "internalvirtualizationvirtualmachines", namespace, vmName)
	if err == nil {
		b.outputResource("InternalVirtualizationVirtualMachine", vmName, namespace, ivvm)
	} else if !b.handleError("InternalVirtualizationVirtualMachine", vmName, err) {
		return err
	}

	// Get IVVMI
	ivvmi, err := b.getInternalResource(ctx, "internalvirtualizationvirtualmachineinstances", namespace, vmName)
	if err == nil {
		b.outputResource("InternalVirtualizationVirtualMachineInstance", vmName, namespace, ivvmi)
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
			b.outputResource("VirtualMachineOperation", vmop.Name, namespace, &vmop)
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
				b.outputResource("InternalVirtualizationVirtualMachineInstanceMigration", name, namespace, item)
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

	// Static block devices
	for _, bdRef := range vm.Spec.BlockDeviceRefs {
		if err := b.collectBlockDevice(ctx, client, namespace, bdRef.Kind, bdRef.Name); err != nil {
			if !b.handleError(string(bdRef.Kind), bdRef.Name, err) {
				return err
			}
		}
	}

	// Hotplug block devices
	for _, bdRef := range vm.Status.BlockDeviceRefs {
		if bdRef.Hotplugged {
			if err := b.collectBlockDevice(ctx, client, namespace, bdRef.Kind, bdRef.Name); err != nil {
				if !b.handleError(string(bdRef.Kind), bdRef.Name, err) {
					return err
				}
			}

			// Get VMBDA
			if bdRef.VirtualMachineBlockDeviceAttachmentName != "" {
				vmbda, err := client.VirtualMachineBlockDeviceAttachments(namespace).Get(ctx, bdRef.VirtualMachineBlockDeviceAttachmentName, metav1.GetOptions{})
				if err == nil {
					b.outputResource("VirtualMachineBlockDeviceAttachment", vmbda.Name, namespace, vmbda)
					b.collectEvents(ctx, client, namespace, "VirtualMachineBlockDeviceAttachment", vmbda.Name)
				} else if !b.handleError("VirtualMachineBlockDeviceAttachment", bdRef.VirtualMachineBlockDeviceAttachmentName, err) {
					return err
				}
			}
		}
	}

	// Get all VMBDA that reference this VM
	vmbdas, err := client.VirtualMachineBlockDeviceAttachments(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, vmbda := range vmbdas.Items {
			if vmbda.Spec.VirtualMachineName == vmName {
				b.outputResource("VirtualMachineBlockDeviceAttachment", vmbda.Name, namespace, &vmbda)
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
					if err := b.collectBlockDevice(ctx, client, namespace, bdKind, vmbda.Spec.BlockDeviceRef.Name); err != nil {
						if !b.handleError(string(bdKind), vmbda.Spec.BlockDeviceRef.Name, err) {
							return err
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
		b.outputResource("VirtualDisk", name, namespace, vd)
		b.collectEvents(ctx, client, namespace, "VirtualDisk", name)

		// Get PVC
		if vd.Status.Target.PersistentVolumeClaim != "" {
			pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, vd.Status.Target.PersistentVolumeClaim, metav1.GetOptions{})
			if err == nil {
				b.outputResource("PersistentVolumeClaim", pvc.Name, namespace, pvc)
				b.collectEvents(ctx, client, namespace, "PersistentVolumeClaim", pvc.Name)

				// Get PV
				if pvc.Spec.VolumeName != "" {
					pv, err := client.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
					if err == nil {
						b.outputResource("PersistentVolume", pv.Name, "", pv)
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
		b.outputResource("VirtualImage", name, namespace, vi)
		b.collectEvents(ctx, client, namespace, "VirtualImage", name)

	case v1alpha2.ClusterVirtualImageKind:
		cvi, err := client.ClusterVirtualImages().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		b.outputResource("ClusterVirtualImage", name, "", cvi)
		// ClusterVirtualImage doesn't have events in namespace

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

	for _, pod := range pods.Items {
		b.outputResource("Pod", pod.Name, namespace, &pod)
		b.collectEvents(ctx, client, namespace, "Pod", pod.Name)

		if b.saveLogs {
			b.collectSinglePodLogs(ctx, client, namespace, pod.Name, false)
		}
	}

	return nil
}

func (b *DebugBundle) collectSystemLogs(ctx context.Context, client kubeclient.Client) error {
	pods, err := client.CoreV1().Pods(systemNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		if b.handleError("Pod", systemNamespace, err) {
			return nil
		}
		return err
	}

	for _, pod := range pods.Items {
		b.collectSinglePodLogs(ctx, client, systemNamespace, pod.Name, true)
	}
	return nil
}

func (b *DebugBundle) collectSinglePodLogs(ctx context.Context, client kubeclient.Client, namespace, podName string, isSystem bool) {
	logPrefix := fmt.Sprintf("%s/%s", namespace, podName)
	if isSystem {
		logPrefix = fmt.Sprintf("system/%s/%s", namespace, podName)
	}

	// Get current logs
	req := client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{})
	if logStream, err := req.Stream(ctx); err == nil {
		if logContent, err := io.ReadAll(logStream); err == nil {
			fmt.Fprintf(b.stderr, "\n# %s\n", logPrefix)
			fmt.Fprintf(b.stderr, "%s\n", string(logContent))
		}
		logStream.Close()
	}

	// Get previous logs
	req = client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{Previous: true})
	if logStream, err := req.Stream(ctx); err == nil {
		if logContent, err := io.ReadAll(logStream); err == nil {
			fmt.Fprintf(b.stderr, "\n# %s (previous)\n", logPrefix)
			fmt.Fprintf(b.stderr, "%s\n", string(logContent))
		}
		logStream.Close()
	}
}

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
		b.outputResource("Event", fmt.Sprintf("%s-%s-%d", strings.ToLower(resourceType), resourceName, i), namespace, &events.Items[i])
	}
}

func (b *DebugBundle) getInternalGVR(resource string) schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    internalAPIGroup,
		Version:  internalAPIVersion,
		Resource: resource,
	}
}

func (b *DebugBundle) getInternalResource(ctx context.Context, resource, namespace, name string) (*unstructured.Unstructured, error) {
	obj, err := b.dynamicClient.Resource(b.getInternalGVR(resource)).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (b *DebugBundle) getInternalResourceList(ctx context.Context, resource, namespace string) ([]*unstructured.Unstructured, error) {
	list, err := b.dynamicClient.Resource(b.getInternalGVR(resource)).Namespace(namespace).List(ctx, metav1.ListOptions{})
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
	// Check if object is unstructured and prepare it
	unstructuredObj, isUnstructured := obj.(*unstructured.Unstructured)
	if isUnstructured {
		// For unstructured objects, ensure kind is set if missing
		// apiVersion should already be present from cluster, but will be handled below if missing
		if unstructuredObj.GetKind() == "" {
			unstructuredObj.SetKind(kind)
		}
	} else {
		// For typed objects, ensure GVK is set
		// TypeMeta is embedded in the struct, but we need to ensure GVK is set
		// so that it's properly serialized
		gvk := obj.GetObjectKind().GroupVersionKind()
		if gvk.Kind == "" {
			gvk.Kind = kind
		}
		// If Group or Version is empty, try to infer from the object type
		// Objects from cluster should have GVK set, but if not, we'll add it during JSON processing
		obj.GetObjectKind().SetGroupVersionKind(gvk)
	}

	// Output separator if not first resource
	if b.resourceCount > 0 {
		fmt.Fprintf(b.stdout, "\n---\n")
	}
	b.resourceCount++

	// Convert to JSON first to preserve all fields including TypeMeta (kind, apiVersion, spec, status, etc.)
	// For typed objects, TypeMeta is embedded in the struct with json:",inline" tag,
	// so json.Marshal should include it. However, if TypeMeta fields are empty,
	// they might be omitted due to omitempty tags. We ensure GVK is set above.
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal %s/%s to JSON: %w", kind, name, err)
	}

	// Always parse JSON and ensure apiVersion and kind are present
	// This handles cases where TypeMeta might not be serialized properly
	var jsonObj map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &jsonObj); err == nil {
		needsUpdate := false
		// If apiVersion is missing, try to get it from the object itself
		if _, ok := jsonObj["apiVersion"]; !ok {
			var apiVersion string

			// For unstructured objects, get apiVersion directly
			if isUnstructured {
				apiVersion = unstructuredObj.GetAPIVersion()
			} else {
				// For typed objects, get apiVersion from GVK
				// Objects from cluster should have GVK set correctly
				gvk := obj.GetObjectKind().GroupVersionKind()

				// If GVK is empty, try to get it from scheme
				if gvk.Kind == "" || (gvk.Group == "" && gvk.Version == "") {
					// Try to get GVK from scheme
					if gvks, _, err := kubeclient.Scheme.ObjectKinds(obj); err == nil && len(gvks) > 0 {
						gvk = gvks[0]
						// Set GVK on the object for future use
						obj.GetObjectKind().SetGroupVersionKind(gvk)
					}
				}

				// Use GroupVersion().String() which automatically formats as "group/version" or "version"
				// This works for both custom resources (group/version) and core resources (version only)
				apiVersion = gvk.GroupVersion().String()
			}

			// If we got a valid apiVersion, use it
			if apiVersion != "" && apiVersion != "/" {
				jsonObj["apiVersion"] = apiVersion
				needsUpdate = true
			} else {
				// Fallback: for core Kubernetes resources, use "v1"
				// This handles cases where GVK is not set for core resources
				coreKinds := map[string]bool{
					"Pod":                   true,
					"PersistentVolumeClaim": true,
					"PersistentVolume":      true,
					"Event":                 true,
					"Service":               true,
					"ConfigMap":             true,
					"Secret":                true,
				}
				if coreKinds[kind] {
					jsonObj["apiVersion"] = coreAPIVersion
					needsUpdate = true
				}
			}
		}
		// Ensure kind is also present
		if _, ok := jsonObj["kind"]; !ok {
			jsonObj["kind"] = kind
			needsUpdate = true
		}
		// Re-marshal with apiVersion and kind if needed
		if needsUpdate {
			jsonBytes, err = json.Marshal(jsonObj)
			if err != nil {
				return fmt.Errorf("failed to re-marshal %s/%s to JSON: %w", kind, name, err)
			}
		}
	}

	// Convert JSON to YAML - this preserves all fields
	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return fmt.Errorf("failed to convert %s/%s to YAML: %w", kind, name, err)
	}

	// Output comment and full YAML resource
	fmt.Fprintf(b.stdout, "# %s: %s", kind, name)
	if namespace != "" {
		fmt.Fprintf(b.stdout, " (namespace: %s)", namespace)
	}
	fmt.Fprintf(b.stdout, "\n%s", string(yamlBytes))

	return nil
}
