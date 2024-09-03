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

	"github.com/deckhouse/virtualization/tests/e2e/config"
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	gt "github.com/deckhouse/virtualization/tests/e2e/git"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	virt "github.com/deckhouse/virtualization/tests/e2e/virtctl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	ShortWaitDuration      = 60 * time.Second
	LongWaitDuration       = 300 * time.Second
	PhaseAttached          = "Attached"
	PhaseReady             = "Ready"
	PhaseBound             = "Bound"
	PhaseReleased          = "Released"
	PhaseSucceeded         = "Succeeded"
	PhaseRunning           = "Running"
	PhaseWaitForUserUpload = "WaitForUserUpload"
)

var (
	conf             *config.Config
	mc               *config.ModuleConfig
	kustomize        *config.Kustomize
	kubectl          kc.Kubectl
	virtctl          virt.Virtctl
	d8Virtualization d8.D8Virtualization
	git              gt.Git
	namePrefix       string
)

func init() {
	var err error
	if conf, err = config.GetConfig(); err != nil {
		log.Fatal(err)
	}
	if mc, err = config.GetModuleConfig(); err != nil {
		log.Fatal(err)
	}
	if kubectl, err = kc.NewKubectl(kc.KubectlConf(conf.ClusterTransport)); err != nil {
		log.Fatal(err)
	}
	if virtctl, err = virt.NewVirtctl(virt.VirtctlConf(conf.ClusterTransport)); err != nil {
		log.Fatal(err)
	}
	if d8Virtualization, err = d8.NewD8Virtualization(d8.D8VirtualizationConf(conf.ClusterTransport)); err != nil {
		log.Fatal(err)
	}
	if git, err = gt.NewGit(); err != nil {
		log.Fatal(err)
	}
	if err = CheckDefaultStorageClass(); err != nil {
		log.Fatal(err)
	}
	if namePrefix, err = config.GetNamePrefix(); err != nil {
		log.Fatal(err)
	}
	conf.Namespace = fmt.Sprintf("%s-%s", namePrefix, conf.Namespace)
	kustomizeFilePath := fmt.Sprintf("%s/%s", conf.VirtualizationResources, "kustomization.yaml")
	if err = kustomize.SetParams(kustomizeFilePath, conf.Namespace, namePrefix); err != nil {
		log.Fatal(err)
	}
	Cleanup()
	res := kubectl.CreateResource(kc.ResourceNamespace, conf.Namespace, kc.CreateOptions{})
	if !res.WasSuccess() {
		log.Fatalf("err: %v\n%s", res.Error(), res.StdErr())
	}
}

func TestTests(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting test suite\n")
	RunSpecs(t, "Tests")
	Cleanup()
}

func Cleanup() {
	kubectl.DeleteResource(kc.ResourceNamespace, conf.Namespace, kc.DeleteOptions{})
	kubectl.DeleteResource(kc.ResourceCVI, "", kc.DeleteOptions{
		Label: fmt.Sprintf("testcase=%s", namePrefix),
	})
}
