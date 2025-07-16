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
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	"github.com/deckhouse/virtualization/tests/e2e/network"
)

const (
	CurlPod           = "curl-helper"
	externalHost      = "https://flant.com"
	nginxActiveStatus = "active"
)

var httpStatusOk = fmt.Sprintf("%v", http.StatusOK)

type PodEntrypoint struct {
	Command string
	Args    []string
}

func RunPod(podName, namespace, image string, entrypoint PodEntrypoint) *executor.CMDResult {
	GinkgoHelper()
	cmd := fmt.Sprintf("run %s --namespace %s --image=%s --labels='name=%s'", podName, namespace, image, podName)
	if entrypoint.Command != "" {
		cmd = fmt.Sprintf("%s --command %s", cmd, entrypoint.Command)
	}
	if entrypoint.Command != "" && len(entrypoint.Args) != 0 {
		rawArgs := strings.Join(entrypoint.Args, " ")
		cmd = fmt.Sprintf("%s -- %s", cmd, rawArgs)
	}
	return kubectl.RawCommand(cmd, ShortWaitDuration)
}

func GenerateServiceURL(svc *corev1.Service, namespace string) string {
	service := fmt.Sprintf("%s.%s.svc:%d", svc.Name, namespace, svc.Spec.Ports[0].Port)
	return service
}

func GetResponseViaPodWithCurl(podName, namespace, host string) *executor.CMDResult {
	cmd := fmt.Sprintf("exec --namespace %s %s -- curl -o - %s", namespace, podName, host)
	return kubectl.RawCommand(cmd, ShortWaitDuration)
}

func CheckCiliumAgents(kubectl kc.Kubectl, vms ...string) {
	GinkgoHelper()
	for _, vm := range vms {
		By(fmt.Sprintf("Cilium agent should be OK's for VM: %s", vm))
		err := network.CheckCilliumAgents(context.Background(), kubectl, vm, conf.Namespace)
		Expect(err).NotTo(HaveOccurred())
	}
}

func CheckExternalConnection(host, httpCode string, vms ...string) {
	GinkgoHelper()
	for _, vm := range vms {
		By(fmt.Sprintf("Response code from %q should be %q for %q", host, httpCode, vm))
		cmd := fmt.Sprintf("curl -o /dev/null -s -w \"%%{http_code}\\n\" %s", host)
		CheckResultSSHCommand(vm, cmd, httpCode)
	}
}

func CheckResultSSHCommand(vmName, cmd, equal string) {
	GinkgoHelper()
	Eventually(func() (string, error) {
		res := d8Virtualization.SSHCommand(vmName, cmd, d8.SSHOptions{
			Namespace:   conf.Namespace,
			Username:    conf.TestData.SSHUser,
			IdenityFile: conf.TestData.Sshkey,
		})
		if res.Error() != nil {
			return "", fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
		}
		return strings.TrimSpace(res.StdOut()), nil
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Equal(equal))
}

var _ = Describe("VM connectivity", ginkgoutil.CommonE2ETestDecorators(), func() {
	var (
		testCaseLabel = map[string]string{"testcase": "vm-connectivity"}
		aObjName      = fmt.Sprintf("%s-vm-connectivity-a", namePrefix)
		bObjName      = fmt.Sprintf("%s-vm-connectivity-b", namePrefix)
		vmA, vmB      virtv2.VirtualMachine
		svcA, svcB    corev1.Service
		err           error

		selectorA string
		selectorB string
	)

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			SaveTestResources(testCaseLabel, CurrentSpecReport().LeafNodeText)
		}
	})

	Context("Preparing the environment", func() {
		It("sets the namespace", func() {
			kustomization := fmt.Sprintf("%s/%s", conf.TestData.Connectivity, "kustomization.yaml")
			ns, err := kustomize.GetNamespace(kustomization)
			Expect(err).NotTo(HaveOccurred(), "%w", err)
			conf.SetNamespace(ns)
		})
	})

	Context("When resources are applied", func() {
		It("result should be succeeded", func() {
			if config.IsReusable() {
				res := kubectl.List(kc.ResourceVM, kc.GetOptions{
					Labels:    testCaseLabel,
					Namespace: conf.Namespace,
					Output:    "jsonpath='{.items[*].metadata.name}'",
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())

				if res.StdOut() != "" {
					return
				}
			}

			res := kubectl.Apply(kc.ApplyOptions{
				Filename:       []string{conf.TestData.Connectivity},
				FilenameOption: kc.Kustomize,
			})
			Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
		})
	})

	Context("When virtual images are applied", func() {
		It("checks VIs phases", func() {
			By(fmt.Sprintf("VIs should be in %s phases", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVI, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual disks are applied", func() {
		It("checks VDs phases", func() {
			By(fmt.Sprintf("VDs should be in %s phase", PhaseReady))
			WaitPhaseByLabel(kc.ResourceVD, PhaseReady, kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context("When virtual machines are applied", func() {
		It("checks VMs phases", func() {
			By("Virtual machine agents should be ready")
			WaitVMAgentReady(kc.WaitOptions{
				Labels:    testCaseLabel,
				Namespace: conf.Namespace,
				Timeout:   MaxWaitTimeout,
			})
		})
	})

	Context(fmt.Sprintf("When run %s", CurlPod), func() {
		It(fmt.Sprintf("status should be in %s phase", PhaseRunning), func() {
			jsonPath := "jsonpath={.status.phase}"
			waitFor := fmt.Sprintf("%s=%s", jsonPath, PhaseRunning)
			res := RunPod(CurlPod, conf.Namespace, conf.HelperImages.CurlImage, PodEntrypoint{
				Command: "sleep",
				Args:    []string{"10000"},
			})
			Expect(res.Error()).NotTo(HaveOccurred())
			WaitResource(kc.ResourcePod, CurlPod, waitFor, ShortWaitDuration)
		})
	})

	Context("When virtual machine agents are ready", func() {
		It("gets VMs and SVCs objects", func() {
			vmA = virtv2.VirtualMachine{}
			err = GetObject(kc.ResourceVM, aObjName, &vmA, kc.GetOptions{
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred())
			vmB = virtv2.VirtualMachine{}
			err = GetObject(kc.ResourceVM, bObjName, &vmB, kc.GetOptions{
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred())

			svcA = corev1.Service{}
			err = GetObject(kc.ResourceService, aObjName, &svcA, kc.GetOptions{
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred())
			svcB = corev1.Service{}
			err = GetObject(kc.ResourceService, bObjName, &svcB, kc.GetOptions{
				Namespace: conf.Namespace,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("check ssh connection via `d8 v` to VMs", func() {
			cmd := "hostname"
			for _, vmName := range []string{vmA.Name, vmB.Name} {
				By(fmt.Sprintf("VirtualMachine %q", vmName))
				CheckResultSSHCommand(vmName, cmd, vmName)
			}
		})

		It("checks VMs connection to external network", func() {
			CheckCiliumAgents(kubectl, vmA.Name, vmB.Name)
			CheckExternalConnection(externalHost, httpStatusOk, vmA.Name, vmB.Name)
		})

		It("check nginx status via `d8 v` on VMs", func() {
			cmd := "systemctl is-active nginx.service"
			for _, vmName := range []string{vmA.Name, vmB.Name} {
				By(fmt.Sprintf("VirtualMachine %q", vmName))
				CheckResultSSHCommand(vmName, cmd, nginxActiveStatus)
			}
		})

		It(fmt.Sprintf("gets page from service %s", aObjName), func() {
			service := GenerateServiceURL(&svcA, conf.Namespace)
			Eventually(func() (string, error) {
				res := GetResponseViaPodWithCurl(CurlPod, conf.Namespace, service)
				if res.Error() != nil {
					return "", fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
				}
				return strings.TrimSpace(res.StdOut()), nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(ContainSubstring(vmA.Name))
		})

		It(fmt.Sprintf("gets page from service %s", bObjName), func() {
			service := GenerateServiceURL(&svcB, conf.Namespace)
			Eventually(func() (string, error) {
				res := GetResponseViaPodWithCurl(CurlPod, conf.Namespace, service)
				if res.Error() != nil {
					return "", fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
				}
				return strings.TrimSpace(res.StdOut()), nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(ContainSubstring(vmB.Name))
		})

		It(fmt.Sprintf("changes selector in service %s with selector from service %s", aObjName, bObjName), func() {
			selectorA = svcA.Spec.Selector["service"]
			selectorB = svcB.Spec.Selector["service"]

			PatchResource(kc.ResourceService, svcA.Name, []*kc.JSONPatch{
				{
					Op:    "replace",
					Path:  "/spec/selector/service",
					Value: selectorB,
				},
			})
		})

		It(fmt.Sprintf("checks selector in service %s", aObjName), func() {
			By(fmt.Sprintf("Selector should be %q", selectorB))
			output := "jsonpath={.spec.selector.service}"
			CheckField(kc.ResourceService, svcA.Name, output, selectorB)
		})

		It(fmt.Sprintf("gets page from service %s", aObjName), func() {
			By(fmt.Sprintf("Response should be from virtual machine %q", vmB.Name))
			service := GenerateServiceURL(&svcA, conf.Namespace)
			Eventually(func() (string, error) {
				res := GetResponseViaPodWithCurl(CurlPod, conf.Namespace, service)
				if res.Error() != nil {
					return "", fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
				}
				return strings.TrimSpace(res.StdOut()), nil
			}).WithTimeout(Timeout).WithPolling(Interval).Should(ContainSubstring(vmB.Name))
		})

		It(fmt.Sprintf("changes back selector in service %s", aObjName), func() {
			PatchResource(kc.ResourceService, svcA.Name, []*kc.JSONPatch{
				{
					Op:    "replace",
					Path:  "/spec/selector/service",
					Value: selectorA,
				},
			})
		})

		It(fmt.Sprintf("checks selector in service %s", aObjName), func() {
			By(fmt.Sprintf("Selector should be %q", selectorA))
			output := "jsonpath={.spec.selector.service}"
			CheckField(kc.ResourceService, svcA.Name, output, selectorA)
		})
	})

	Context("When test is completed", func() {
		It("deletes test case resources", func() {
			resourcesToDelete := ResourcesToDelete{
				AdditionalResources: []AdditionalResource{
					{
						Resource: kc.ResourcePod,
						Labels:   map[string]string{"name": CurlPod},
					},
				},
			}

			if config.IsCleanUpNeeded() {
				resourcesToDelete.KustomizationDir = conf.TestData.Connectivity
			}

			DeleteTestCaseResources(resourcesToDelete)
		})
	})
})
