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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	storagev1 "k8s.io/api/storage/v1"

	"github.com/deckhouse/virtualization/tests/e2e/config"
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	"github.com/deckhouse/virtualization/tests/e2e/ginkgoutil"
	gt "github.com/deckhouse/virtualization/tests/e2e/git"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

const (
	Interval                  = 5 * time.Second
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
)

var (
	conf                     *config.Config
	mc                       *config.ModuleConfig
	kustomize                *config.Kustomize
	kubectl                  kc.Kubectl
	d8Virtualization         d8.D8Virtualization
	git                      gt.Git
	namePrefix               string
	defaultStorageClass      *storagev1.StorageClass
	phaseByVolumeBindingMode string
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
		fmt.Sprintf("%s/%s", conf.TestData.SizingPolicy, "kustomization.yaml"),
		fmt.Sprintf("%s/%s", conf.TestData.VdSnapshots, "kustomization.yaml"),
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

func Cleanup() []error {
	cleanupErrs := make([]error, 0)
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
			Filename:       []string{conf.Namespace},
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

	res := kubectl.Delete(kc.DeleteOptions{
		IgnoreNotFound: true,
		Labels:         map[string]string{"id": namePrefix},
		Resource:       kc.ResourceCVI,
	})
	if res.Error() != nil {
		cleanupErrs = append(
			cleanupErrs, fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr()),
		)
	}
	res = kubectl.Delete(kc.DeleteOptions{
		IgnoreNotFound: true,
		Labels:         map[string]string{"id": namePrefix},
		Resource:       kc.ResourceVMClass,
	})
	if res.Error() != nil {
		cleanupErrs = append(
			cleanupErrs, fmt.Errorf("cmd: %s\nstderr: %s", res.GetCmd(), res.StdErr()),
		)
	}

	return cleanupErrs
}
