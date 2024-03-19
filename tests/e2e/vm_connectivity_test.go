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

const (
	CurlPod = "curl-helper"
)

type Service struct {
	ApiVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
		Labels    struct {
			vm string `yaml:"vm,omitempty"`
		} `yaml:"labels,omitempty"`
	} `yaml:"metadata"`
	Spec struct {
		Selector struct {
			Service string `yaml:"service,omitempty"`
		} `yaml:"selector"`
		Ports []struct {
			Port       int `yaml:"port"`
			TargetPort int `yaml:"targetPort,omitempty"`
			NodePort   int `yaml:"nodePort,omitempty"`
		} `yaml:"ports"`
	} `yaml:"spec"`
}

func getSVC(manifestPath string) (*Service, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		log.Fatalf("Error read file: %v", err)
	}

	var service Service
	err = yaml.Unmarshal(data, &service)
	if err != nil {
		log.Fatalf("Error parsing yaml: %v", err)
	}

	return &service, err
}

var _ = Describe("VM connectivity", Ordered, ContinueOnFailure, func() {
	imageManifest := vmPath("image.yaml")
	vmOneManifest := vmPath("connectivity/vm1_connectivity.yaml")
	vmTwoManifest := vmPath("connectivity/vm2_connectivity.yaml")
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
			Expect(res.Error()).
				NotTo(HaveOccurred(), "check ssh failed for %s/%s.\n%s\n%s", conf.Namespace, vmName, res.StdErr(),
					vmPath("sshkeys/id_ed"))
			Expect(strings.TrimSpace(res.StdOut())).To(Equal(equal))
		}

		svc1, err := getSVC(vmSvcOne)
		Expect(err).NotTo(HaveOccurred())
		svc2, err := getSVC(vmSvcTwo)
		Expect(err).NotTo(HaveOccurred())

		curlSVC := func(vmName string, serv *Service, namespace string) *executor.CMDResult {
			GinkgoHelper()
			svc := *serv

			subCurlCMD := fmt.Sprintf("%s %s.%s.svc:%d", "curl -o -", svc.Metadata.Name, svc.Metadata.Namespace,
				svc.Spec.Ports[0].Port)
			subCMD := fmt.Sprintf("run -n %s --restart=Never -i --tty %s-%s --image=%s -- %s",
				namespace, CurlPod, vmName, conf.HelperImages.CurlImage, subCurlCMD)
			fmt.Printf("%s <-- subCurlCMD", subCMD)
			return kubectl.RawCommand(subCMD, ShortWaitDuration)
		}

		deletePodHelper := func(vmName, namespace string) *executor.CMDResult {
			GinkgoHelper()
			subCMD := fmt.Sprintf("-n %s delete po %s-%s", namespace, CurlPod, vmName)
			return kubectl.RawCommand(subCMD, ShortWaitDuration)
		}

		ItApplyFromFile(vmOneIPClaim)
		ItApplyFromFile(vmOneManifest)
		ItApplyFromFile(vmSvcOne)
		ItApplyFromFile(vmTwoIPClaim)
		ItApplyFromFile(vmTwoManifest)
		ItApplyFromFile(vmSvcTwo)

		vmOne, err := GetVMFromManifest(vmOneManifest)
		Expect(err).NotTo(HaveOccurred(), "%s", err)
		vmTwo, err := GetVMFromManifest(vmTwoManifest)
		Expect(err).NotTo(HaveOccurred(), "%s", err)

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

		It(fmt.Sprintf("Get nginx page from %s through service %s", vmOne.Name, svc1.Metadata.Name), func() {
			res := curlSVC(vmOne.Name, svc1, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
			Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vmOne.Name))
		})

		It(fmt.Sprintf("Delete pod helper for %s", vmOne.Name), func() {
			res := deletePodHelper(vmOne.Name, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
		})

		It(fmt.Sprintf("Get nginx page from %s through service %s", vmTwo.Name, svc2.Metadata.Name), func() {
			res := curlSVC(vmTwo.Name, svc2, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
			Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vmTwo.Name))
		})

		It(fmt.Sprintf("Delete pod helper for %s", vmTwo.Name), func() {
			res := deletePodHelper(vmTwo.Name, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
		})

		It(fmt.Sprintf("Change selector on %s", svc1.Metadata.Name), func() {
			PatchSvcSelector := func(name, label string) {
				GinkgoHelper()
				PatchResource(kc.ResourceService, name, &kc.JsonPatch{
					Op:    "replace",
					Path:  "/spec/selector/service",
					Value: label,
				})
			}

			PatchSvcSelector(svc1.Metadata.Name, svc2.Spec.Selector.Service)
		})
		It(fmt.Sprintf("Check selector on %s, must be %s", svc1.Metadata.Name, svc2.Spec.Selector.Service), func() {
			GetSvcLabel := func(name, label string) {
				GinkgoHelper()
				output := "jsonpath={.spec.selector.service}"
				CheckField(kc.ResourceService, name, output, label)
			}
			GetSvcLabel(svc1.Metadata.Name, svc2.Spec.Selector.Service)
		})

		It(fmt.Sprintf("Get nginx page from %s and expect %s hostname", vmOne.Name, vmTwo.Name), func() {
			res := curlSVC(vmOne.Name, svc1, conf.Namespace)
			Expect(res.Error()).NotTo(HaveOccurred(), "%s", res.StdErr())
			Expect(strings.TrimSpace(res.StdOut())).Should(ContainSubstring(vmTwo.Name))
		})
	})
})
