/*
Copyright 2026 Flant JSC

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

package dvcr

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	dvcrrepo "github.com/deckhouse/virtualization/api/dvcr"
)

const registryURL = "dvcr.d8-virtualization.svc"

var _ = Describe("RegistryImage", func() {
	var (
		s       *Settings
		longNS  = strings.Repeat("n", 63)
		maxName = strings.Repeat("m", 253)
	)

	BeforeEach(func() {
		s = &Settings{RegistryURL: registryURL}
	})

	DescribeTable("should keep the repository within the OCI limit",
		func(image func() string) {
			// The host counts towards the limit, so measure what the parser sees.
			name := registryURL + "/" + s.RepoPath(image())
			Expect(len(name)).To(BeNumerically("<=", dvcrrepo.RepoNameMaxLen), name)
		},
		Entry("CVI", func() string { return s.RegistryImageForCVI(objectWithName("", maxName)) }),
		Entry("VI in default", func() string { return s.RegistryImageForVI(objectWithName("default", maxName)) }),
		Entry("VI in a long namespace", func() string { return s.RegistryImageForVI(objectWithName(longNS, maxName)) }),
		Entry("VD in a long namespace", func() string { return s.RegistryImageForVD(objectWithName(longNS, maxName)) }),
	)

	It("should keep short names as is", func() {
		obj := objectWithName("default", "ubuntu-noble")

		Expect(s.RepoPath(s.RegistryImageForVI(obj))).To(Equal("vi/default/ubuntu-noble"))
	})

	// The DVCR garbage collector maps a repository back to its owner by building
	// the same path from the resource: "<kind>/<namespace>/<shortened name>". If
	// the templates here stop agreeing with it, images of long-named resources
	// are reported as owned by nobody and deleted.
	DescribeTable("should write to the path the garbage collector looks for",
		func(image, want func() string) {
			Expect(s.RepoPath(image())).To(Equal(want()))
		},
		Entry("cvi",
			func() string { return s.RegistryImageForCVI(objectWithName("", maxName)) },
			func() string { return "cvi/" + dvcrrepo.ClusterImageRepoName(registryURL, maxName) }),
		Entry("vi",
			func() string { return s.RegistryImageForVI(objectWithName(longNS, maxName)) },
			func() string { return "vi/" + longNS + "/" + dvcrrepo.ImageRepoName(registryURL, longNS, maxName) }),
		Entry("vd",
			func() string { return s.RegistryImageForVD(objectWithName(longNS, maxName)) },
			func() string { return "vd/" + longNS + "/" + dvcrrepo.DiskRepoName(registryURL, longNS, maxName) }),
	)
})

func objectWithName(namespace, name string) *v1alpha2.VirtualImage {
	return &v1alpha2.VirtualImage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("59bf9d0e-8bf4-407b-a00c-fb77518d4858"),
		},
	}
}
