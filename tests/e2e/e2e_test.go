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
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	dv1alpha2 "github.com/deckhouse/virtualization/tests/e2e/api/deckhouse/v1alpha2"
	"github.com/deckhouse/virtualization/tests/e2e/config"
	el "github.com/deckhouse/virtualization/tests/e2e/errlogger"
	"github.com/deckhouse/virtualization/tests/e2e/framework"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	_ "github.com/deckhouse/virtualization/tests/e2e/storage"
)

const (
	Interval                 = 5 * time.Second
	ShortTimeout             = 30 * time.Second
	Timeout                  = 90 * time.Second
	ShortWaitDuration        = 60 * time.Second
	LongWaitDuration         = 300 * time.Second
	MaxWaitTimeout           = 1000 * time.Second
	PhaseAttached            = "Attached"
	PhaseReady               = "Ready"
	PhasePending             = "Pending"
	PhaseRunning             = "Running"
	VirtualizationController = "virtualization-controller"
	VirtualizationNamespace  = "d8-virtualization"
	storageClassName         = "STORAGE_CLASS_NAME"
	testDataDir              = "/tmp/testdata"
)

var (
	conf       *config.Config
	kustomize  *config.Kustomize
	kubectl    kc.Kubectl
	namePrefix string
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	if err := initE2E(); err != nil {
		t.Fatalf("initE2E failed: %s", err)
	}
	RunSpecs(t, "Tests")
}

func initE2E() (err error) {
	if err = config.CheckStorageClassOption(); err != nil {
		return err
	}
	if err = config.CheckWithPostCleanUpOption(); err != nil {
		return err
	}

	conf = framework.GetConfig()
	defer framework.SetConfig(conf)

	clients := framework.GetClients()
	kubectl = clients.Kubectl()

	if conf.StorageClass.DefaultStorageClass, err = GetDefaultStorageClass(); err != nil {
		return err
	}

	if !config.SkipImmediateStorageClassCheck() {
		if conf.StorageClass.ImmediateStorageClass, err = GetImmediateStorageClass(conf.StorageClass.DefaultStorageClass.Provisioner); err != nil {
			Fail(err.Error())
		}
	}

	scFromEnv, err := GetStorageClassFromEnv(storageClassName)
	if err != nil {
		return err
	}

	if scFromEnv != nil {
		conf.StorageClass.TemplateStorageClass = scFromEnv
	} else {
		conf.StorageClass.TemplateStorageClass = conf.StorageClass.DefaultStorageClass
	}

	if err = SetStorageClass(testDataDir, map[string]string{storageClassName: conf.StorageClass.TemplateStorageClass.Name}); err != nil {
		return err
	}

	if err = config.CheckDefaultVMClass(clients.VirtClient()); err != nil {
		return err
	}

	if namePrefix, err = framework.NewFramework("").GetNamePrefix(); err != nil {
		return err
	}

	if err = ChmodFile(conf.TestData.Sshkey, 0o600); err != nil {
		return err
	}

	return nil
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
			Expect(err).NotTo(HaveOccurred())
		}
	}

	Expect(Cleanup()).To(Succeed())

	Expect(defaultLogStreamer.Start()).To(Succeed())
}, func() {})

var _ = SynchronizedAfterSuite(func() {}, func() {
	Expect(defaultControllerRestartChecker.Check()).To(Succeed())
	Expect(defaultLogStreamer.Stop()).To(Succeed())

	DeferCleanup(func() {
		if config.IsCleanUpNeeded() {
			Expect(Cleanup()).To(Succeed())
		}
	})
})

func Cleanup() error {
	var eg errgroup.Group

	err := deleteProjects()
	if err != nil {
		return err
	}

	eg.Go(deleteNamespaces)
	eg.Go(deleteResources)

	return eg.Wait()
}

type logStreamer struct {
	ctx     context.Context
	cancel  context.CancelFunc
	closers []io.Closer
	wg      *sync.WaitGroup

	resultNum int
	resultErr error
	mu        sync.Mutex
}

var defaultLogStreamer = &logStreamer{}

// This function is used to detect `v12n-controller` errors while the test suite is running.
func (l *logStreamer) Start() error {
	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.wg = &sync.WaitGroup{}

	c := framework.GetConfig()
	excludePatterns := c.LogFilter
	excludeRegexpPatterns := c.RegexpLogFilter
	logStreamer := el.NewLogStreamer(excludePatterns, excludeRegexpPatterns)

	kubeClient := framework.GetClients().KubeClient()
	pods, err := kubeClient.CoreV1().Pods(VirtualizationNamespace).List(l.ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"app": VirtualizationController}).String(),
	})
	if err != nil {
		return fmt.Errorf("failed to obtain the `Virtualization-controller` pods: %w", err)
	}

	for _, p := range pods.Items {
		req := kubeClient.CoreV1().Pods(VirtualizationNamespace).GetLogs(p.Name, &corev1.PodLogOptions{
			Container: VirtualizationController,
			Follow:    true,
		})
		readCloser, err := req.Stream(l.ctx)
		if err != nil {
			return fmt.Errorf("failed to stream the `Virtualization-controller` logs: %w", err)
		}

		l.closers = append(l.closers, readCloser)

		l.wg.Add(1)
		go func() {
			defer l.wg.Done()

			n, err := logStreamer.Stream(readCloser, GinkgoWriter)
			l.mu.Lock()
			defer l.mu.Unlock()
			if err != nil && !errors.Is(err, context.Canceled) {
				l.resultErr = errors.Join(l.resultErr, err)
			}
			l.resultNum += n
		}()
	}
	return nil
}

func (l *logStreamer) Stop() error {
	l.cancel()
	l.wg.Wait()
	for _, c := range l.closers {
		_ = c.Close()
	}

	if l.resultErr != nil {
		return l.resultErr
	}
	if l.resultNum > 0 {
		return fmt.Errorf("errors have appeared in the `Virtualization-controller` logs")
	}

	return nil
}

type controllerRestartChecker struct {
	startedAt metav1.Time
}

var defaultControllerRestartChecker = &controllerRestartChecker{startedAt: metav1.Now()}

func (c *controllerRestartChecker) Check() error {
	kubeClient := framework.GetClients().KubeClient()
	pods, err := kubeClient.CoreV1().Pods(VirtualizationNamespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{"app": VirtualizationController}).String(),
	})
	if err != nil {
		return err
	}

	var errs error
	for _, pod := range pods.Items {
		foundContainer := false
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if containerStatus.Name == VirtualizationController && containerStatus.State.Running != nil {
				foundContainer = true
				if containerStatus.State.Running.StartedAt.After(c.startedAt.Time) {
					errs = errors.Join(errs, fmt.Errorf("the container %q was restarted: %s", VirtualizationController, pod.Name))
				}
			}
		}
		if !foundContainer {
			errs = errors.Join(errs, fmt.Errorf("the container %q was not found: %s", VirtualizationController, pod.Name))
		}
	}

	return errs
}

func deleteProjects() error {
	genericClient := framework.GetClients().GenericClient()

	projects := &dv1alpha2.ProjectList{}
	err := genericClient.List(context.Background(), projects, crclient.MatchingLabels{"id": namePrefix})
	if err != nil {
		return err
	}

	var errs error
	for _, project := range projects.Items {
		err = genericClient.Delete(context.Background(), &project)
		if err != nil && !k8serrors.IsNotFound(err) {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

func deleteNamespaces() error {
	testCases, err := conf.GetTestCases()
	if err != nil {
		return err
	}

	kubeClient := framework.GetClients().KubeClient()

	var cleanupErr error

	for _, tc := range testCases {
		kustomizeFilePath := fmt.Sprintf("%s/kustomization.yaml", tc)
		namespace, err := kustomize.GetNamespace(kustomizeFilePath)
		if err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
			continue
		}

		err = kubeClient.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			cleanupErr = errors.Join(cleanupErr, err)
			continue
		}
	}

	return cleanupErr
}

func deleteResources() error {
	defer GinkgoRecover()

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
