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

// Package dvcr keeps names of DVCR repositories in sync between the components
// that write images and the components that look their owners up: the
// virtualization-controller builds the repository path when it starts an
// importer or an uploader, while the DVCR garbage collector maps an existing
// repository back to the resource that owns it. Both must derive the same
// string from the same resource, so the shortening lives here and not in either
// of them.
package dvcr

import (
	"encoding/hex"
	"hash/fnv"
	"strings"
)

const (
	// RepoNameMaxLen is the maximum length of an OCI repository name, as defined
	// by the distribution spec. A longer name is rejected while the reference is
	// parsed, before any request reaches the registry.
	//
	// The registry host counts towards the limit: the reference parser vendored
	// into containers/image applies it to the whole name, host included, and that
	// parser is what the pvc-importer reads its source image with.
	RepoNameMaxLen = 255

	// DefaultRegistryHost is the address of DVCR inside the cluster. The registry
	// is a component of the module rather than a configurable endpoint, so the
	// host is the same everywhere.
	// ponytail: a constant while the address is fixed; pass the real host from the
	// module configuration once DVCR can live elsewhere.
	DefaultRegistryHost = "dvcr.d8-virtualization.svc"

	// hashLen is the length of a FNV-1a-64 hash in hex, the shortening scheme the
	// project already uses for names derived from a resource name.
	hashLen = 16
	// Separators are not allowed to end a repository path component.
	separators = "-._"
)

// ClusterImageRepoName returns the name to use in the "cvi/<name>" repository path.
func ClusterImageRepoName(registryHost, name string) string {
	return shorten(hostLen(registryHost)+len("cvi/"), name)
}

// ImageRepoName returns the name to use in the "vi/<namespace>/<name>" repository path.
func ImageRepoName(registryHost, namespace, name string) string {
	return shorten(hostLen(registryHost)+len("vi/")+len(namespace)+len("/"), name)
}

// DiskRepoName returns the name to use in the "vd/<namespace>/<name>" repository path.
func DiskRepoName(registryHost, namespace, name string) string {
	return shorten(hostLen(registryHost)+len("vd/")+len(namespace)+len("/"), name)
}

func hostLen(registryHost string) int {
	if registryHost == "" {
		registryHost = DefaultRegistryHost
	}
	return len(registryHost) + len("/")
}

// shorten keeps the whole repository path within RepoNameMaxLen. Names that
// already fit are returned as is, so repository paths of existing images never
// change. Longer ones are trimmed and get a hash of the original name appended,
// which keeps names that differ only past the cut point apart.
func shorten(prefixLen int, name string) string {
	if prefixLen+len(name) <= RepoNameMaxLen {
		return name
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	hash := hex.EncodeToString(h.Sum(nil)) // 16 hex chars for a 64-bit hash

	keep := RepoNameMaxLen - prefixLen - len("-") - hashLen
	if keep < 1 {
		return hash
	}

	return strings.TrimRight(name[:keep], separators) + "-" + hash
}
