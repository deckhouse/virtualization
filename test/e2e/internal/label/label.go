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

package label

import (
	. "github.com/onsi/ginkgo/v2"
)

// SIG labels identify the Special Interest Group that owns a group of e2e
// specs, mirroring Kubernetes' [sig-*] test-ownership labels. They give every
// spec an owner and an axis to run/filter a whole group by, e.g.
// `go tool ginkgo --label-filter='sig-storage'`.
const (
	// SIGStorage owns VirtualDisks, VirtualImages, snapshots, data exports,
	// quota and storage profiles (the blockdevice suite).
	SIGStorage = "sig-storage"
	// SIGCompute owns the VirtualMachine lifecycle: run policy, sizing, CPU/memory
	// hotplug, power state, snapshots, pools and operations.
	SIGCompute = "sig-compute"
	// SIGNetwork owns VM networking: connectivity, IPAM and additional interfaces.
	SIGNetwork = "sig-network"
	// SIGMigration owns live migration and evacuation.
	SIGMigration = "sig-migration"
)

// SIGDescribe is [Describe] that tags every spec in the container with the
// owning SIG label. Mirrors Kubernetes' framework.SIGDescribe: it records who
// owns the group and lets it be run in isolation via `--label-filter`.
func SIGDescribe(sig, text string, args ...interface{}) bool {
	return Describe(text, append([]interface{}{Label(sig)}, args...)...)
}

func Slow() Labels {
	return Label("Slow")
}

func TPM() Labels {
	return Label("TPM")
}

func Legacy() Labels {
	return Label("Legacy")
}
