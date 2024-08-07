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
	"testing"
	"time"

	"github.com/deckhouse/virtualization/tests/e2e/config"
	d8 "github.com/deckhouse/virtualization/tests/e2e/d8"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	virt "github.com/deckhouse/virtualization/tests/e2e/virtctl"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	ShortWaitDuration      = 60 * time.Second
	LongWaitDuration       = 300 * time.Second
	PhaseReady             = "Ready"
	PhaseBound             = "Bound"
	PhaseReleased          = "Released"
	PhaseSucceeded         = "Succeeded"
	PhaseWaitForUserUpload = "WaitForUserUpload"
)

var (
	conf             *config.Config
	kubectl          kc.Kubectl
	virtctl          virt.Virtctl
	d8Virtualization d8.D8Virtualization
)

func init() {
	var err error
	if conf, err = config.GetConfig(); err != nil {
		panic(err)
	}
	if kubectl, err = kc.NewKubectl(kc.KubectlConf(conf.ClusterTransport)); err != nil {
		panic(err)
	}
	if virtctl, err = virt.NewVirtctl(virt.VirtctlConf(conf.ClusterTransport)); err != nil {
		panic(err)
	}
	if d8Virtualization, err = d8.NewD8Virtualization(d8.D8VirtualizationConf(conf.ClusterTransport)); err != nil {
		panic(err)
	}
	Cleanup()
	res := kubectl.CreateResource(kc.ResourceNamespace, conf.Namespace, kc.CreateOptions{})
	if !res.WasSuccess() {
		panic(fmt.Sprintf("err: %v\n%s", res.Error(), res.StdErr()))
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
}
