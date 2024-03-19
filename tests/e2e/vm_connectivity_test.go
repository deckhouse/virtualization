package e2e

import (
	"fmt"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	virt "github.com/deckhouse/virtualization/tests/e2e/virtctl"
)

type Service struct {
	ApiVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		Ports []struct {
			Port       int `yaml:"port"`
			TargetPort int `yaml:"targetPort,omitempty"`
			NodePort   int `yaml:"nodePort,omitempty"`
		} `yaml:"ports"`
	} `yaml:"spec"`
}

func getSVC(manifestPath string) *Service {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("Error read file: %v", err)
	}

	var service Service
	err = yaml.Unmarshal(data, &service)
	if err != nil {
		log.Fatalf("Error parsing yaml: %v", err)
	}

	return &service
}

var _ = Describe("VM connectivity", Ordered, ContinueOnFailure, func() {
	imageManifest := vmPath("image.yaml")
	vmOneManifest := vmPath("connectivity/vm1_connectivity_service.yaml")
	vmTwoManifest := vmPath("connectivity/vm2_connectivity_service.yaml")
	vmOneIPClaim := vmPath("connectivity/vm1_ipclaim.yaml")
	vmTwoIPClaim := vmPath("connectivity/vm2_ipclaim.yaml")
	vmSvcOne := vmPath("connectivity/vm1_svc.yaml")
	vmSvcTwo := vmPath("connectivity/vm2_svc.yaml")

	waitVmStatus := func(name, phase string) {
		GinkgoHelper()
		WaitResource(kc.ResourceVM, name, "jsonpath={.status.phase}="+phase, LongWaitDuration)
	}

	BeforeAll(func() {
		By("Apply image for vms")
		ApplyFromFile(imageManifest)
		WaitFromFile(imageManifest, PhaseReady, LongWaitDuration)
	})

	AfterAll(func() {
		By("Delete all manifests")
		files := make([]string, 0)
		err := filepath.Walk(
			conf.VM.TestDataDir, func(path string, info fs.FileInfo, err error) error {
				if err == nil && strings.HasSuffix(info.Name(), "yaml") {
					files = append(files, path)
				}
				return nil
			},
		)
		if err != nil || len(files) == 0 {
			kubectl.Delete(imageManifest, kc.DeleteOptions{})
			kubectl.Delete(conf.VM.TestDataDir, kc.DeleteOptions{})
		} else {
			for _, f := range files {
				kubectl.Delete(f, kc.DeleteOptions{})
			}
		}
	})

	Context("Connectivity test", func() {
		CheckResultSshCommand := func(vmName, command, equal string) {
			GinkgoHelper()
			res := virtctl.SshCommand(vmName, command, virt.SshOptions{
				Namespace:   conf.Namespace,
				Username:    "cloud",
				IdenityFile: vmPath("sshkeys/id_ed"),
			})
			Expect(res.Error()).To(
				BeNil(), "check ssh failed for %s/%s.\n%s\n%s", conf.Namespace, vmName, res.StdErr(),
				vmPath("sshkeys/id_ed"))
			Expect(strings.TrimSpace(res.StdOut())).To(Equal(equal))
		}

		curlSVC := func(vmName, svcName, namespace string) *executor.CMDResult {
			GinkgoHelper()
			svc := getSVC(svcName)

			subCurlCMD := fmt.Sprintf("%s %s.%s.svc:%d", "curl -o -", svc.Metadata.Name, svc.Metadata.Namespace,
				svc.Spec.Ports[0].Port)
			subCMD := fmt.Sprintf("run -n %s --restart=Never -i --tty %s-%s --image=%s -- %s",
				namespace, UploadHelpPod, vmName, conf.Disks.UploadHelperImage, subCurlCMD)
			return kubectl.RawCommand(subCMD, ShortWaitDuration)
		}

		deletePodHelper := func(vmName, namespace string) *executor.CMDResult {
			GinkgoHelper()
			subCMD := fmt.Sprintf("-n %s delete po %s-%s", namespace, UploadHelpPod, vmName)
			return kubectl.RawCommand(subCMD, ShortWaitDuration)
		}

		ItApplyFromFile(vmOneIPClaim)
		ItApplyFromFile(vmOneManifest)
		ItApplyFromFile(vmSvcOne)
		ItApplyFromFile(vmTwoIPClaim)
		ItApplyFromFile(vmTwoManifest)
		ItApplyFromFile(vmSvcTwo)

		vmOne, err := GetVMFromManifest(vmOneManifest)
		Expect(err).To(BeNil())
		vmTwo, err := GetVMFromManifest(vmTwoManifest)
		Expect(err).To(BeNil())

		It(fmt.Sprintf("Wait %s running", vmOne.Name), func() {
			waitVmStatus(vmOne.Name, VMStatusRunning)
		})
		It(fmt.Sprintf("Wait %s running", vmTwo.Name), func() {
			waitVmStatus(vmTwo.Name, VMStatusRunning)
		})
		It("Wait 30 sec for sshd started", func() {
			time.Sleep(30 * time.Second)
		})

		It("Check ssh via virtctl", func() {
			command := "hostname"
			CheckResultSshCommand(vmOne.Name, command, vmOne.Name)
		})
		It("Check external site from VM", func() {
			command := "curl -o /dev/null -s -w \"%{http_code}\\n\" https://flant.com"
			httpCode := "200"
			CheckResultSshCommand(vmOne.Name, command, httpCode)
		})

		It(fmt.Sprintf("Get nginx page through service from %s", vmOne.Name), func() {
			res := curlSVC(vmOne.Name, vmSvcOne, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
			Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vmOne.Name))
		})

		It(fmt.Sprintf("Delete pod helper for %s", vmOne.Name), func() {
			res := deletePodHelper(vmOne.Name, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
		})

		It(fmt.Sprintf("Get nginx page through service %s", vmTwo.Name), func() {
			res := curlSVC(vmTwo.Name, vmSvcTwo, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
			Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vmTwo.Name))
		})

		It(fmt.Sprintf("Delete pod helper for %s", vmTwo.Name), func() {
			res := deletePodHelper(vmTwo.Name, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
		})

		It(fmt.Sprintf("Change label on %s", "vm1-service"), func() {
			PatchSvcSelector := func(name, label string) {
				GinkgoHelper()
				PatchResource(kc.ResourceService, name, &kc.JsonPatch{
					Op:    "replace",
					Path:  "/spec/selector/service",
					Value: label,
				})
			}

			PatchSvcSelector("vm1-service", "v2")
		})
		It(fmt.Sprintf("Check label %s", "vm1-service"), func() {
			GetSvcLabel := func(name, label string) {
				GinkgoHelper()
				output := "jsonpath={.spec.selector.service}"
				CheckField(kc.ResourceService, name, output, label)
			}
			GetSvcLabel("vm1-service", "v2")
		})

		It(fmt.Sprintf("Get nginx page from %s and expect %s hostname", vmOne.Name, vmTwo.Name), func() {
			res := curlSVC(vmOne.Name, vmSvcOne, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
			Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vmTwo.Name))
		})
	})
})
