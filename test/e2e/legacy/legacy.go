/*
Copyright 2025 Flant JSC

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

package legacy

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	dv1alpha2 "github.com/deckhouse/virtualization/test/e2e/internal/api/deckhouse/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/config"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	kc "github.com/deckhouse/virtualization/test/e2e/internal/kubectl"
)

const (
	Interval          = 5 * time.Second
	ShortTimeout      = 30 * time.Second
	Timeout           = 90 * time.Second
	ShortWaitDuration = 60 * time.Second
	LongWaitDuration  = 300 * time.Second
	MaxWaitTimeout    = 1000 * time.Second
	PhaseAttached     = "Attached"
	PhaseReady        = "Ready"
	PhasePending      = "Pending"
	PhaseRunning      = "Running"
	storageClassName  = "STORAGE_CLASS_NAME"
	testDataDir       = "/tmp/testdata"
)

var (
	conf       *config.Config
	kustomize  *config.Kustomize
	kubectl    kc.Kubectl
	namePrefix string
)

func init() {
	err := configure()
	if err != nil {
		panic(fmt.Errorf("failed to configure: %w", err))
	}
}

func configure() (err error) {
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

	//nolint:staticcheck // It can be used in legacy tests.
	namePrefix, err = framework.NewFramework("").GetNamePrefix(conf.StorageClass.TemplateStorageClass)
	if err != nil {
		return err
	}

	if err = ChmodFile(conf.TestData.Sshkey, 0o600); err != nil {
		return err
	}

	return nil
}

func NewBeforeProcess1Body() {
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
}

func NewAfterAllProcessBody() {
	if config.IsCleanUpNeeded() {
		DeferCleanup(func() {
			Expect(Cleanup()).To(Succeed())
		})
	}
}

func Cleanup() error {
	err := deleteProjects()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer GinkgoRecover()
		defer wg.Done()
		if err := deleteNamespaces(); err != nil {
			errChan <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer GinkgoRecover()
		defer wg.Done()
		if err := deleteResources(); err != nil {
			errChan <- err
		}
	}()

	go func() {
		defer GinkgoRecover()
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
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
