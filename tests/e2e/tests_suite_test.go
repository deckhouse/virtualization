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
	"reflect"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
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
	kubeClient                   kubernetes.Interface
	virtClient                   kubeclient.Client
	kubectl                      kc.Kubectl
	crClient                     client.Client
	d8Virtualization             d8.D8Virtualization
	git                          gt.Git
	namePrefix                   string
	defaultStorageClass          *storagev1.StorageClass
	phaseByVolumeBindingMode     string
	logStreamByV12nControllerPod = make(map[string]*el.LogStream, 0)
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

	restConfig, err := newRestConfig(conf.ClusterTransport)
	if err != nil {
		log.Fatal(err)
	}
	if kubeClient, err = kubernetes.NewForConfig(restConfig); err != nil {
		log.Fatal(err)
	}
	if virtClient, err = kubeclient.GetClientFromRESTConfig(restConfig); err != nil {
		log.Fatal(err)
	}

	scheme := apiruntime.NewScheme()
	err = virtv2.AddToScheme(scheme)
	if err != nil {
		log.Fatal(err)
	}

	if crClient, err = client.New(restConfig, client.Options{Scheme: scheme}); err != nil {
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
	var kustomizationFiles []string
	v := reflect.ValueOf(conf.TestData)
	t := reflect.TypeOf(conf.TestData)

	if v.Kind() == reflect.Struct {
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldType := t.Field(i)

			// Ignore
			if fieldType.Name == "Sshkey" || fieldType.Name == "SSHUser" {
				continue
			}

			if field.Kind() == reflect.String {
				path := fmt.Sprintf("%s/%s", field.String(), "kustomization.yaml")
				kustomizationFiles = append(kustomizationFiles, path)
			}
		}
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

func newRestConfig(transport config.ClusterTransport) (*rest.Config, error) {
	configFlags := genericclioptions.ConfigFlags{}
	if transport.KubeConfig != "" {
		configFlags.KubeConfig = &transport.KubeConfig
	}
	if transport.Token != "" {
		configFlags.BearerToken = &transport.Token
	}
	if transport.InsecureTLS {
		configFlags.Insecure = &transport.InsecureTLS
	}
	if transport.CertificateAuthority != "" {
		configFlags.CAFile = &transport.CertificateAuthority
	}
	if transport.Endpoint != "" {
		configFlags.APIServer = &transport.Endpoint
	}
	return configFlags.ToRESTConfig()
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
	StartV12nControllerLogStream(logStreamByV12nControllerPod)
})

var _ = AfterSuite(func() {
	errs := make([]error, 0)
	checkErrs := CheckV12nControllerRestarts(logStreamByV12nControllerPod)
	if len(checkErrs) != 0 {
		errs = append(errs, checkErrs...)
	}
	stopErrs := StopV12nControllerLogStream(logStreamByV12nControllerPod)
	if len(stopErrs) != 0 {
		errs = append(errs, stopErrs...)
	}
	Expect(errs).Should(BeEmpty())
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
			Resource:       kc.Resource(r),
		})
		if res.Error() != nil {
			cleanupErrs = append(
				cleanupErrs, fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr()),
			)
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

func StopV12nControllerLogStream(logStreamByPod map[string]*el.LogStream) []error {
	mu := &sync.Mutex{}
	errs := make([]error, 0)
	for _, logStream := range logStreamByPod {
		logStream.Cancel()
		logStream.LogStreamWaitGroup.Add(1)
		go func() {
			defer GinkgoRecover()
			defer logStream.LogStreamWaitGroup.Done()
			warn, err := logStream.WaitCmd()
			mu.Lock()
			if err != nil {
				errs = append(errs, err)
			}
			if warn != "" {
				_, err := GinkgoWriter.Write([]byte(warn))
				if err != nil {
					errs = append(errs, err)
				}
			}
			mu.Unlock()
		}()
		logStream.LogStreamWaitGroup.Wait()
	}
	return errs
}

func CheckV12nControllerRestarts(logStreamByPod map[string]*el.LogStream) []error {
	errs := make([]error, 0)
	for pod, logStream := range logStreamByPod {
		isRestarted, err := IsContainerRestarted(
			pod,
			VirtualizationController,
			VirtualizationNamespace,
			logStream.ContainerStartedAt,
		)
		if err != nil {
			errs = append(errs, err)
		}
		if isRestarted {
			errs = append(errs, fmt.Errorf("the container %q was restarted: %s", VirtualizationController, pod))
		}
	}
	return errs
}
