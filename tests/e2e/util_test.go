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

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	"k8s.io/apimachinery/pkg/watch"
	k8snet "k8s.io/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
	"github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

type ApplyWaitGetOptions struct {
	WaitTimeout time.Duration
	Phase       string
}

func ItApplyWaitGet(filepath string, options ApplyWaitGetOptions) {
	GinkgoHelper()
	timeout := ShortWaitDuration
	if options.WaitTimeout != 0 {
		timeout = options.WaitTimeout
	}
	phase := PhaseReady
	if options.Phase != "" {
		phase = options.Phase
	}
	ItApplyFromFile(filepath)
	ItWaitFromFile(filepath, phase, timeout)
	ItChekStatusPhaseFromFile(filepath, phase)
}

func ItApplyFromFile(filepath string) {
	GinkgoHelper()
	It("Apply resource from file", func() {
		ApplyFromFile(filepath)
	})
}

func ApplyFromFile(filepath string) {
	GinkgoHelper()
	fmt.Printf("Apply file %s\n", filepath)
	res := kubectl.Apply(kc.ApplyOptions{
		Filename:       []string{filepath},
		FilenameOption: kc.Filename,
	})
	Expect(res.Error()).NotTo(HaveOccurred(), "apply failed for file %s\n%s", filepath, res.StdErr())
}

func ItWaitFromFile(filepath, phase string, timeout time.Duration) {
	GinkgoHelper()
	It("Wait resource", func() {
		WaitFromFile(filepath, phase, timeout)
	})
}

func WaitFromFile(filepath, phase string, timeout time.Duration) {
	GinkgoHelper()
	For := "jsonpath={.status.phase}=" + phase
	res := kubectl.Wait(filepath, kc.WaitOptions{
		Timeout: timeout,
		For:     For,
	})
	Expect(res.Error()).NotTo(HaveOccurred(), "wait failed for file %s\n%s", filepath, res.StdErr())
}

func ItChekStatusPhaseFromFile(filepath, phase string) {
	GinkgoHelper()
	out := "jsonpath={.status.phase}"
	ItCheckStatusFromFile(filepath, out, phase)
}

func ItCheckStatusFromFile(filepath, output, compareField string) {
	GinkgoHelper()
	unstructs, err := helper.ParseYaml(filepath)
	Expect(err).NotTo(HaveOccurred(), "cannot decode objs from yaml file %s", filepath)

	for _, u := range unstructs {
		It("Get recourse status "+u.GetName(), func() {
			fullName := helper.GetFullAPIResourceName(u)
			var res *executor.CMDResult
			if u.GetNamespace() == "" {
				res = kubectl.GetResource(fullName, u.GetName(), kc.GetOptions{Output: output})
			} else {
				res = kubectl.GetResource(fullName, u.GetName(), kc.GetOptions{
					Output:    output,
					Namespace: u.GetNamespace(),
				})
			}
			Expect(res.Error()).NotTo(HaveOccurred(),
				"get failed resource %s %s/%s.\n%s",
				u.GetKind(),
				u.GetNamespace(),
				u.GetName(),
				res.StdErr(),
			)
			Expect(res.StdOut()).To(Equal(compareField))
		})
	}
}

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

func GetVMFromManifest(manifest string) (*virtv2.VirtualMachine, error) {
	unstructs, err := helper.ParseYaml(manifest)
	if err != nil {
		return nil, err
	}
	var unstruct *unstructured.Unstructured
	for _, u := range unstructs {
		if helper.GetFullAPIResourceName(u) == kc.ResourceVM {
			unstruct = u
			break
		}
	}
	var vm virtv2.VirtualMachine
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.Object, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
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

func ChmodFile(pathFile string, permission os.FileMode) {
	stats, err := os.Stat(pathFile)
	if err != nil {
		log.Fatal(err)
	}

	if stats.Mode().Perm() != permission {
		err = os.Chmod(pathFile, permission)
		if err != nil {
			log.Fatal(err)
		}
	}
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
				waitErr = append(waitErr, fmt.Sprintf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr()))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	Expect(waitErr).To(BeEmpty())
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
		return nil, fmt.Errorf("Default StorageClass not found in the cluster: please set a default StorageClass.")
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
		return string(virtv2.DiskReady)
	case storagev1.VolumeBindingWaitForFirstConsumer:
		return string(virtv2.DiskWaitForFirstConsumer)
	default:
		return string(virtv2.DiskReady)
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

func RebootVirtualMachinesByVMOP(label map[string]string, vmNamespace string, vmNames ...string) {
	GinkgoHelper()
	CreateAndApplyVMOPs(label, virtv2.VMOPTypeRestart, vmNamespace, vmNames...)
}

func StopVirtualMachinesByVMOP(label map[string]string, vmNamespace string, vmNames ...string) {
	GinkgoHelper()
	CreateAndApplyVMOPs(label, virtv2.VMOPTypeStop, vmNamespace, vmNames...)
}

func StartVirtualMachinesByVMOP(label map[string]string, vmNamespace string, vmNames ...string) {
	GinkgoHelper()
	CreateAndApplyVMOPs(label, virtv2.VMOPTypeStart, vmNamespace, vmNames...)
}

func CreateAndApplyVMOPs(label map[string]string, vmopType virtv2.VMOPType, vmNamespace string, vmNames ...string) {
	GinkgoHelper()

	CreateAndApplyVMOPsWithSuffix(label, "", vmopType, vmNamespace, vmNames...)
}

func CreateAndApplyVMOPsWithSuffix(label map[string]string, suffix string, vmopType virtv2.VMOPType, vmNamespace string, vmNames ...string) {
	GinkgoHelper()

	for _, vmName := range vmNames {
		vmop := GenerateVMOPWithSuffix(vmName, vmNamespace, suffix, label, vmopType)
		_, err := virtClient.VirtualMachineOperations(vmNamespace).Create(context.TODO(), vmop, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

	}
}

func GenerateVMOP(vmName, vmNamespace string, labels map[string]string, vmopType virtv2.VMOPType) *virtv2.VirtualMachineOperation {
	return &virtv2.VirtualMachineOperation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: virtv2.SchemeGroupVersion.String(),
			Kind:       virtv2.VirtualMachineOperationKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", vmName, strings.ToLower(string(vmopType))),
			Namespace: vmNamespace,
			Labels:    labels,
		},
		Spec: virtv2.VirtualMachineOperationSpec{
			Type:           vmopType,
			VirtualMachine: vmName,
		},
	}
}

func GenerateVMOPWithSuffix(vmName, vmNamespace, suffix string, labels map[string]string, vmopType virtv2.VMOPType) *virtv2.VirtualMachineOperation {
	res := GenerateVMOP(vmName, vmNamespace, labels, vmopType)
	res.ObjectMeta.Name = fmt.Sprintf("%s%s", res.ObjectMeta.Name, suffix)
	return res
}

func StopVirtualMachinesBySSH(vmNamespace string, vmNames ...string) {
	GinkgoHelper()

	cmd := "sudo nohup poweroff -f > /dev/null 2>&1 &"

	for _, vmName := range vmNames {
		ExecSSHCommand(vmNamespace, vmName, cmd)
	}
}

func RebootVirtualMachinesBySSH(vmNamespace string, vmNames ...string) {
	GinkgoHelper()

	cmd := "sudo nohup reboot -f > /dev/null 2>&1 &"

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

func IsContainerRestarted(podName, containerName, namespace string, startedAt metav1.Time) (bool, error) {
	podObj := &corev1.Pod{}
	err := GetObject(kc.ResourcePod, podName, podObj, kc.GetOptions{
		Namespace: namespace,
	})
	if err != nil {
		return false, fmt.Errorf("failed to obtain the pod object(this may be caused by restarting the pod: %s)", podName)
	}
	for _, cs := range podObj.Status.ContainerStatuses {
		if cs.Name == containerName {
			if cs.State.Running.StartedAt != startedAt {
				return true, fmt.Errorf("the container %q was restarted: %s", containerName, podName)
			} else {
				return false, nil
			}
		}
	}
	return false, fmt.Errorf("failed to compare the `startedAt` field before and after the tests ran: %s", podName)
}

func SaveTestResources(labels map[string]string, additional string) {
	replacer := strings.NewReplacer(
		" ", "_",
		":", "_",
		"[", "_",
		"]", "_",
		"(", "_",
		")", "_",
		"|", "_",
	)
	additional = replacer.Replace(strings.ToLower(additional))

	str := fmt.Sprintf("/tmp/e2e_failed__%s__%s.yaml", labels["testcase"], additional)

	cmdr := kubectl.Get("virtualization -A", kc.GetOptions{Output: "yaml", Labels: labels})
	Expect(cmdr.Error()).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", cmdr.GetCmd(), cmdr.StdErr())

	err := os.WriteFile(str, cmdr.StdOutBytes(), 0o644)
	Expect(err).NotTo(HaveOccurred(), "cmd: %s\nstderr: %s", cmdr.GetCmd(), cmdr.StdErr())
}

type Watcher interface {
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

type Resource interface {
	*virtv2.VirtualMachineIPAddress | *virtv2.VirtualMachineIPAddressLease
}

type EventHandler[R Resource] func(eventType watch.EventType, r R) (bool, error)

func WaitFor[R Resource](ctx context.Context, w Watcher, h EventHandler[R], opts metav1.ListOptions) (R, error) {
	wi, err := w.Watch(ctx, opts)
	if err != nil {
		return nil, err
	}

	defer wi.Stop()

	for event := range wi.ResultChan() {
		r, ok := event.Object.(R)
		if !ok {
			return nil, errors.New("conversion error")
		}

		ok, err = h(event.Type, r)
		if err != nil {
			return nil, err
		}

		if ok {
			return r, nil
		}
	}

	return nil, fmt.Errorf("the condition for matching was not successfully met: %w", ctx.Err())
}

func CreateResource(ctx context.Context, obj client.Object) {
	GinkgoHelper()
	err := crClient.Create(ctx, obj)
	Expect(err).NotTo(HaveOccurred())
}

func DeleteResource(ctx context.Context, obj client.Object) {
	GinkgoHelper()
	err := crClient.Delete(ctx, obj)
	Expect(err).NotTo(HaveOccurred())
}

func CreateNamespace(name string) {
	GinkgoHelper()

	result := kubectl.RawCommand(fmt.Sprintf("create namespace %s", name), ShortTimeout)
	Expect(result.Error()).NotTo(HaveOccurred(), result.GetCmd())

	WaitResourcesByPhase(
		[]string{name},
		kc.ResourceNamespace,
		string(corev1.NamespaceActive),
		kc.WaitOptions{
			Timeout: ShortTimeout,
		},
	)
}
