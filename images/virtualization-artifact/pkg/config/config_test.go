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

package config

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var _ = Describe("Config", func() {
	var cfgRaw = []byte(`apiVersion: configuration.virtualization.deckhouse.io/v1alpha1
kind: VirtualizationControllerConfiguration
spec:
  namespace: d8-virtualization
  virtControllerName: "virt-controller"
  virtualMachineCIDRs:
  - "10.66.10.0/24"
  - "10.66.20.0/24"
  - "10.66.30.0/24"
  - "10.66.40.0/24"
  virtualMachineIPLeasesRetentionDuration: 10m
  firmwareImage: firmwareimage:latest
  garbageCollector:
    vmop:
      ttl: 24h
      schedule: "0 * * * *"
    vmiMigration:
      ttl: 24h
      schedule: "0 * * * *"
  importSettings:
    importerImage: importerimage:latest
    uploaderImage: uploaderimage:latest
    bounderImage: bounderimage:latest
    requirements:
      limits:
        cpu: 750m
        memory: 600Mi
      requests:
        cpu: 100m
      memory: 60Mi
  dvcr:
    authSecret: dvcr-dockercfg-rw
    certsSecret: dvcr-tls
    registryURL: "dvcr.d8-virtualization.svc"
    insecureTLS: true
  ingress:
    host: virtualization.example.com
    tlsSecret: ingress-tls
    class: "nginx"
`)
	It("Should decode config", func() {
		_, err := decodeVirtualizationControllerConfiguration(cfgRaw)
		Expect(err).NotTo(HaveOccurred())
	})
})
