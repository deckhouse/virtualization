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
	"errors"
	"fmt"
	"log"
	"reflect"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	"github.com/deckhouse/virtualization/tests/e2e/d8"
	el "github.com/deckhouse/virtualization/tests/e2e/errlogger"
	gt "github.com/deckhouse/virtualization/tests/e2e/git"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	Interval                  = 5 * time.Second
	ShortTimeout              = 30 * time.Second
	Timeout                   = 90 * time.Second
	ShortWaitDuration         = 60 * time.Second
	LongWaitDuration          = 300 * time.Second
	MaxWaitTimeout            = 1000 * time.Second
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
	phaseByVolumeBindingMode     string
	logStreamByV12nControllerPod = make(map[string]*el.LogStream, 0)
)

func init() {
	err := config.CheckReusableOption()
	if err != nil {
		log.Fatal(err)
	}
	err = config.CheckStorageClassOption()
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

	scheme := runtime.NewScheme()
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
	if conf.StorageClass.DefaultStorageClass, err = GetDefaultStorageClass(); err != nil {
		log.Fatal(err)
	}
	if !config.SkipImmediateStorageClassCheck() {
		if conf.StorageClass.ImmediateStorageClass, err = GetImmediateStorageClass(conf.StorageClass.DefaultStorageClass.Provisioner); err != nil {
			log.Fatal(err)
		}
	}
	if namePrefix, err = config.GetNamePrefix(); err != nil {
		log.Fatal(err)
	}
	ChmodFile(conf.TestData.Sshkey, 0o600)
	phaseByVolumeBindingMode = GetPhaseByVolumeBindingMode(conf.StorageClass.DefaultStorageClass)
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
}

var _ = SynchronizedBeforeSuite(func() {
	var kustomizationFiles []string
	v := reflect.ValueOf(conf.TestData)
	t := reflect.TypeOf(conf.TestData)

	if v.Kind() == reflect.Struct {
		for i := range v.NumField() {
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

	ns := fmt.Sprintf("%s-%s", namePrefix, conf.NamespaceSuffix)
	for _, filePath := range kustomizationFiles {
		err := kustomize.SetParams(filePath, ns, namePrefix)
		if err != nil {
			log.Fatal(err)
		}
	}

	if !config.IsReusable() {
		err := Cleanup()
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Println("Run test in REUSABLE mode")
	}

	StartV12nControllerLogStream(logStreamByV12nControllerPod)
	DeferCleanup(func() {
		if config.IsCleanUpNeeded() {
			err := Cleanup()
			if err != nil {
				log.Fatal(err)
			}
		}
	})
}, func() {})

var _ = SynchronizedAfterSuite(func() {}, func() {
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

func Cleanup() error {
	var eg errgroup.Group

	err := deleteProject()
	if err != nil {
		return err
	}

	eg.Go(deleteNamespaces)
	eg.Go(deleteResources)

	return eg.Wait()
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

func deleteProject() error {
	res := kubectl.Delete(kc.DeleteOptions{
		IgnoreNotFound: true,
		Labels:         map[string]string{"id": namePrefix},
		Resource:       kc.ResourceProject,
	})
	if res.Error() != nil {
		return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
	}

	return nil
}

func deleteNamespaces() error {
	testCases, cleanupErr := conf.GetTestCases()
	if cleanupErr != nil {
		return cleanupErr
	}

	var eg errgroup.Group

	for _, tc := range testCases {
		eg.Go(func() error {
			kustomizeFilePath := fmt.Sprintf("%s/kustomization.yaml", tc)
			namespace, err := kustomize.GetNamespace(kustomizeFilePath)
			if err != nil {
				return fmt.Errorf("cannot cleanup namespace %q: %w", namespace, err)
			}
			res := kubectl.Delete(kc.DeleteOptions{
				Filename:       []string{namespace},
				IgnoreNotFound: true,
				Resource:       kc.ResourceNamespace,
			})
			if res.Error() != nil {
				return fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr())
			}

			return nil
		})
	}

	return eg.Wait()
}

func deleteResources() error {
	var cleanupErr error

	for _, r := range conf.CleanupResources {
		res := kubectl.Delete(kc.DeleteOptions{
			IgnoreNotFound: true,
			Labels:         map[string]string{"id": namePrefix},
			Resource:       kc.Resource(r),
		})
		if res.Error() != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr()))
		}
	}

	return cleanupErr
}
