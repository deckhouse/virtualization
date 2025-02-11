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
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8snet "k8s.io/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
	. "github.com/deckhouse/virtualization/tests/e2e/config"
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
			fullName := helper.GetFullApiResourceName(u)
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

func WaitResource(resource kc.Resource, name, waitFor string, timeout time.Duration) {
	GinkgoHelper()
	waitOpts := kc.WaitOptions{
		Namespace: conf.Namespace,
		For:       waitFor,
		Timeout:   timeout,
	}
	res := kubectl.WaitResources(resource, waitOpts, name)
	Expect(res.Error()).NotTo(HaveOccurred(), "wait failed %s %s/%s.\n%s", resource, conf.Namespace, name, res.StdErr())
}

func PatchResource(resource kc.Resource, name string, patch *kc.JsonPatch) {
	GinkgoHelper()
	res := kubectl.PatchResource(resource, name, kc.PatchOptions{
		Namespace: conf.Namespace,
		JsonPatch: patch,
	})
	Expect(res.Error()).NotTo(HaveOccurred(), "patch failed %s %s/%s.\n%s", resource, conf.Namespace, name,
		res.StdErr())
}

func MergePatchResource(resource kc.Resource, name, patch string) error {
	GinkgoHelper()
	res := kubectl.PatchResource(resource, name, kc.PatchOptions{
		Namespace:  conf.Namespace,
		MergePatch: patch,
	})
	if res.Error() != nil {
		return fmt.Errorf("patch failed %s %s/%s.\n%s", resource, conf.Namespace, name, res.StdErr())
	}
	return nil
}

func CheckField(resource kc.Resource, name, output, compareValue string) {
	GinkgoHelper()
	res := kubectl.GetResource(resource, name, kc.GetOptions{
		Namespace: conf.Namespace,
		Output:    output,
	})
	Expect(res.Error()).NotTo(HaveOccurred(), "get failed %s %s/%s.\n%s", resource, conf.Namespace, name, res.StdErr())
	Expect(res.StdOut()).To(Equal(compareValue))
}

func GetVMFromManifest(manifest string) (*virtv2.VirtualMachine, error) {
	unstructs, err := helper.ParseYaml(manifest)
	if err != nil {
		return nil, err
	}
	var unstruct *unstructured.Unstructured
	for _, u := range unstructs {
		if helper.GetFullApiResourceName(u) == kc.ResourceVM {
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
	cmd := kubectl.List(resource, cmdOpts)
	if cmd.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", cmd.GetCmd(), cmd.StdErr())
	}
	err := json.Unmarshal(cmd.StdOutBytes(), object)
	if err != nil {
		return err
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

func WaitVmReady(opts kc.WaitOptions) {
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
	wg := sync.WaitGroup{}
	mu := sync.Mutex{}

	res := kubectl.List(resource, kc.GetOptions{
		ExcludedLabels: opts.ExcludedLabels,
		Labels:         opts.Labels,
		Namespace:      opts.Namespace,
		Output:         "jsonpath='{.items[*].metadata.name}'",
	})
	Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

	resources := strings.Split(res.StdOut(), " ")
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

func GetConditionStatus(obj client.Object, conditionType string) (string, error) {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return "", err
	}

	unstructuredObj := &unstructured.Unstructured{Object: u}

	conditions, found, err := unstructured.NestedSlice(unstructuredObj.Object, "status", "conditions")
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf(".status.conditions not found")
	}

	for _, c := range conditions {
		if conditionMap, isMap := c.(map[string]interface{}); isMap {
			if conditionMap["type"] == conditionType {
				if status, exists := conditionMap["status"].(string); exists {
					return status, nil
				}
			}
		}
	}

	return "", fmt.Errorf("condition %s not found", conditionType)
}

func GetPhaseByVolumeBindingMode(c *Config) string {
	switch c.StorageClass.VolumeBindingMode {
	case "Immediate":
		return PhaseReady
	case "WaitForFirstConsumer":
		return PhaseWaitForFirstConsumer
	default:
		return PhaseReady
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
func DeleteTestCaseResources(resources ResourcesToDelete) {
	By("Response on deletion request should be successful", func() {
		const errMessage = "cannot delete test case resources"

		if resources.KustomizationDir != "" {
			kustimizationFile := fmt.Sprintf("%s/%s", resources.KustomizationDir, "kustomization.yaml")
			err := kustomize.ExcludeResource(kustimizationFile, "ns.yaml")
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("%s\nkustomizationDir: %s\nstderr: %s", errMessage, resources.KustomizationDir, err))

			res := kubectl.Delete(kc.DeleteOptions{
				Filename:       []string{resources.KustomizationDir},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.Error()).NotTo(HaveOccurred(), fmt.Sprintf("%s\nkustomizationDir: %s\ncmd: %s\nstderr: %s", errMessage, resources.KustomizationDir, res.GetCmd(), res.StdErr()))
		}

		for _, r := range resources.AdditionalResources {
			res := kubectl.Delete(kc.DeleteOptions{
				Labels:    r.Labels,
				Namespace: conf.Namespace,
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
