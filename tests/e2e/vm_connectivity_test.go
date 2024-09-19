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
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	CurlPod           = "curl-helper"
	Timeout           = 90 * time.Second
	Interval          = 5 * time.Second
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
	cmd := fmt.Sprintf("run %s --namespace %s --image=%s", podName, namespace, image)
	if entrypoint.Command != "" {
		cmd = fmt.Sprintf("%s --command %s", cmd, entrypoint.Command)
	}
	if entrypoint.Command != "" && len(entrypoint.Args) != 0 {
		rawArgs := strings.Join(entrypoint.Args, " ")
		cmd = fmt.Sprintf("%s -- %s", cmd, rawArgs)
	}
	return kubectl.RawCommand(cmd, ShortWaitDuration)
}

func GenerateServiceUrl(svc *corev1.Service, namespace string) string {
	service := fmt.Sprintf("%s.%s.svc:%d", svc.Name, namespace, svc.Spec.Ports[0].Port)
	return service
}

func GetResponseViaPodWithCurl(podName, namespace, host string) *executor.CMDResult {
	cmd := fmt.Sprintf("exec --namespace %s %s -- curl -o - %s", namespace, podName, host)
	return kubectl.RawCommand(cmd, ShortWaitDuration)
}

func CheckExternalConnection(sshKeyPath, host, httpCode string, vms ...string) {
	GinkgoHelper()
	for _, vm := range vms {
		By(fmt.Sprintf("Response code from %s should be is %s for %s", host, httpCode, vm))
		cmd := fmt.Sprintf("curl -o /dev/null -s -w \"%%{http_code}\\n\" %s", host)
		CheckResultSshCommand(vm, cmd, httpCode, sshKeyPath)
	}
}

func getSVC(manifestPath string) (*corev1.Service, error) {
	GinkgoHelper()
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("Error read file: %v", err)
	}

	service := corev1.Service{}
	err = yaml.Unmarshal(data, &service)
	if err != nil {
		log.Fatalf("Error parsing yaml: %v", err)
	}

	return &service, err
}

func CheckResultSshCommand(vmName, cmd, equal, key string) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		res := d8Virtualization.SshCommand(vmName, cmd, d8.SshOptions{
			Namespace:   conf.Namespace,
			Username:    "cloud",
			IdenityFile: key,
		})
		g.Expect(res.Error()).NotTo(HaveOccurred(), "check ssh failed for %s/%s.\n%s\n%s", conf.Namespace, vmName, res.StdErr(), key)
		g.Expect(strings.TrimSpace(res.StdOut())).To(Equal(equal))
	}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
}

var _ = Describe("VM connectivity", Ordered, ContinueOnFailure, func() {
	Context("Resources", func() {
		When("Resources applied", func() {
			It("Result must have no error", func() {
				res := kubectl.Kustomize(conf.TestData.Connectivity, kc.KustomizeOptions{})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
			})
		})
	})

	Context("Virtual disks", func() {
		When("VD applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseReady), func() {
				jsonPath := "jsonpath={.status.phase}"
				waitFor := fmt.Sprintf("%s=%s", jsonPath, PhaseReady)
				vd1Name := fmt.Sprintf("%s-vm1-vd-from-vi", namePrefix)
				vd2Name := fmt.Sprintf("%s-vm2-vd-from-vi", namePrefix)
				WaitResource(kc.ResourceVD, vd1Name, waitFor, LongWaitDuration)
				WaitResource(kc.ResourceVD, vd2Name, waitFor, LongWaitDuration)
			})
		})
	})

	Context("Virtual machines", func() {
		When("VM applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseRunning), func() {
				jsonPath := "jsonpath={.status.phase}"
				waitFor := fmt.Sprintf("%s=%s", jsonPath, PhaseRunning)
				vm1Name := fmt.Sprintf("%s-vm1", namePrefix)
				vm2Name := fmt.Sprintf("%s-vm2", namePrefix)
				WaitResource(kc.ResourceVM, vm1Name, waitFor, LongWaitDuration)
				WaitResource(kc.ResourceVM, vm2Name, waitFor, LongWaitDuration)
			})
		})
	})

	Context("Connectivity test", func() {
		vm1Name := fmt.Sprintf("%s-vm1", namePrefix)
		vm2Name := fmt.Sprintf("%s-vm2", namePrefix)

		svc1Path := fmt.Sprintf("%s/resources/vm1-svc.yaml", conf.TestData.Connectivity)
		svc2Path := fmt.Sprintf("%s/resources/vm2-svc.yaml", conf.TestData.Connectivity)

		sshKeyPath := fmt.Sprintf("%s/id_ed", conf.TestData.Sshkeys)
		ChmodFile(sshKeyPath, 0o600)

		svc1, err := getSVC(svc1Path)
		Expect(err).NotTo(HaveOccurred(), err)
		svc2, err := getSVC(svc2Path)
		Expect(err).NotTo(HaveOccurred(), err)

		svc1.Name = fmt.Sprintf("%s-%s", namePrefix, svc1.Name)
		svc2.Name = fmt.Sprintf("%s-%s", namePrefix, svc2.Name)

		When(fmt.Sprintf("Run %s", CurlPod), func() {
			It(fmt.Sprintf("Pod status should be in %s phase", PhaseRunning), func() {
				jsonPath := "jsonpath={.status.phase}"
				waitFor := fmt.Sprintf("%s=%s", jsonPath, PhaseRunning)
				res := RunPod(CurlPod, conf.Namespace, conf.HelperImages.CurlImage, PodEntrypoint{
					Command: "sleep",
					Args:    []string{"10000"},
				})
				Expect(res.Error()).NotTo(HaveOccurred(), res.StdErr())
				WaitResource(kc.ResourcePod, CurlPod, waitFor, ShortWaitDuration)
			})
		})

		When("Virtual machine is running", func() {
			It("Virtual machine must have to be connected to external network", func() {
				CheckExternalConnection(sshKeyPath, externalHost, httpStatusOk, vm1Name)
			})

			It(fmt.Sprintf("Check ssh via 'd8 v' on VM %s", vm1Name), func() {
				cmd := "hostname"
				CheckResultSshCommand(vm1Name, cmd, vm1Name, sshKeyPath)
			})

			It(fmt.Sprintf("Check nginx via 'd8 v' on VM %s", vm1Name), func() {
				cmd := "systemctl is-active nginx.service"
				CheckResultSshCommand(vm1Name, cmd, nginxActiveStatus, sshKeyPath)
			})
		})

		It(fmt.Sprintf("Get nginx page from %s through service %s", vm1Name, svc1.Name), func() {
			service := GenerateServiceUrl(svc1, conf.Namespace)
			Eventually(func(g Gomega) {
				res := GetResponseViaPodWithCurl(CurlPod, conf.Namespace, service)
				g.Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
				g.Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vm1Name))
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
		})

		It(fmt.Sprintf("Get nginx page from %s through service %s", vm2Name, svc2.Name), func() {
			service := GenerateServiceUrl(svc2, conf.Namespace)
			Eventually(func(g Gomega) {
				res := GetResponseViaPodWithCurl(CurlPod, conf.Namespace, service)
				g.Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
				g.Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vm2Name))
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
		})

		It(fmt.Sprintf("Change selector on %s", svc1.Name), func() {
			PatchSvcSelector := func(name, label string) {
				GinkgoHelper()
				PatchResource(kc.ResourceService, name, &kc.JsonPatch{
					Op:    "replace",
					Path:  "/spec/selector/service",
					Value: label,
				})
			}

			PatchSvcSelector(svc1.Name, svc2.Spec.Selector["service"])
		})

		It(fmt.Sprintf("Check selector on %s, must be %s", svc1.Name, svc2.Spec.Selector["service"]), func() {
			GetSvcLabel := func(name, label string) {
				GinkgoHelper()
				output := "jsonpath={.spec.selector.service}"
				CheckField(kc.ResourceService, name, output, label)
			}
			GetSvcLabel(svc1.Name, svc2.Spec.Selector["service"])
		})

		It(fmt.Sprintf("Get nginx page from %s and expect %s hostname", vm1Name, vm2Name), func() {
			service := GenerateServiceUrl(svc1, conf.Namespace)
			Eventually(func(g Gomega) {
				res := GetResponseViaPodWithCurl(CurlPod, conf.Namespace, service)
				g.Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
				g.Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vm2Name))
			}).WithTimeout(Timeout).WithPolling(Interval).Should(Succeed())
		})
	})
})
