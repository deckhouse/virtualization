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
	"regexp"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// pathComponent is the repository path component grammar from the distribution spec.
var pathComponent = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)*$`)

const host = DefaultRegistryHost

func TestRepo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DVCR Repository Name Suite")
}

var _ = Describe("Repository name", func() {
	var (
		longNS  = strings.Repeat("n", 63)
		maxName = strings.Repeat("m", 253)
	)

	DescribeTable("should keep the reference within the limit",
		func(path string) {
			// The host counts towards the limit, see RepoNameMaxLen.
			Expect(len(host + "/" + path)).To(BeNumerically("<=", RepoNameMaxLen))

			for _, component := range strings.Split(path, "/") {
				Expect(component).To(MatchRegexp(pathComponent.String()), "invalid path component")
			}
		},
		Entry("cvi", "cvi/"+ClusterImageRepoName(host, maxName)),
		Entry("vi in default", "vi/default/"+ImageRepoName(host, "default", maxName)),
		Entry("vi in a long namespace", "vi/"+longNS+"/"+ImageRepoName(host, longNS, maxName)),
		Entry("vd in a long namespace", "vd/"+longNS+"/"+DiskRepoName(host, longNS, maxName)),
	)

	// These values are the addresses images are actually stored under. Changing
	// the shortening scheme moves every long-named image to a new repository,
	// orphaning what is already in DVCR — so these entries are meant to fail loudly.
	DescribeTable("should derive the same name it always has",
		func(got, wantSuffix string, wantLen int) {
			Expect(got).To(HaveSuffix(wantSuffix))
			Expect(got).To(HaveLen(wantLen))
		},
		Entry("cvi at the maximum name length",
			ClusterImageRepoName(host, "cvi-max-upload-"+strings.Repeat("x", 253-len("cvi-max-upload-"))),
			"-3dd4207548577095", 224),
		Entry("vi at the maximum name length in the longest namespace",
			ImageRepoName(host, "ns-max-"+strings.Repeat("x", 63-len("ns-max-")),
				"vi-max-upload-"+strings.Repeat("x", 253-len("vi-max-upload-"))),
			"-71106fc097cdd39e", 161),
	)

	DescribeTable("should keep names that still fit as is",
		func(name string) {
			// Otherwise images already stored in DVCR would change their address.
			Expect(ClusterImageRepoName(host, name)).To(Equal(name))
		},
		Entry("a name of a usual length", "ubuntu-noble"),
		Entry("a name at the very edge of the budget", strings.Repeat("m", 224)),
	)

	It("should keep long names apart", func() {
		// Both names are trimmed at the same point, so only the hash tells them apart.
		prefix := strings.Repeat("m", 224)
		first := ClusterImageRepoName(host, prefix+"a")

		Expect(first).ToNot(Equal(ClusterImageRepoName(host, prefix+"b")), "names collapsed past the cut point")
		Expect(ClusterImageRepoName(host, prefix+"a")).To(Equal(first), "not deterministic")
	})

	It("should not end a trimmed name with a separator", func() {
		// A trailing separator right before the hash would make the component invalid.
		name := strings.Repeat("m", 200) + strings.Repeat("-", 5) + strings.Repeat("t", 50)

		Expect(ClusterImageRepoName(host, name)).To(MatchRegexp(pathComponent.String()))
	})
})
