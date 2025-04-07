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
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/tests/e2e/config"
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	el "github.com/deckhouse/virtualization/tests/e2e/errlogger"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	gt "github.com/deckhouse/virtualization/tests/e2e/git"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	Interval                  = 5 * time.Second
	ShortTimeout              = 30 * time.Second
	Timeout                   = 90 * time.Second
	ShortWaitDuration         = 60 * time.Second
	LongWaitDuration          = 300 * time.Second
	MaxWaitTimeout            = 600 * time.Second
	PhaseAttached             = "Attached"
	PhaseReady                = "Ready"
	PhaseBound                = "Bound"
	PhaseCompleted            = "Completed"
	PhasePending              = "Pending"
	PhaseReleased             = "Released"
	PhaseSucceeded            = "Succeeded"
	PhaseRunning              = "Running"
	PhaseWaitForUserUpload    = "WaitForUserUpload"
	PhaseWaitForFirstConsumer = "WaitForFirstConsumer"
	VirtualizationController  = "virtualization-controller"
	VirtualizationNamespace   = "d8-virtualization"
)

var (
	conf                         *config.Config
	mc                           *config.ModuleConfig
	kustomize                    *config.Kustomize
	kubectl                      kc.Kubectl
	d8Virtualization             d8.D8Virtualization
	git                          gt.Git
	namePrefix                   string
	defaultStorageClass          *storagev1.StorageClass
	phaseByVolumeBindingMode     string
	logStreamByV12nControllerPod map[string]*el.LogStream
)

func init() {
	err := config.CheckReusableOption()
	if err != nil {
		log.Fatal(err)
	}
	err = config.CheckWithPostCleanUpOption()
	if err != nil {
		log.Fatal(err)
	}
	if conf, err = config.GetConfig(); err != nil {
		log.Fatal(err)
	}
	if mc, err = config.GetModuleConfig(); err != nil {
		log.Fatal(err)
	}
	if kubectl, err = kc.NewKubectl(kc.KubectlConf(conf.ClusterTransport)); err != nil {
		log.Fatal(err)
	}
	if d8Virtualization, err = d8.NewD8Virtualization(d8.D8VirtualizationConf(conf.ClusterTransport)); err != nil {
		log.Fatal(err)
	}
	if git, err = gt.NewGit(); err != nil {
		log.Fatal(err)
	}
	if defaultStorageClass, err = GetDefaultStorageClass(); err != nil {
		log.Fatal(err)
	}
	if namePrefix, err = config.GetNamePrefix(); err != nil {
		log.Fatal(err)
	}
	ChmodFile(conf.TestData.Sshkey, 0o600)
	conf.Namespace = fmt.Sprintf("%s-%s", namePrefix, conf.Namespace)
	conf.StorageClass.VolumeBindingMode = *defaultStorageClass.VolumeBindingMode
	phaseByVolumeBindingMode = GetPhaseByVolumeBindingMode(conf)
	// TODO: get kustomization files from testdata directory when all tests will be refactored
	kustomizationFiles := []string{
		fmt.Sprintf("%s/%s", conf.TestData.AffinityToleration, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.ComplexTest, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.Connectivity, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.DiskResizing, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.ImageHotplug, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.SizingPolicy, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.ImporterNetworkPolicy, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.VdSnapshots, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.ImagesCreation, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.VmConfiguration, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.VmLabelAnnotation, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.VmMigration, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.VmDiskAttachment, "kustomization.yaml"),
	}
	for _, filePath := range kustomizationFiles {
		if err = kustomize.SetParams(filePath, conf.Namespace, namePrefix); err != nil {
			log.Fatal(err)
		}
	}

	if !config.IsReusable() {
		errs := Cleanup()
		if len(errs) != 0 {
			log.Fatal(errs)
		}
	} else {
		log.Println("Run test in REUSABLE mode")
	}
}

func TestTests(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting test suite\n")
	RunSpecs(t, "Tests")

	if (ginkgoutil.FailureBehaviourEnvSwitcher{}).IsStopOnFailure() || !config.IsCleanUpNeeded() {
		return
	}

	err := Cleanup()
	if len(err) != 0 {
		log.Fatal(err)
	}
}

var _ = BeforeSuite(func() {
	pods := &corev1.PodList{}
	err := GetObjects(kc.ResourcePod, pods, kc.GetOptions{
		Labels:    map[string]string{"app": VirtualizationController},
		Namespace: VirtualizationNamespace,
	})
	Expect(err).NotTo(HaveOccurred(), "failed to obtain the `Virtualization-controller` pods")
	Expect(pods.Items).ShouldNot(BeEmpty())

	logStreamByV12nControllerPod = make(map[string]*el.LogStream, len(pods.Items))
	StartV12nControllerLogStream(logStreamByV12nControllerPod)
})

var _ = AfterSuite(func() {
	StopV12nControllerLogStream(logStreamByV12nControllerPod)
})

func Cleanup() []error {
	cleanupErrs := make([]error, 0)

	res := kubectl.Delete(kc.DeleteOptions{
		IgnoreNotFound: true,
		Labels:         map[string]string{"id": namePrefix},
		Resource:       kc.ResourceProject,
	})
	if res.Error() != nil {
		cleanupErrs = append(
			cleanupErrs, fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr()),
		)
	}

	testCases, err := conf.GetTestCases()
	if err != nil {
		cleanupErrs = append(cleanupErrs, err)
		return cleanupErrs
	}

	for _, tc := range testCases {
		kustomizeFilePath := fmt.Sprintf("%s/kustomization.yaml", tc)
		namespace, err := kustomize.GetNamespace(kustomizeFilePath)
		if err != nil {
			cleanupErrs = append(
				cleanupErrs, fmt.Errorf("cannot cleanup namespace %q: %w", namespace, err),
			)
			continue
		}
		res := kubectl.Delete(kc.DeleteOptions{
			Filename:       []string{namespace},
			IgnoreNotFound: true,
			Resource:       kc.ResourceNamespace,
		})
		if res.Error() != nil {
			cleanupErrs = append(
				cleanupErrs, fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr()),
			)
			continue
		}
	}
	
	for _, r := range conf.CleanupResources {
		res = kubectl.Delete(kc.DeleteOptions{
			IgnoreNotFound: true,
			Labels:         map[string]string{"id": namePrefix},
			Resource: 		kc.Resource(r),
		})
		if res.Error() != nil {
			cleanupErrs = append(
				cleanupErrs, fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr()),
			)
			continue
		}
	}

	return cleanupErrs
}

// This function is used to detect `v12n-controller` errors while the test suite is running.
func StartV12nControllerLogStream(logStreamByPod map[string]*el.LogStream) {
	startTime := time.Now()

	pods := &corev1.PodList{}
	err := GetObjects(kc.ResourcePod, pods, kc.GetOptions{
		Labels:    map[string]string{"app": VirtualizationController},
		Namespace: VirtualizationNamespace,
	})
	Expect(err).NotTo(HaveOccurred(), "failed to obtain the `Virtualization-controller` pods")
	Expect(pods.Items).ShouldNot(BeEmpty())

	for _, p := range pods.Items {
		logStreamCmd, logStreamCancel := kubectl.LogStream(
			p.Name,
			kc.LogOptions{
				Container: VirtualizationController,
				Namespace: VirtualizationNamespace,
				Follow:    true,
			},
		)

		var containerStartedAt v1.Time
		for _, s := range p.Status.ContainerStatuses {
			if s.Name == VirtualizationController {
				containerStartedAt = s.State.Running.StartedAt
				Expect(containerStartedAt).ShouldNot(BeNil())
			}
		}

		logStreamByPod[p.Name] = &el.LogStream{
			Cancel:             logStreamCancel,
			ContainerStartedAt: containerStartedAt,
			LogStreamCmd:       logStreamCmd,
			LogStreamWaitGroup: &sync.WaitGroup{},
			PodName:            p.Name,
		}
	}

	for _, logStream := range logStreamByPod {
		logStream.ConnectStderr()
		logStream.ConnectStdout()
		logStream.Start()

		logStream.LogStreamWaitGroup.Add(1)
		go logStream.ParseStderr()
		logStream.LogStreamWaitGroup.Add(1)
		go logStream.ParseStdout(conf.LogFilter, conf.RegexpLogFilter, startTime)
	}
}

func StopV12nControllerLogStream(logStreamByPod map[string]*el.LogStream) {
	for _, logStream := range logStreamByPod {
		logStream.Cancel()
		logStream.LogStreamWaitGroup.Add(1)
		go func() {
			defer GinkgoRecover()
			defer logStream.LogStreamWaitGroup.Done()
			warn, err := logStream.WaitCmd()
			Expect(err).NotTo(HaveOccurred())
			if warn != "" {
				_, err := GinkgoWriter.Write([]byte(warn))
				Expect(err).NotTo(HaveOccurred())
			}
		}()
		logStream.LogStreamWaitGroup.Wait()
	}
	for pod, logStream := range logStreamByPod {
		isRestarted, err := IsContainerRestarted(
			pod,
			VirtualizationController,
			VirtualizationNamespace,
			logStream.ContainerStartedAt,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(isRestarted).ShouldNot(BeTrue(),
			"the container %q was restarted: %s",
			VirtualizationController,
			pod,
		)
	}
}
