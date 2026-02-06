/*
Copyright 2024 Flant JSC

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

package legacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8snet "k8s.io/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/d8"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
	"github.com/deckhouse/virtualization/test/e2e/internal/network"
)

const (
	CurlPod           = "curl-helper"
	externalHost      = "https://flant.ru"
	nginxActiveStatus = "active"
	httpStatusOk      = "200"
)

func WaitResource(resource kc.Resource, ns, name, waitFor string, timeout time.Duration) {
	GinkgoHelper()
	waitOpts := kc.WaitOptions{
		Namespace: ns,
		For:       waitFor,
		Timeout:   timeout,
	}
	res := kubectl.WaitResources(resource, waitOpts, name)
	Expect(res.Error()).NotTo(HaveOccurred(), "wait failed %s %s/%s.\n%s", resource, ns, name, res.StdErr())
}

func PatchResource(resource kc.Resource, ns, name string, patch []*kc.JSONPatch) {
	GinkgoHelper()
	res := kubectl.PatchResource(resource, name, kc.PatchOptions{
		Namespace: ns,
		JSONPatch: patch,
	})
	Expect(res.Error()).NotTo(HaveOccurred(), "patch failed %s %s/%s.\n%s", resource, ns, name,
		res.StdErr())
}

func MergePatchResource(resource kc.Resource, ns, name, patch string) error {
	GinkgoHelper()
	res := kubectl.PatchResource(resource, name, kc.PatchOptions{
		Namespace:  ns,
		MergePatch: patch,
	})
	if res.Error() != nil {
		return fmt.Errorf("patch failed %s %s/%s.\n%s", resource, ns, name, res.StdErr())
	}
	return nil
}

func CheckField(resource kc.Resource, ns, name, output, compareValue string) {
	GinkgoHelper()
	res := kubectl.GetResource(resource, name, kc.GetOptions{
		Namespace: ns,
		Output:    output,
	})
	Expect(res.Error()).NotTo(HaveOccurred(), "get failed %s %s/%s.\n%s", resource, ns, name, res.StdErr())
	Expect(res.StdOut()).To(Equal(compareValue))
}

func GetObject(resource kc.Resource, name string, object client.Object, opts kc.GetOptions) error {
	GinkgoHelper()
	cmdOpts := kc.GetOptions{
		Output: "json",
	}
	if opts.Namespace != "" {
		cmdOpts.Namespace = opts.Namespace
	}
	if opts.Labels != nil {
		cmdOpts.Labels = opts.Labels
	}
	cmd := kubectl.GetResource(resource, name, cmdOpts)
	if cmd.Error() != nil {
		return errors.New(cmd.StdErr())
	}
	err := json.Unmarshal(cmd.StdOutBytes(), object)
	if err != nil {
		return err
	}
	return nil
}

func GetObjects(resource kc.Resource, object client.ObjectList, opts kc.GetOptions) error {
	GinkgoHelper()
	cmdOpts := kc.GetOptions{
		Output: "json",
	}
	if opts.Namespace != "" {
		cmdOpts.Namespace = opts.Namespace
	}
	if opts.Labels != nil {
		cmdOpts.Labels = opts.Labels
	}
	if opts.ExcludedLabels != nil {
		cmdOpts.ExcludedLabels = opts.ExcludedLabels
	}
	if opts.IgnoreNotFound {
		cmdOpts.IgnoreNotFound = opts.IgnoreNotFound
	}
	cmd := kubectl.List(resource, cmdOpts)
	if cmd.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", cmd.GetCmd(), cmd.StdErr())
	}
	if cmd.StdOut() != "" {
		err := json.Unmarshal(cmd.StdOutBytes(), object)
		if err != nil {
			return err
		}
	}
	return nil
}

func ChmodFile(pathFile string, permission os.FileMode) error {
	stats, err := os.Stat(pathFile)
	if err != nil {
		return err
	}

	if stats.Mode().Perm() != permission {
		err = os.Chmod(pathFile, permission)
		if err != nil {
			return err
		}
	}
	return nil
}

func WaitVMAgentReady(opts kc.WaitOptions) {
	GinkgoHelper()
	WaitPhaseByLabel(kc.ResourceVM, PhaseRunning, opts)
	WaitConditionIsTrueByLabel(kc.ResourceVM, vmcondition.TypeAgentReady.String(), opts)
}

func WaitConditionIsTrueByLabel(resource kc.Resource, conditionName string, opts kc.WaitOptions) {
	GinkgoHelper()
	opts.For = fmt.Sprintf("condition=%s=True", conditionName)
	WaitByLabel(resource, opts)
}

// Useful when require to async await resources filtered by labels.
//
//	Static condition `wait --for`: `jsonpath={.status.phase}=phase`.
func WaitPhaseByLabel(resource kc.Resource, phase string, opts kc.WaitOptions) {
	GinkgoHelper()
	opts.For = fmt.Sprintf("'jsonpath={.status.phase}=%s'", phase)
	WaitByLabel(resource, opts)
}

func WaitByLabel(resource kc.Resource, opts kc.WaitOptions) {
	GinkgoHelper()

	res := kubectl.List(resource, kc.GetOptions{
		ExcludedLabels: opts.ExcludedLabels,
		Labels:         opts.Labels,
		Namespace:      opts.Namespace,
		Output:         "jsonpath='{.items[*].metadata.name}'",
	})
	Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

	var resources []string
	if stdout := res.StdOut(); stdout != "" {
		resources = strings.Split(res.StdOut(), " ")
	}
	WaitResources(resources, resource, opts)
}

// Useful when require to async await resources with specified names.
//
// Do not use 'labels' or 'excluded labels' in opts; they will be ignored.
//
//	Static condition `wait --for`: `jsonpath={.status.phase}=phase`.
func WaitResourcesByPhase(resources []string, resource kc.Resource, phase string, opts kc.WaitOptions) {
	GinkgoHelper()
	opts.For = fmt.Sprintf("'jsonpath={.status.phase}=%s'", phase)
	WaitResources(resources, resource, opts)
}

func WaitResources(resources []string, resource kc.Resource, opts kc.WaitOptions) {
	GinkgoHelper()

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	waitErr := make([]string, 0, len(resources))
	waitOpts := kc.WaitOptions{
		For:       opts.For,
		Namespace: opts.Namespace,
		Timeout:   opts.Timeout,
	}

	for _, name := range resources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := kubectl.WaitResource(resource, name, waitOpts)
			if res.Error() != nil {
				mu.Lock()
				waitErr = append(waitErr, fmt.Sprintf("cmd: %s\nstderr: %s\nwaited for: %s", res.GetCmd(), res.StdErr(), opts.For))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	Expect(waitErr).To(BeEmpty(), "should observe resources in '%s' state before %s timeout", opts.For, opts.Timeout.String())
}

func GetStorageClassFromEnv(envName string) (*storagev1.StorageClass, error) {
	sc := &storagev1.StorageClass{}
	scName, ok := os.LookupEnv(envName)
	if ok {
		err := GetObject(kc.ResourceStorageClass, scName, sc, kc.GetOptions{})
		if err != nil {
			return nil, err
		}
		return sc, nil
	}

	return nil, nil
}

func SetStorageClass(tmplRoot string, storageClasse map[string]string) error {
	return filepath.Walk(tmplRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to set a storage class: %w", err)
		}

		if !info.IsDir() {
			tmpl, err := template.ParseFiles(path)
			if err != nil {
				return err
			}

			file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, info.Mode())
			if err != nil {
				return err
			}
			defer file.Close()

			err = tmpl.Execute(file, storageClasse)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func GetDefaultStorageClass() (*storagev1.StorageClass, error) {
	var scList storagev1.StorageClassList
	res := kubectl.List(kc.ResourceStorageClass, kc.GetOptions{Output: "json"})
	if !res.WasSuccess() {
		return nil, errors.New(res.StdErr())
	}

	err := json.Unmarshal([]byte(res.StdOut()), &scList)
	if err != nil {
		return nil, err
	}

	var defaultClasses []*storagev1.StorageClass
	for idx := range scList.Items {
		if scList.Items[idx].Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			defaultClasses = append(defaultClasses, &scList.Items[idx])
		}
	}

	if len(defaultClasses) == 0 {
		return nil, errors.New("default StorageClass not found in the cluster: please set a default StorageClass")
	}

	// Primary sort by creation timestamp, newest first
	// Secondary sort by class name, ascending order
	sort.Slice(defaultClasses, func(i, j int) bool {
		if defaultClasses[i].CreationTimestamp.UnixNano() == defaultClasses[j].CreationTimestamp.UnixNano() {
			return defaultClasses[i].Name < defaultClasses[j].Name
		}
		return defaultClasses[i].CreationTimestamp.UnixNano() > defaultClasses[j].CreationTimestamp.UnixNano()
	})

	return defaultClasses[0], nil
}

func GetImmediateStorageClass(provisioner string) (*storagev1.StorageClass, error) {
	scl := &storagev1.StorageClassList{}
	err := GetObjects(kc.ResourceStorageClass, scl, kc.GetOptions{})
	if err != nil {
		return nil, err
	}

	for _, sc := range scl.Items {
		if sc.Provisioner == provisioner && *sc.VolumeBindingMode == storagev1.VolumeBindingImmediate {
			return &sc, nil
		}
	}

	return nil, fmt.Errorf("immediate storage class does not found; please set up immediate storage class with the %q provisioner; to skip the immediate storage class check, set %s=yes",
		provisioner,
		config.SkipImmediateStorageClassCheckEnv,
	)
}

func GetWaitForFirstConsumerStorageClass() (*storagev1.StorageClass, error) {
	scList := storagev1.StorageClassList{}
	err := GetObjects(kc.ResourceStorageClass, &scList, kc.GetOptions{})
	if err != nil {
		return nil, err
	}
	for _, sc := range scList.Items {
		if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
			return &sc, nil
		}
	}
	return nil, nil
}

func toIPNet(prefix netip.Prefix) *net.IPNet {
	return &net.IPNet{
		IP:   prefix.Masked().Addr().AsSlice(),
		Mask: net.CIDRMask(prefix.Bits(), prefix.Addr().BitLen()),
	}
}

func isFirstLastIP(ip netip.Addr, cidr netip.Prefix) (bool, error) {
	ipNet := toIPNet(cidr)
	size := int(k8snet.RangeSize(ipNet))

	first, err := k8snet.GetIndexedIP(ipNet, 0)
	if err != nil {
		return false, err
	}

	if first.Equal(ip.AsSlice()) {
		return true, nil
	}

	last, err := k8snet.GetIndexedIP(ipNet, size-1)
	if err != nil {
		return false, err
	}

	return last.Equal(ip.AsSlice()), nil
}

func FindUnassignedIP(subnets []string) (string, error) {
	findError := fmt.Errorf("error: cannot find unassigned IP address")
	res := kubectl.List(kc.ResourceVMIPLease, kc.GetOptions{Output: "jsonpath='{.items[*].metadata.name}'"})
	if !res.WasSuccess() {
		return "", fmt.Errorf("failed to get vmipl: %s", res.StdErr())
	}
	ips := strings.Split(res.StdOut(), " ")
	reservedIPs := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		reservedIPs[ip] = struct{}{}
	}
	for _, rawSubnet := range subnets {
		prefix, err := netip.ParsePrefix(rawSubnet)
		if err != nil {
			return "", fmt.Errorf("failed to parse subnet %s: %w", rawSubnet, err)
		}
		nextAddr := prefix.Addr().Next()
		for {
			nextAddr = nextAddr.Next()
			ip := fmt.Sprintf("ip-%s", strings.ReplaceAll(nextAddr.String(), ".", "-"))
			if _, found := reservedIPs[ip]; found {
				continue
			}
			isFirstLast, err := isFirstLastIP(nextAddr, prefix)
			if err != nil {
				return "", findError
			}
			if isFirstLast {
				continue
			}
			if prefix.Contains(nextAddr) {
				return nextAddr.String(), nil
			}
			break
		}
	}
	return "", findError
}

func GetConditionStatus(obj client.Object, conditionType string) (metav1.ConditionStatus, error) {
	condition, err := GetCondition(conditionType, obj)
	if err != nil {
		return "", err
	}

	return condition.Status, nil
}

func GetCondition(conditionType string, obj client.Object) (metav1.Condition, error) {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return metav1.Condition{}, err
	}

	unstructuredObj := &unstructured.Unstructured{Object: u}

	conditions, found, err := unstructured.NestedSlice(unstructuredObj.Object, "status", "conditions")
	if err != nil {
		return metav1.Condition{}, err
	}
	if !found {
		return metav1.Condition{}, fmt.Errorf(".status.conditions not found")
	}

	for _, c := range conditions {
		if conditionMap, isMap := c.(map[string]interface{}); isMap {
			if conditionMap["type"] == conditionType {
				return metav1.Condition{
					Type:               conditionMap["type"].(string),
					Status:             metav1.ConditionStatus(conditionMap["status"].(string)),
					Reason:             conditionMap["reason"].(string),
					ObservedGeneration: conditionMap["observedGeneration"].(int64),
				}, nil
			}
		}
	}

	return metav1.Condition{}, fmt.Errorf("condition %s not found", conditionType)
}

func GetPhaseByVolumeBindingModeForTemplateSc() string {
	return GetPhaseByVolumeBindingMode(conf.StorageClass.TemplateStorageClass)
}

func GetPhaseByVolumeBindingMode(sc *storagev1.StorageClass) string {
	switch *sc.VolumeBindingMode {
	case storagev1.VolumeBindingImmediate:
		return string(v1alpha2.DiskReady)
	case storagev1.VolumeBindingWaitForFirstConsumer:
		return string(v1alpha2.DiskWaitForFirstConsumer)
	default:
		return string(v1alpha2.DiskReady)
	}
}

// Test data templates does not contain this resources, but this resources are created in test case.
type AdditionalResource struct {
	Resource kc.Resource
	Labels   map[string]string
}

// KustomizationDir - `kubectl delete --kustomize <dir>`
//
// AdditionalResources - for each resource `kubectl delete <resource> <labels>`
//
// Files - `kubectl delete --filename <files>`
type ResourcesToDelete struct {
	KustomizationDir    string
	AdditionalResources []AdditionalResource
	Files               []string
}

// This function checks that all resources in test case can be deleted correctly.
func DeleteTestCaseResources(ns string, resources ResourcesToDelete) {
	By("Response on deletion request should be successful", func() {
		const errMessage = "cannot delete test case resources"

		if resources.KustomizationDir != "" {
			kustimizationFile := fmt.Sprintf("%s/%s", resources.KustomizationDir, "kustomization.yaml")
			err := kustomize.ExcludeResource(kustimizationFile, "ns.yaml")
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("%s\nkustomizationDir: %s\nstderr: %s", errMessage, resources.KustomizationDir, err))

			res := kubectl.Delete(kc.DeleteOptions{
				Filename:       []string{resources.KustomizationDir},
				FilenameOption: kc.Kustomize,
				IgnoreNotFound: true,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), fmt.Sprintf("%s\nkustomizationDir: %s\ncmd: %s\nstderr: %s", errMessage, resources.KustomizationDir, res.GetCmd(), res.StdErr()))
		}

		for _, r := range resources.AdditionalResources {
			res := kubectl.Delete(kc.DeleteOptions{
				Labels:    r.Labels,
				Namespace: ns,
				Resource:  r.Resource,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), fmt.Sprintf("%s\ncmd: %s\nstderr: %s", errMessage, res.GetCmd(), res.StdErr()))
		}

		if len(resources.Files) != 0 {
			res := kubectl.Delete(kc.DeleteOptions{
				Filename:       resources.Files,
				FilenameOption: kc.Filename,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), fmt.Sprintf("%s\ncmd: %s\nstderr: %s", errMessage, res.GetCmd(), res.StdErr()))
		}
	})
}

func MigrateVirtualMachines(label map[string]string, vmNamespace string, vmNames ...string) {
	GinkgoHelper()
	CreateAndApplyVMOPs(label, v1alpha2.VMOPTypeEvict, vmNamespace, vmNames...)
}

func RebootVirtualMachinesByVMOP(label map[string]string, vmNamespace string, vmNames ...string) {
	GinkgoHelper()
	CreateAndApplyVMOPs(label, v1alpha2.VMOPTypeRestart, vmNamespace, vmNames...)
}

func StopVirtualMachinesByVMOP(label map[string]string, vmNamespace string, vmNames ...string) {
	GinkgoHelper()
	CreateAndApplyVMOPs(label, v1alpha2.VMOPTypeStop, vmNamespace, vmNames...)
}

func StartVirtualMachinesByVMOP(label map[string]string, vmNamespace string, vmNames ...string) {
	GinkgoHelper()
	CreateAndApplyVMOPs(label, v1alpha2.VMOPTypeStart, vmNamespace, vmNames...)
}

func CreateAndApplyVMOPs(label map[string]string, vmopType v1alpha2.VMOPType, vmNamespace string, vmNames ...string) {
	GinkgoHelper()

	CreateAndApplyVMOPsWithSuffix(label, "", vmopType, vmNamespace, vmNames...)
}

func CreateAndApplyVMOPsWithSuffix(label map[string]string, suffix string, vmopType v1alpha2.VMOPType, vmNamespace string, vmNames ...string) {
	GinkgoHelper()

	for _, vmName := range vmNames {
		vmop := GenerateVMOPWithSuffix(vmName, vmNamespace, suffix, label, vmopType)
		_, err := framework.GetClients().VirtClient().VirtualMachineOperations(vmNamespace).Create(context.TODO(), vmop, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func GenerateVMOP(vmName, vmNamespace string, labels map[string]string, vmopType v1alpha2.VMOPType) *v1alpha2.VirtualMachineOperation {
	return &v1alpha2.VirtualMachineOperation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineOperationKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", vmName, strings.ToLower(string(vmopType))),
			Namespace: vmNamespace,
			Labels:    labels,
		},
		Spec: v1alpha2.VirtualMachineOperationSpec{
			Type:           vmopType,
			VirtualMachine: vmName,
		},
	}
}

func GenerateVMOPWithSuffix(vmName, vmNamespace, suffix string, labels map[string]string, vmopType v1alpha2.VMOPType) *v1alpha2.VirtualMachineOperation {
	res := GenerateVMOP(vmName, vmNamespace, labels, vmopType)
	res.ObjectMeta.Name = fmt.Sprintf("%s%s", res.ObjectMeta.Name, suffix)
	return res
}

func StopVirtualMachinesBySSH(vmNamespace string, vmNames ...string) {
	GinkgoHelper()

	// The `sleep` command is required to prevent an "abnormal closure" error.
	cmd := "nohup sh -c \"sleep 5 && sudo systemctl poweroff --no-wall\" > /dev/null 2>&1 &"

	for _, vmName := range vmNames {
		ExecSSHCommand(vmNamespace, vmName, cmd)
	}
}

func RebootVirtualMachinesBySSH(vmNamespace string, vmNames ...string) {
	GinkgoHelper()

	// The `sleep` command is required to prevent an "abnormal closure" error.
	cmd := "nohup sh -c \"sleep 5 && sudo systemctl reboot --no-wall\" > /dev/null 2>&1 &"

	for _, vmName := range vmNames {
		ExecSSHCommand(vmNamespace, vmName, cmd)
	}
}

func IsContainsAnnotation(obj client.Object, annotation string) bool {
	_, ok := obj.GetAnnotations()[annotation]
	return ok
}

func IsContainsAnnotationWithValue(obj client.Object, annotation, value string) bool {
	val, ok := obj.GetAnnotations()[annotation]
	return ok && val == value
}

func IsContainsLabel(obj client.Object, label string) bool {
	_, ok := obj.GetLabels()[label]
	return ok
}

func IsContainsLabelWithValue(obj client.Object, label, value string) bool {
	val, ok := obj.GetLabels()[label]
	return ok && val == value
}

// SaveTestCaseDump dump some resources, logs and descriptions that may help in further diagnostic.
//
// NOTE: This method is called in AfterEach for failed specs only. Avoid to use Expect,
// as it fails without reporting. Better use GinkgoWriter to report errors at this point.
// leafNodeText: LeafNodeType, LeadNodeLocation, LeafNodeLabels and LeafNodeText capture the NodeType, CodeLocation, and text
// of the Ginkgo node being tested (typically an NodeTypeIt node, though this can also be
// one of the NodeTypesForSuiteLevelNodes node types)
func SaveTestCaseDump(labels map[string]string, leafNodeText, namespace string) {
	replacer := strings.NewReplacer(
		" ", "_",
		":", "_",
		"[", "_",
		"]", "_",
		"(", "_",
		")", "_",
		"|", "_",
		"`", "",
		"'", "",
	)
	leafNodeText = replacer.Replace(strings.ToLower(leafNodeText))

	tmpDir := os.Getenv("RUNNER_TEMP")
	if tmpDir == "" {
		tmpDir = "/tmp"
	}

	SaveTestCaseResources(labels, leafNodeText, namespace, tmpDir)
	SavePodLogsAndDescriptions(labels, leafNodeText, namespace, tmpDir)
	SaveIntvirtvmDescriptions(labels, leafNodeText, namespace, tmpDir)
	SaveNodeOWide(labels, leafNodeText, tmpDir)
	SaveNodeDescribe(labels, leafNodeText, tmpDir)
}

func SaveTestCaseResources(labels map[string]string, leafNodeText, namespace, dumpPath string) {
	resFileName := fmt.Sprintf("%s/e2e_failed__%s__%s.yaml", dumpPath, labels["testcase"], leafNodeText)

	clusterResourceResult := kubectl.Get("cvi,vmc", kc.GetOptions{
		Labels:            labels,
		Namespace:         namespace,
		Output:            "yaml",
		ShowManagedFields: false,
	})
	if clusterResourceResult.Error() != nil {
		GinkgoWriter.Printf("Get resources error:\n%s\n%v\n%s\n", clusterResourceResult.GetCmd(), clusterResourceResult.Error(), clusterResourceResult.StdErr())
	}

	namespacedResourceResult := kubectl.Get("virtualization,intvirt,pod,volumesnapshot,pvc", kc.GetOptions{
		Namespace:         namespace,
		Output:            "yaml",
		ShowManagedFields: false,
	})
	if namespacedResourceResult.Error() != nil {
		GinkgoWriter.Printf("Get resources error:\n%s\n%v\n%s\n", namespacedResourceResult.GetCmd(), namespacedResourceResult.Error(), namespacedResourceResult.StdErr())
	}

	// Stdout may present even if error is occurred.
	if len(clusterResourceResult.StdOutBytes()) > 0 || len(namespacedResourceResult.StdOutBytes()) > 0 {
		delimiter := []byte("---\n")

		result := make([]byte, 0, len(clusterResourceResult.StdOutBytes())+len(delimiter)+len(namespacedResourceResult.StdOutBytes()))
		result = append(result, clusterResourceResult.StdOutBytes()...)
		result = append(result, delimiter...)
		result = append(result, namespacedResourceResult.StdOutBytes()...)

		err := os.WriteFile(resFileName, result, 0o644)
		if err != nil {
			GinkgoWriter.Printf("Save resources to file '%s' failed: %s\n", resFileName, err)
		}
	}
}

func SavePodLogsAndDescriptions(labels map[string]string, leafNodeText, namespace, dumpPath string) {
	pods := &corev1.PodList{}
	err := GetObjects(kc.ResourcePod, pods, kc.GetOptions{Namespace: namespace})
	if err != nil {
		GinkgoWriter.Printf("Failed to get PodList:\n%s\n", err)
	}

	if len(pods.Items) == 0 {
		GinkgoWriter.Println("The list of pods is empty; nothing to dump.")
	}

	for _, pod := range pods.Items {
		// Get pod logs
		logCmd := kubectl.RawCommand(fmt.Sprintf("logs %s --namespace %s", pod.Name, pod.Namespace), framework.ShortTimeout)
		if logCmd.Error() != nil {
			GinkgoWriter.Printf("Failed to get logs:\nPodName: %s\nError: %s\n", pod.Name, logCmd.StdErr())
		}

		fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__%s__logs.json", dumpPath, labels["testcase"], leafNodeText, pod.Name)
		err := os.WriteFile(fileName, logCmd.StdOutBytes(), 0o644)
		if err != nil {
			GinkgoWriter.Printf("Failed to save logs:\nPodName: %s\nError: %s\n", pod.Name, err)
		}

		// Get pod descriptions
		describeCmd := kubectl.RawCommand(fmt.Sprintf("describe pod %s --namespace %s", pod.Name, pod.Namespace), framework.ShortTimeout)
		if describeCmd.Error() != nil {
			GinkgoWriter.Printf("Failed to describe pod:\nPodName: %s\nError: %s\n", pod.Name, describeCmd.StdErr())
		}

		fileName = fmt.Sprintf("%s/e2e_failed__%s__%s__%s__describe", dumpPath, labels["testcase"], leafNodeText, pod.Name)
		err = os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
		if err != nil {
			GinkgoWriter.Printf("Failed to save pod description:\nPodName: %s\nError: %s\n", pod.Name, err)
		}

		// Get pod guest info
		if pod.Labels != nil && pod.Status.Phase == corev1.PodRunning {
			if value, ok := pod.Labels["kubevirt.internal.virtualization.deckhouse.io"]; ok && value == "virt-launcher" {
				vlctlGuestInfoCmd := kubectl.RawCommand(fmt.Sprintf("exec --stdin=true --tty=true %s --namespace %s -- vlctl guest info", pod.Name, pod.Namespace), framework.ShortTimeout)
				if vlctlGuestInfoCmd.Error() != nil {
					GinkgoWriter.Printf("Failed to get pod guest info:\nPodName: %s\nError: %s\n", pod.Name, vlctlGuestInfoCmd.StdErr())
				}

				fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__%s__vlctl_guest_info", dumpPath, labels["testcase"], leafNodeText, pod.Name)
				err := os.WriteFile(fileName, vlctlGuestInfoCmd.StdOutBytes(), 0o644)
				if err != nil {
					GinkgoWriter.Printf("Failed to save pod guest info:\nPodName: %s\nError: %s\n", pod.Name, err)
				}
			}
		}
	}
}

func SaveIntvirtvmDescriptions(labels map[string]string, leafNodeText, namespace, dumpPath string) {
	describeCmd := kubectl.RawCommand(fmt.Sprintf("describe intvirtvm --namespace %s", namespace), framework.ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to describe InternalVirtualizationVirtualMachine:\nError: %s\n", describeCmd.StdErr())
	}

	fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__intvirtvm_describe", dumpPath, labels["testcase"], leafNodeText)
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save InternalVirtualizationVirtualMachine description:\nError: %s\n", err)
	}
}

func SaveNodeOWide(labels map[string]string, leafNodeText, dumpPath string) {
	GinkgoHelper()
	describeCmd := kubectl.RawCommand("get nodes -o wide", framework.ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to get node owide:\nError: %s\n", describeCmd.StdErr())
	}
	fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__nodes_owide.log", dumpPath, labels["testcase"], leafNodeText)
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save node owide:\nError: %s\n", err)
	}
}

func SaveNodeDescribe(labels map[string]string, leafNodeText, dumpPath string) {
	GinkgoHelper()
	describeCmd := kubectl.RawCommand("describe node -o wide", framework.ShortTimeout)
	if describeCmd.Error() != nil {
		GinkgoWriter.Printf("Failed to get node describe:\nError: %s\n", describeCmd.StdErr())
	}
	fileName := fmt.Sprintf("%s/e2e_failed__%s__%s__nodes_describe.log", dumpPath, labels["testcase"], leafNodeText)
	err := os.WriteFile(fileName, describeCmd.StdOutBytes(), 0o644)
	if err != nil {
		GinkgoWriter.Printf("Failed to save node describe:\nError: %s\n", err)
	}
}

func CreateResource(ctx context.Context, obj client.Object) {
	GinkgoHelper()
	err := framework.GetClients().GenericClient().Create(ctx, obj)
	Expect(err).NotTo(HaveOccurred())
}

func DeleteResource(ctx context.Context, obj client.Object) {
	GinkgoHelper()
	err := framework.GetClients().GenericClient().Delete(ctx, obj)
	Expect(err).NotTo(HaveOccurred())
}

func CreateNamespace(name string) {
	GinkgoHelper()

	result := kubectl.RawCommand(fmt.Sprintf("create namespace %s", name), ShortTimeout)
	Expect(result.Error()).NotTo(HaveOccurred(), result.GetCmd(), result.StdErr())

	WaitResourcesByPhase(
		[]string{name},
		kc.ResourceNamespace,
		string(corev1.NamespaceActive),
		kc.WaitOptions{
			Timeout: ShortTimeout,
		},
	)
}

func CheckResultSSHCommand(vmNamespace, vmName, cmd, equal string) {
	GinkgoHelper()
	Eventually(func() (string, error) {
		res := framework.GetClients().D8Virtualization().SSHCommand(vmName, cmd, d8.SSHOptions{
			Namespace:    vmNamespace,
			Username:     conf.TestData.SSHUser,
			IdentityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return "", fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		return strings.TrimSpace(res.StdOut()), nil
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Equal(equal))
}

func CheckCiliumAgents(kubectl kc.Kubectl, namespace string, vms ...string) {
	GinkgoHelper()
	for _, vm := range vms {
		By(fmt.Sprintf("Cilium agent should be OK's for VM: %s", vm))
		Eventually(func() error {
			return network.CheckCiliumAgents(context.Background(), kubectl, vm, namespace)
		}).
			WithTimeout(Timeout).
			WithPolling(Interval).
			Should(Succeed())
	}
}

func CheckExternalConnection(host, httpCode, vmNamespace string, vmNames ...string) {
	GinkgoHelper()
	for _, vmName := range vmNames {
		By(fmt.Sprintf("Response code from %q should be %q for %q", host, httpCode, vmName))
		cmd := fmt.Sprintf("curl -o /dev/null -s -w \"%%{http_code}\\n\" %s", host)
		CheckResultSSHCommand(vmNamespace, vmName, cmd, httpCode)
	}
}

func ExecSSHCommand(vmNamespace, vmName, cmd string) {
	GinkgoHelper()

	Eventually(func() error {
		res := framework.GetClients().D8Virtualization().SSHCommand(vmName, cmd, d8.SSHOptions{
			Namespace:    vmNamespace,
			Username:     conf.TestData.SSHUser,
			IdentityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		return nil
	}).WithTimeout(Timeout).WithPolling(Interval).ShouldNot(HaveOccurred())
}
