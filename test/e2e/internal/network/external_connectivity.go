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

package network

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
)

var ExternalConnectivityHosts = []string{
	"https://flant.ru",
	"https://google.com",
	"https://ya.ru",
}

func CheckExternalConnectivity(f *framework.Framework, vmName string, hosts []string) {
	GinkgoHelper()

	cmd := fmt.Sprintf(`set -e
for host in %s; do
	if curl --head -k -sS -o /dev/null --connect-timeout 5 --max-time 15 "$host"; then
		echo "$host"
		exit 0
	fi
done
exit 1`, strings.Join(hosts, " "))

	reachableHost, err := f.SSHCommand(
		vmName,
		f.Namespace().Name,
		cmd,
		framework.WithSSHTimeout(time.Minute),
	)
	Expect(err).NotTo(HaveOccurred(), "VM %s should have outbound connectivity via at least one host from %v", vmName, hosts)
	Expect(strings.TrimSpace(reachableHost)).NotTo(BeEmpty(), "VM %s should report a reachable external host from %v", vmName, hosts)
}
