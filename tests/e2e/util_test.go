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
	"fmt"
	"log"
	"net/netip"
	"os"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/tests/e2e/executor"
	"github.com/deckhouse/virtualization/tests/e2e/helper"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	res := kubectl.Apply(filepath, kc.ApplyOptions{})
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
					Namespace: u.GetNamespace()})
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

func WaitResource(resource kc.Resource, name, For string, timeout time.Duration) {
	GinkgoHelper()
	waitOpts := kc.WaitOptions{
		Namespace: conf.Namespace,
		For:       For,
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

func MergePatchResource(resource kc.Resource, name string, patch string) {
	GinkgoHelper()
	res := kubectl.PatchResource(resource, name, kc.PatchOptions{
		Namespace:  conf.Namespace,
		MergePatch: patch,
	})
	Expect(res.Error()).NotTo(HaveOccurred(), "patch failed %s %s/%s.\n%s", resource, conf.Namespace, name,
		res.StdErr())
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

type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualMachineSpec `json:"spec"`
}

type VirtualMachineSpec struct {
	RunPolicy                        RunPolicy           `json:"runPolicy"`
	VirtualMachineIPAddressClaimName string              `json:"virtualMachineIPAddressClaimName,omitempty"`
	NodeSelector                     map[string]string   `json:"nodeSelector,omitempty"`
	PriorityClassName                string              `json:"priorityClassName"`
	Tolerations                      []corev1.Toleration `json:"tolerations,omitempty"`
	TerminationGracePeriodSeconds    *int64              `json:"terminationGracePeriodSeconds,omitempty"`
	EnableParavirtualization         bool                `json:"enableParavirtualization,omitempty"`

	ApprovedChangeID string `json:"approvedChangeID,omitempty"`
}

type RunPolicy string

func GetVMFromManifest(manifest string) (*VirtualMachine, error) {
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
	var vm VirtualMachine
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.Object, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
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

func CheckDefaultStorageClass() error {
	storageClass := kc.Resource("sc")
	res := kubectl.List(storageClass, kc.GetOptions{Output: "json"})
	if !res.WasSuccess() {
		return fmt.Errorf(res.StdErr())
	}

	defaultStorageClassFlag := false
	var scList storagev1.StorageClassList
	err := json.Unmarshal([]byte(res.StdOut()), &scList)
	if err != nil {
		return err
	}

	for _, sc := range scList.Items {
		isDefault, ok := sc.Annotations["storageclass.kubernetes.io/is-default-class"]
		if ok && isDefault == "true" {
			defaultStorageClassFlag = true
			break
		}
	}
	if !defaultStorageClassFlag {
		return fmt.Errorf(
			"Default StorageClass not found in the cluster: please provide a StorageClass name or set a default StorageClass.",
		)
	}
	return nil
}

func FindUnassignedIP(subnets []string) (string, error) {
	findError := fmt.Errorf("error: cannot find unassigned IP address")
	res := kubectl.List(kc.ResourceVMIPLease, kc.GetOptions{Output: "jsonpath='{.items[*].metadata.name}'"})
	if !res.WasSuccess() {
		return "", fmt.Errorf("failed to get vmipl: %s", res.StdErr())
	}

	reservedIPs := strings.Split(res.StdOut(), " ")
	for _, rawSubnet := range subnets {
		prefix, err := netip.ParsePrefix(rawSubnet)
		if err != nil {
			return "", fmt.Errorf("failed to parse subnet %s: %w", rawSubnet, err)
		}
		nextAddr := prefix.Addr().Next()
		for {
			nextAddr = nextAddr.Next()
			ip := fmt.Sprintf("ip-%s", strings.ReplaceAll(nextAddr.String(), ".", "-"))
			if slices.Contains(reservedIPs, ip) {
				continue
			}
			if prefix.Contains(nextAddr) {
				return nextAddr.String(), nil
			}
			return "", findError
		}
	}
	return "", findError
}
