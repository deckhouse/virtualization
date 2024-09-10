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
	"os"
	"strings"
	"time"

	"github.com/deckhouse/virtualization/tests/e2e/executor"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	corev1 "k8s.io/api/core/v1"
)

const (
	CurlPod = "curl-helper"
)

func curlSVC(vmName string, serv *corev1.Service, namespace string) *executor.CMDResult {
	GinkgoHelper()
	svc := *serv

	subCurlCMD := fmt.Sprintf("%s %s.%s.svc:%d", "curl -o -", svc.Name, namespace,
		svc.Spec.Ports[0].Port)
	subCMD := fmt.Sprintf("run -n %s --restart=Never -i --tty %s-%s --image=%s -- %s",
		namespace, CurlPod, vmName, conf.HelperImages.CurlImage, subCurlCMD)
	fmt.Println(subCMD, "<---- subCurlCMD")
	return kubectl.RawCommand(subCMD, ShortWaitDuration)
}

func deletePodHelper(vmName, namespace string) *executor.CMDResult {
	GinkgoHelper()
	subCMD := fmt.Sprintf("-n %s delete po %s-%s", namespace, CurlPod, vmName)
	return kubectl.RawCommand(subCMD, ShortWaitDuration)
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

func CheckResultSshCommand(vmName, command, equal, key string) {
	GinkgoHelper()
	res := d8Virtualization.SshCommand(vmName, command, d8.SshOptions{
		Namespace:   conf.Namespace,
		Username:    "cloud",
		IdenityFile: key,
	})
	Expect(res.Error()).
		NotTo(HaveOccurred(), "check ssh failed for %s/%s.\n%s\n%s", conf.Namespace, vmName, res.StdErr(), key)
	Expect(strings.TrimSpace(res.StdOut())).To(Equal(equal))
}

var _ = Describe("VM connectivity", Ordered, ContinueOnFailure, func() {
	BeforeAll(func() {
		sshKeyPath := fmt.Sprintf("%s/sshkeys/id_ed", conf.Connectivity)
		ChmodFile(sshKeyPath, 0600)
	})

	Context("Resources", func() {
		When("Resources applied", func() {
			It("Result must have no error", func() {
				res := kubectl.Kustomize(conf.Connectivity, kc.KustomizeOptions{})
				Expect(res.WasSuccess()).To(Equal(true), res.StdErr())
			})
		})
	})

	Context("Virtual machines", func() {
		When("VM applied", func() {
			It(fmt.Sprintf("Phase should be %s", PhaseRunning), func() {
				CheckPhase("vm", PhaseRunning)
			})
		})
	})

	Context("Connectivity test", func() {
		var (
			vm1Name = fmt.Sprintf("%s-vm1", namePrefix)
			vm2Name = fmt.Sprintf("%s-vm2", namePrefix)

			svc1Path = fmt.Sprintf("%s/resources/vm1-svc.yaml", conf.Connectivity)
			svc2Path = fmt.Sprintf("%s/resources/vm2-svc.yaml", conf.Connectivity)

			sshKeyPath = fmt.Sprintf("%s/sshkeys/id_ed", conf.Connectivity)
		)

		svc1, err := getSVC(svc1Path)
		Expect(err).NotTo(HaveOccurred())
		svc2, err := getSVC(svc2Path)
		Expect(err).NotTo(HaveOccurred())

		svc1.Name = fmt.Sprintf("%s-%s", namePrefix, svc1.Name)
		svc2.Name = fmt.Sprintf("%s-%s", namePrefix, svc2.Name)

		It("Wait 60 sec for sshd started", func() {
			time.Sleep(60 * time.Second)
		})

		It(fmt.Sprintf("Check ssh via 'd8 v' on VM %s", vm1Name), func() {
			command := "hostname"
			CheckResultSshCommand(vm1Name, command, vm1Name, sshKeyPath)
		})

		It(fmt.Sprintf("Curl https://flant.com site from %s", vm1Name), func() {
			command := "curl -o /dev/null -s -w \"%{http_code}\\n\" https://flant.com"
			httpCode := "200"
			CheckResultSshCommand(vm1Name, command, httpCode, sshKeyPath)
		})

		It(fmt.Sprintf("Get nginx page from %s through service %s", vm1Name, svc1.Name), func() {
			res := curlSVC(vm1Name, svc1, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
			Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vm1Name))
		})

		It(fmt.Sprintf("Delete pod helper for %s", vm1Name), func() {
			res := deletePodHelper(vm1Name, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
		})

		It(fmt.Sprintf("Get nginx page from %s through service %s", vm2Name, svc2.Name), func() {
			res := curlSVC(vm2Name, svc2, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
			Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vm2Name))
		})

		It(fmt.Sprintf("Delete pod helper for %s", vm2Name), func() {
			res := deletePodHelper(vm2Name, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
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
			res := curlSVC(vm1Name, svc1, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
			Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vm2Name))
		})
	})
})
