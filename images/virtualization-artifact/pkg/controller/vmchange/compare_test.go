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

package vmchange

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/yaml"

	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var _ = Describe("VMSpecComparator", func() {
	DescribeTable("returns the action a spec change requires",
		func(currentSpec, desiredSpec string, features []featuregate.Feature, assertFn func(changes SpecChanges)) {
			gate := featuregates.Default()
			if len(features) > 0 {
				gate = newFeatureGate(features...)
			}

			changes := NewVMSpecComparator(gate).Compare(loadVMSpec(currentSpec), loadVMSpec(desiredSpec))
			assertFn(changes)
		},
		Entry(
			// SPICE builds the display, the sound card, the redirection slots and the
			// vdagent channel into the domain at start, so it cannot be switched on a
			// running VM.
			"restart on spice.enabled change",
			`
spice:
  enabled: false
`,
			`
spice:
  enabled: true
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("spice.enabled", ChangeReplace),
			),
		),
		Entry(
			"restart on cpu.cores change",
			`
cpu:
  cores: 2
`,
			`
cpu:
  cores: 3
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("cpu.cores", ChangeReplace),
			),
		),
		Entry(
			"restart on cpu.coreFraction change",
			`
cpu:
  cores: 2
  coreFraction: 60%
`,
			`
cpu:
  cores: 2
  coreFraction: 40%
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("cpu.coreFraction", ChangeReplace),
			),
		),
		Entry(
			"restart on cpu section change when hotplug is disabled",
			`
cpu:
  cores: 2
  coreFraction: 60%
`,
			`
cpu:
  cores: 6
  coreFraction: 40%
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("cpu", ChangeReplace),
			),
		),
		Entry(
			"immediate apply on cpu section change when hotplug is enabled",
			`
cpu:
  cores: 2
  coreFraction: 60%
`,
			`
cpu:
  cores: 6
  coreFraction: 40%
`,
			[]featuregate.Feature{featuregates.HotplugCPUWithLiveMigration},
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("cpu", ChangeReplace),
			),
		),
		Entry(
			"restart on cpu section change for Legacy osType even with hotplug enabled",
			`
osType: Legacy
cpu:
  cores: 2
  coreFraction: 60%
`,
			`
osType: Legacy
cpu:
  cores: 4
  coreFraction: 60%
`,
			[]featuregate.Feature{featuregates.HotplugCPUWithLiveMigration},
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("cpu.cores", ChangeReplace),
			),
		),
		Entry(
			"restart on memory.size change for Legacy osType even with hotplug enabled",
			`
osType: Legacy
memory:
  size: 2Gi
`,
			`
osType: Legacy
memory:
  size: 4Gi
`,
			[]featuregate.Feature{featuregates.HotplugMemoryWithLiveMigration},
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("memory.size", ChangeReplace),
			),
		),
		Entry(
			"restart on cpu.coreFraction crossing the QoS boundary even with hotplug",
			`
cpu:
  cores: 2
  coreFraction: 100%
`,
			`
cpu:
  cores: 2
  coreFraction: 50%
`,
			[]featuregate.Feature{featuregates.HotplugCPUAndMemoryWithInPlaceResize},
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("cpu.coreFraction", ChangeReplace),
			),
		),
		Entry(
			"immediate apply on cpu.coreFraction change within Burstable with hotplug",
			`
cpu:
  cores: 2
  coreFraction: 50%
`,
			`
cpu:
  cores: 2
  coreFraction: 99%
`,
			[]featuregate.Feature{featuregates.HotplugCPUAndMemoryWithInPlaceResize},
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("cpu.coreFraction", ChangeReplace),
			),
		),
		Entry(
			"no restart cpu.coreFraction from empty to default value",
			`
cpu:
  cores: 2
`,
			`
cpu:
  cores: 2
  coreFraction: 100%
`,
			nil,
			assertChanges(
				actionRequired(ActionNone),
				requirePathOperation("cpu.coreFraction", ChangeAdd),
			),
		),
		Entry(
			"no restart cpu.coreFraction from default value to empty",
			`
cpu:
  cores: 2
  coreFraction: 100%
`,
			`
cpu:
  cores: 2
`,
			nil,
			assertChanges(
				actionRequired(ActionNone),
				requirePathOperation("cpu.coreFraction", ChangeRemove),
			),
		),
		Entry(
			"restart on memory.size change: no hotplug",
			`
memory:
  size: 256Mi
`,
			`
memory:
  size: 512Mi
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("memory.size", ChangeReplace),
			),
		),
		Entry(
			"restart on memory.size change: enable hotplug",
			`
memory:
  size: 384Mi
`,
			`
memory:
  size: 2Gi
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("memory.size", ChangeReplace),
			),
		),
		Entry(
			"restart on memory.size change: disable hotplug",
			`
memory:
  size: 3Gi
`,
			`
memory:
  size: 128Mi
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("memory.size", ChangeReplace),
			),
		),
		Entry(
			"immediate apply on memory.size increase when hotplug is enabled",
			`
memory:
  size: 2Gi
`,
			`
memory:
  size: 4Gi
`,
			[]featuregate.Feature{featuregates.HotplugMemoryWithLiveMigration},
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("memory.size", ChangeReplace),
			),
		),
		Entry(
			"restart on memory.size increase when hotplug is disabled",
			`
memory:
  size: 2Gi
`,
			`
memory:
  size: 4Gi
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("memory.size", ChangeReplace),
			),
		),
		Entry(
			"restart on memory.size reduce when hotplug is enabled",
			`
memory:
  size: 4Gi
`,
			`
memory:
  size: 2Gi
`,
			[]featuregate.Feature{featuregates.HotplugMemoryWithLiveMigration},
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("memory.size", ChangeReplace),
			),
		),
		Entry(
			"restart on memory.size reduce when hotplug is disabled",
			`
memory:
  size: 4Gi
`,
			`
memory:
  size: 2Gi
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("memory.size", ChangeReplace),
			),
		),
		Entry(
			"apply immediate on blockDeviceRefs section add",
			``,
			`
blockDeviceRefs:
- kind: VirtualImage
  name: linux
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs", ChangeAdd),
			),
		),
		Entry(
			"apply immediate on blockDeviceRefs section remove",
			`
blockDeviceRefs:
- kind: VirtualImage
  name: linux
`,
			``,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs", ChangeRemove),
			),
		),
		Entry(
			"apply immediate on blockDeviceRefs add disk",
			`
enableParavirtualization: true
blockDeviceRefs:
- kind: VirtualImage
  name: linux
`,
			`
enableParavirtualization: true
blockDeviceRefs:
- kind: VirtualDisk
  name: linux
- kind: VirtualImage
  name: linux
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs.0", ChangeAdd),
			),
		),
		Entry(
			"apply immediate on blockDeviceRefs remove disk",
			`
enableParavirtualization: true
blockDeviceRefs:
- kind: VirtualDisk
  name: linux
- kind: VirtualImage
  name: linux
`,
			`
enableParavirtualization: true
blockDeviceRefs:
- kind: VirtualImage
  name: linux
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs.0", ChangeRemove),
			),
		),
		Entry(
			"restart on blockDeviceRefs remove disk with paravirt disabled",
			`
enableParavirtualization: false
blockDeviceRefs:
- kind: VirtualDisk
  name: linux
- kind: VirtualImage
  name: linux
`,
			`
enableParavirtualization: false
blockDeviceRefs:
- kind: VirtualImage
  name: linux
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("blockDeviceRefs.0", ChangeRemove),
			),
		),
		Entry(
			"apply immediate on blockDeviceRefs change order",
			`
enableParavirtualization: true
blockDeviceRefs:
- kind: VirtualImage
  name: linux
- kind: VirtualDisk
  name: linux
`,
			`
enableParavirtualization: true
blockDeviceRefs:
- kind: VirtualDisk
  name: linux
- kind: VirtualImage
  name: linux
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs.0", ChangeReplace),
				requirePathOperation("blockDeviceRefs.1", ChangeReplace),
			),
		),
		Entry(
			"apply immediate on blockDeviceRefs change order :: bigger",
			`
enableParavirtualization: true
blockDeviceRefs:
- kind: VirtualImage
  name: linux
- kind: VirtualDisk
  name: linux
- kind: ClusterVirtualImage
  name: ubuntu
- kind: VirtualDisk
  name: main
- kind: VirtualDisk
  name: data
`,
			// Change order: 12345 -> 25341
			`
enableParavirtualization: true
blockDeviceRefs:
- kind: VirtualDisk
  name: linux
- kind: VirtualDisk
  name: data
- kind: ClusterVirtualImage
  name: ubuntu
- kind: VirtualDisk
  name: main
- kind: VirtualImage
  name: linux
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs.0", ChangeReplace),
				requirePathOperation("blockDeviceRefs.1", ChangeReplace),
				requirePathOperation("blockDeviceRefs.4", ChangeReplace),
			),
		),
		Entry(
			"restart on provisioning add",
			`
`,
			`
provisioning:
  type: UserData
  userData: |
    #cloudinit
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning", ChangeAdd),
			),
		),
		Entry(
			"apply immediate on provisioning remove",
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: Secret
    name: cloud-init-secret
`,
			"",
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("provisioning", ChangeRemove),
			),
		),
		Entry(
			"apply immediate on inline UserData provisioning remove",
			`
provisioning:
  type: UserData
  userData: |
    #cloud-config
`,
			"",
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("provisioning", ChangeRemove),
			),
		),
		Entry(
			"restart on Sysprep provisioning remove",
			`
provisioning:
  type: SysprepRef
  sysprepRef:
    kind: Secret
    name: sysprep-secret
`,
			"",
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning", ChangeRemove),
			),
		),
		Entry(
			"restart on provisioning type change from UserData to Sysprep",
			`
provisioning:
  type: UserData
  userData: |
    #cloud-config
`,
			`
provisioning:
  type: SysprepRef
  sysprepRef:
    kind: Secret
    name: sysprep-secret
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning", ChangeReplace),
			),
		),
		Entry(
			"restart on provisioning type change",
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: Secret
    name: cloud-init-secret
`,
			`
provisioning:
  type: UserData
  userData: |
    #cloudinit
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning", ChangeReplace),
			),
		),
		Entry(
			"restart on provisioning secretref name change",
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: Secret
    name: cloud-init-secret
`,
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: Secret
    name: provisioning-secret
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning.userDataRef.name", ChangeReplace),
			),
		),
		Entry(
			"restart on enableParavirtualization change true to false",
			`
enableParavirtualization: true
`,
			`
enableParavirtualization: false
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("enableParavirtualization", ChangeReplace),
			),
		),
		Entry(
			"restart on enableParavirtualization change false to true",
			`
enableParavirtualization: false
`,
			`
enableParavirtualization: true
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("enableParavirtualization", ChangeReplace),
			),
		),
		Entry(
			"no restart when network ids are assigned",
			`
networks:
- type: Main
- type: Network
  name: net1
`,
			`
networks:
- type: Main
  id: 1
- type: Network
  name: net1
  id: 2
`,
			nil,
			assertChanges(
				actionRequired(ActionNone),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"apply immediate when adding non-main network to nil networks",
			``,
			`
networks:
- type: Main
  id: 1
- type: ClusterNetwork
  name: additional
  id: 2
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("networks", ChangeAdd),
			),
		),
		Entry(
			"apply immediate when adding non-main network to existing main",
			`
networks:
- type: Main
  id: 1
`,
			`
networks:
- type: Main
  id: 1
- type: ClusterNetwork
  name: additional
  id: 2
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"apply immediate when removing non-main network",
			`
networks:
- type: Main
  id: 1
- type: Network
  name: net1
  id: 2
`,
			`
networks:
- type: Main
  id: 1
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"apply immediate when only ipAddressName is added",
			`
networks:
- type: Main
  id: 1
- type: Network
  name: net1
  id: 2
`,
			`
networks:
- type: Main
  id: 1
- type: Network
  name: net1
  id: 2
  ipAddressName: my-ip
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"restart when main network ipAddressName changes",
			`
networks:
- type: Main
  id: 1
  ipAddressName: main-ip-a
`,
			`
networks:
- type: Main
  id: 1
  ipAddressName: main-ip-b
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"restart when main network is removed",
			`
networks:
- type: Main
  id: 1
- type: Network
  name: net1
  id: 2
`,
			`
networks:
- type: Network
  name: net1
  id: 2
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"apply immediate when removing non-main network from VM without main",
			`
networks:
- type: ClusterNetwork
  name: net1
  id: 2
- type: ClusterNetwork
  name: net2
  id: 3
`,
			`
networks:
- type: ClusterNetwork
  name: net1
  id: 2
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"apply immediate when adding non-main network to VM without main",
			`
networks:
- type: ClusterNetwork
  name: net1
  id: 2
`,
			`
networks:
- type: ClusterNetwork
  name: net1
  id: 2
- type: ClusterNetwork
  name: net2
  id: 3
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"restart when only-non-main spec gains main network",
			`
networks:
- type: ClusterNetwork
  name: net1
  id: 2
`,
			`
networks:
- type: Main
  id: 1
- type: ClusterNetwork
  name: net1
  id: 2
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"no restart when gpu devices only change order",
			`
gpus:
- gpuClassName: nvidia-h100
- gpuClassName: nvidia-a100
`,
			`
gpus:
- gpuClassName: nvidia-a100
- gpuClassName: nvidia-h100
`,
			nil,
			assertNoChanges(),
		),
		Entry(
			"restart when gpu device gpuClassName changes",
			`
gpus:
- gpuClassName: nvidia-a100
`,
			`
gpus:
- gpuClassName: nvidia-h100
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("gpus", ChangeReplace),
			),
		),
		Entry(
			"no restart when gpu devices change between empty list and unset",
			`
gpus: []
`,
			``,
			nil,
			assertNoChanges(),
		),
		Entry(
			"restart when underlay network vlanID changes",
			`
networks:
- type: Main
  id: 1
- type: UnderlayNetwork
  name: uplink
  id: 2
  vlanID: 100
`,
			`
networks:
- type: Main
  id: 1
- type: UnderlayNetwork
  name: uplink
  id: 2
  vlanID: 200
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"restart when underlay network vlanID is set",
			`
networks:
- type: UnderlayNetwork
  name: uplink
  id: 1
`,
			`
networks:
- type: UnderlayNetwork
  name: uplink
  id: 1
  vlanID: 100
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"restart when underlay network vlanID is removed",
			`
networks:
- type: UnderlayNetwork
  name: uplink
  id: 1
  vlanID: 100
`,
			`
networks:
- type: UnderlayNetwork
  name: uplink
  id: 1
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"no restart when network ids are assigned and vlanID is unchanged",
			`
networks:
- type: Main
- type: UnderlayNetwork
  name: uplink
  vlanID: 100
`,
			`
networks:
- type: Main
  id: 1
- type: UnderlayNetwork
  name: uplink
  id: 2
  vlanID: 100
`,
			nil,
			actionRequired(ActionNone),
		),
		Entry(
			"restart when a pci device is added",
			`
pciDevices:
- name: pci-a
`,
			`
pciDevices:
- name: pci-a
- name: pci-b
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("pciDevices", ChangeReplace),
			),
		),
		Entry(
			"restart when the first pci device is added",
			``,
			`
pciDevices:
- name: pci-a
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("pciDevices", ChangeAdd),
			),
		),
		Entry(
			"restart when the last pci device is removed",
			`
pciDevices:
- name: pci-a
`,
			``,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("pciDevices", ChangeRemove),
			),
		),
		Entry(
			"no restart when pci devices only change order",
			`
pciDevices:
- name: pci-a
- name: pci-b
`,
			`
pciDevices:
- name: pci-b
- name: pci-a
`,
			nil,
			assertNoChanges(),
		),
		Entry(
			"no restart when pci devices change between empty list and unset",
			`
pciDevices: []
`,
			``,
			nil,
			assertNoChanges(),
		),
		Entry(
			"restart on virtualMachineIPAddressName change reports the spec field path",
			`
virtualMachineIPAddressName: ip-a
`,
			`
virtualMachineIPAddressName: ip-b
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("virtualMachineIPAddressName", ChangeReplace),
			),
		),
		Entry(
			"restart when virtualMachineIPAddressName is set",
			``,
			`
virtualMachineIPAddressName: ip-a
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("virtualMachineIPAddressName", ChangeAdd),
			),
		),
		Entry(
			"restart on virtualMachineClassName change",
			`
virtualMachineClassName: class-a
`,
			`
virtualMachineClassName: class-b
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("virtualMachineClassName", ChangeReplace),
			),
		),
		Entry(
			"apply immediate on runPolicy change",
			`
runPolicy: AlwaysOn
`,
			`
runPolicy: Manual
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("runPolicy", ChangeReplace),
			),
		),
		Entry(
			"no action on disruptions.restartApprovalMode change",
			`
disruptions:
  restartApprovalMode: Manual
`,
			`
disruptions:
  restartApprovalMode: Automatic
`,
			nil,
			assertChanges(
				actionRequired(ActionNone),
				requirePathOperation("disruptions.restartApprovalMode", ChangeReplace),
			),
		),
		Entry(
			"restart on priorityClassName change",
			`
priorityClassName: low
`,
			`
priorityClassName: high
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("priorityClassName", ChangeReplace),
			),
		),
		Entry(
			"restart on topologySpreadConstraints add",
			``,
			`
topologySpreadConstraints:
- maxSkew: 1
  topologyKey: kubernetes.io/hostname
  whenUnsatisfiable: ScheduleAnyway
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("topologySpreadConstraints", ChangeAdd),
			),
		),
		Entry(
			"edition placement action on nodeSelector change",
			`
nodeSelector:
  zone: a
`,
			`
nodeSelector:
  zone: b
`,
			nil,
			assertChanges(
				actionRequired(placementActionForEdition()),
				requirePathOperation("nodeSelector", ChangeReplace),
			),
		),
		Entry(
			"edition placement action on affinity add",
			``,
			`
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
      - matchExpressions:
        - key: zone
          operator: In
          values: ["a"]
`,
			nil,
			assertChanges(
				actionRequired(placementActionForEdition()),
				requirePathOperation("affinity", ChangeAdd),
			),
		),
		Entry(
			"edition placement action on tolerations add",
			``,
			`
tolerations:
- key: dedicated
  operator: Exists
  effect: NoSchedule
`,
			nil,
			assertChanges(
				actionRequired(placementActionForEdition()),
				requirePathOperation("tolerations", ChangeAdd),
			),
		),
		Entry(
			"restart on terminationGracePeriodSeconds change",
			`
terminationGracePeriodSeconds: 60
`,
			`
terminationGracePeriodSeconds: 120
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("terminationGracePeriodSeconds", ChangeReplace),
			),
		),
		Entry(
			"no action when terminationGracePeriodSeconds is set to the default",
			``,
			`
terminationGracePeriodSeconds: 60
`,
			nil,
			assertChanges(
				actionRequired(ActionNone),
				requirePathOperation("terminationGracePeriodSeconds", ChangeAdd),
			),
		),
		Entry(
			"no action when spice.enabled is set to the default",
			``,
			`
spice:
  enabled: false
`,
			nil,
			assertChanges(
				actionRequired(ActionNone),
				requirePathOperation("spice.enabled", ChangeAdd),
			),
		),
		Entry(
			"restart on osType change",
			`
osType: Generic
`,
			`
osType: Windows
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("osType", ChangeReplace),
			),
		),
		Entry(
			"no action when osType is set to the default",
			``,
			`
osType: Generic
`,
			nil,
			assertChanges(
				actionRequired(ActionNone),
				requirePathOperation("osType", ChangeAdd),
			),
		),
		Entry(
			"restart on bootloader change",
			`
bootloader: BIOS
`,
			`
bootloader: EFI
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("bootloader", ChangeReplace),
			),
		),
		Entry(
			"no action when bootloader is set to the default",
			``,
			`
bootloader: BIOS
`,
			nil,
			assertChanges(
				actionRequired(ActionNone),
				requirePathOperation("bootloader", ChangeAdd),
			),
		),
		Entry(
			"restart with a reason on cpu.cores change across the socket boundary even with hotplug",
			`
cpu:
  cores: 16
`,
			`
cpu:
  cores: 18
`,
			[]featuregate.Feature{featuregates.HotplugCPUWithLiveMigration},
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("cpu.cores", ChangeReplace),
				requireRestartMessage("cpu.cores", "sockets"),
			),
		),
		Entry(
			"apply immediate on cpu.cores change within one socket with hotplug",
			`
cpu:
  cores: 2
`,
			`
cpu:
  cores: 4
`,
			[]featuregate.Feature{featuregates.HotplugCPUWithLiveMigration},
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("cpu.cores", ChangeReplace),
			),
		),
		Entry(
			"restart on blockDeviceRefs add disk with paravirt disabled",
			`
enableParavirtualization: false
blockDeviceRefs:
- kind: VirtualDisk
  name: vd1
`,
			`
enableParavirtualization: false
blockDeviceRefs:
- kind: VirtualDisk
  name: vd1
- kind: VirtualDisk
  name: vd2
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("blockDeviceRefs.1", ChangeAdd),
			),
		),
		Entry(
			"restart on sysprepRef name change",
			`
provisioning:
  type: SysprepRef
  sysprepRef:
    kind: Secret
    name: sysprep-a
`,
			`
provisioning:
  type: SysprepRef
  sysprepRef:
    kind: Secret
    name: sysprep-b
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning.sysprepRef.name", ChangeReplace),
			),
		),
		Entry(
			"restart on inline userData change",
			`
provisioning:
  type: UserData
  userData: "#cloud-config\n"
`,
			`
provisioning:
  type: UserData
  userData: "#cloud-config\nhostname: vm\n"
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning.userData", ChangeReplace),
			),
		),
		Entry(
			"restart when an underlay network is added",
			`
networks:
- type: Main
  id: 1
`,
			`
networks:
- type: Main
  id: 1
- type: UnderlayNetwork
  name: uplink
  id: 2
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"restart when an underlay network is removed",
			`
networks:
- type: Main
  id: 1
- type: UnderlayNetwork
  name: uplink
  id: 2
`,
			`
networks:
- type: Main
  id: 1
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		Entry(
			"apply immediate when a disk is replaced at the same position",
			`
blockDeviceRefs:
- kind: VirtualDisk
  name: vd1
`,
			`
blockDeviceRefs:
- kind: VirtualDisk
  name: vd2
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("blockDeviceRefs.0", ChangeReplace),
			),
		),
		Entry(
			"apply immediate when a usb device is added",
			``,
			`
usbDevices:
- name: usb-a
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("usbDevices", ChangeAdd),
			),
		),
		Entry(
			"apply immediate when a usb device is removed",
			`
usbDevices:
- name: usb-a
- name: usb-b
`,
			`
usbDevices:
- name: usb-a
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("usbDevices", ChangeReplace),
			),
		),
		Entry(
			"no changes when terminationGracePeriodSeconds is set to the same value",
			`
terminationGracePeriodSeconds: 120
`,
			`
terminationGracePeriodSeconds: 120
`,
			nil,
			assertNoChanges(),
		),
		// The networks validator forbids changing an id on a running VM, so this case does not
		// reach the comparator in a cluster. It pins the current action for a direct change.
		Entry(
			"apply immediate when an additional network id changes",
			`
networks:
- type: Network
  name: net1
  id: 2
`,
			`
networks:
- type: Network
  name: net1
  id: 3
`,
			nil,
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("networks", ChangeReplace),
			),
		),
		// The validator rejects an unparsable coreFraction. The comparator treats such a value
		// as not Guaranteed, so it never forces a restart on its own.
		Entry(
			"apply immediate on an unparsable cpu.coreFraction with hotplug",
			`
cpu:
  cores: 2
  coreFraction: 50%
`,
			`
cpu:
  cores: 2
  coreFraction: abc
`,
			[]featuregate.Feature{featuregates.HotplugCPUWithLiveMigration},
			assertChanges(
				actionRequired(ActionApplyImmediate),
				requirePathOperation("cpu.coreFraction", ChangeReplace),
			),
		),
		Entry(
			"restart when sysprepRef is set on a SysprepRef provisioning",
			`
provisioning:
  type: SysprepRef
`,
			`
provisioning:
  type: SysprepRef
  sysprepRef:
    kind: Secret
    name: sysprep-a
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning.sysprepRef", ChangeAdd),
			),
		),
		Entry(
			"restart on sysprepRef kind change",
			`
provisioning:
  type: SysprepRef
  sysprepRef:
    kind: Secret
    name: sysprep-a
`,
			`
provisioning:
  type: SysprepRef
  sysprepRef:
    kind: ConfigMap
    name: sysprep-a
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning.sysprepRef.kind", ChangeReplace),
			),
		),
		Entry(
			"restart when userDataRef is set on a UserDataRef provisioning",
			`
provisioning:
  type: UserDataRef
`,
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: Secret
    name: cloud-init-a
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning.userDataRef", ChangeAdd),
			),
		),
		Entry(
			"restart on userDataRef kind change",
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: Secret
    name: cloud-init-a
`,
			`
provisioning:
  type: UserDataRef
  userDataRef:
    kind: ConfigMap
    name: cloud-init-a
`,
			nil,
			assertChanges(
				actionRequired(ActionRestart),
				requirePathOperation("provisioning.userDataRef.kind", ChangeReplace),
			),
		),
		Entry(
			"no changes for an unknown provisioning type",
			`
provisioning:
  type: Unknown
`,
			`
provisioning:
  type: Unknown
`,
			nil,
			assertNoChanges(),
		),
	)
})

var _ = Describe("CompareClassSpecs", func() {
	DescribeTable("returns the action a VirtualMachineClass spec change requires",
		func(currentSpec, desiredSpec string, assertFn func(changes SpecChanges)) {
			var current, desired v1alpha2.VirtualMachineClassSpec
			Expect(yaml.Unmarshal([]byte(currentSpec), &current)).To(Succeed())
			Expect(yaml.Unmarshal([]byte(desiredSpec), &desired)).To(Succeed())

			assertFn(CompareClassSpecs(&current, &desired))
		},
		Entry(
			"no changes for an equal class spec",
			`
nodeSelector:
  matchLabels:
    zone: a
`,
			`
nodeSelector:
  matchLabels:
    zone: a
`,
			assertNoChanges(),
		),
		Entry(
			"edition placement action on class nodeSelector matchLabels change",
			`
nodeSelector:
  matchLabels:
    zone: a
`,
			`
nodeSelector:
  matchLabels:
    zone: b
`,
			assertChanges(
				actionRequired(placementActionForEdition()),
				requirePathOperation("VirtualMachineClass:spec.nodeSelector", ChangeReplace),
			),
		),
		Entry(
			"edition placement action on class nodeSelector matchExpressions add",
			``,
			`
nodeSelector:
  matchExpressions:
  - key: zone
    operator: In
    values: ["a"]
`,
			assertChanges(
				actionRequired(placementActionForEdition()),
				requirePathOperation("VirtualMachineClass:spec.nodeSelector", ChangeAdd),
			),
		),
		Entry(
			"edition placement action on class tolerations add",
			``,
			`
tolerations:
- key: dedicated
  operator: Exists
  effect: NoSchedule
`,
			assertChanges(
				actionRequired(placementActionForEdition()),
				requirePathOperation("VirtualMachineClass:spec.tolerations", ChangeAdd),
			),
		),
		Entry(
			"no changes when only the class cpu or sizing policies differ",
			`
cpu:
  type: Host
`,
			`
cpu:
  type: Discovery
sizingPolicies:
- cores:
    min: 1
    max: 4
  coreFractions: [10, 50]
`,
			assertNoChanges(),
		),
	)
})

var _ = Describe("SpecChanges", func() {
	It("upgrades hotplug compute and block device changes to restart", func() {
		const (
			cpuReason   = "cpu restart reason"
			quotaReason = "quota restart reason"
		)

		changes := SpecChanges{}
		changes.Add(
			FieldChange{Path: "cpu.cores", ActionRequired: ActionApplyImmediate, RestartMessage: cpuReason},
			FieldChange{Path: "memory.size", ActionRequired: ActionApplyImmediate},
			FieldChange{Path: "blockDeviceRefs.0", ActionRequired: ActionApplyImmediate},
		)

		changes.UpgradeHotplugComputeChangesToRestart()
		changes.UpgradeBlockDeviceChangesToRestartMatching(nil)

		Expect(changes.GetAll()[0].ActionRequired).To(Equal(ActionRestart))
		Expect(changes.GetAll()[0].RestartMessage).To(Equal(cpuReason))
		Expect(changes.GetAll()[1].ActionRequired).To(Equal(ActionRestart))
		Expect(changes.GetAll()[2].ActionRequired).To(Equal(ActionRestart))

		changes = SpecChanges{}
		changes.Add(FieldChange{Path: "memory.size", ActionRequired: ActionApplyImmediate})

		changes.UpgradeHotplugComputeChangesToRestartWithMessage(quotaReason)

		Expect(changes.GetAll()[0].ActionRequired).To(Equal(ActionRestart))
		Expect(changes.GetAll()[0].RestartMessage).To(Equal(quotaReason))

		changes = SpecChanges{}
		changes.Add(
			FieldChange{Path: "cpu.cores", ActionRequired: ActionApplyImmediate},
			FieldChange{Path: "memory.size", ActionRequired: ActionApplyImmediate},
		)

		changes.UpgradeHotplugComputeChangesToRestartWithMessage(quotaReason)

		Expect(changes.GetAll()[0].ActionRequired).To(Equal(ActionRestart))
		Expect(changes.GetAll()[1].ActionRequired).To(Equal(ActionRestart))
		Expect(changes.GetRestartMessages()).To(Equal([]string{quotaReason}))
	})

	It("treats an unknown action as equal to any other", func() {
		Expect(ActionType("Unknown").Cmp(ActionRestart)).To(Equal(0))
	})

	It("returns an error when a pending change cannot be serialized", func() {
		var changes SpecChanges
		changes.Add(FieldChange{Operation: ChangeReplace, Path: "cpu.cores", DesiredValue: make(chan int), ActionRequired: ActionRestart})
		_, err := changes.ConvertPendingRestartChanges()
		Expect(err).To(HaveOccurred())
	})

	It("treats two nil int64 pointers as equal", func() {
		Expect(isEqualPtrInt64(nil, nil)).To(BeTrue())
	})

	It("returns no changes from a comparator without a function", func() {
		Expect(vmSpecFieldComparator(nil).Compare(&v1alpha2.VirtualMachineSpec{}, &v1alpha2.VirtualMachineSpec{})).To(BeNil())
	})

	It("reports only changes that wait for a restart as pending", func() {
		var changes SpecChanges

		pending, err := changes.ConvertPendingRestartChanges()
		Expect(err).NotTo(HaveOccurred())
		Expect(pending).To(BeEmpty())
		Expect(changes.ActionType()).To(Equal(ActionNone))
		Expect(changes.IsDisruptive()).To(BeFalse())

		changes.Add(
			FieldChange{Operation: ChangeReplace, Path: "runPolicy", CurrentValue: "AlwaysOn", DesiredValue: "Manual", ActionRequired: ActionApplyImmediate},
			FieldChange{Operation: ChangeReplace, Path: "virtualMachineIPAddressName", CurrentValue: "ip-a", DesiredValue: "ip-b", ActionRequired: ActionRestart},
			FieldChange{Operation: ChangeReplace, Path: "disruptions.restartApprovalMode", CurrentValue: "Manual", DesiredValue: "Automatic", ActionRequired: ActionNone},
		)
		Expect(changes.IsDisruptive()).To(BeTrue())

		pending, err = changes.ConvertPendingRestartChanges()
		Expect(err).NotTo(HaveOccurred())
		Expect(pending).To(HaveLen(1))
		Expect(string(pending[0].Raw)).To(MatchJSON(`{"operation":"replace","path":"virtualMachineIPAddressName","currentValue":"ip-a","desiredValue":"ip-b"}`))
	})
})

func loadVMSpec(inYAML string) *v1alpha2.VirtualMachineSpec {
	var spec v1alpha2.VirtualMachineSpec
	ExpectWithOffset(1, yaml.Unmarshal([]byte(inYAML), &spec)).To(Succeed(), "Should load vm spec from '%s'", inYAML)

	return &spec
}

func assertChanges(asserts ...func(changes SpecChanges)) func(changes SpecChanges) {
	return func(changes SpecChanges) {
		for _, fn := range asserts {
			fn(changes)
		}
	}
}

func assertNoChanges() func(changes SpecChanges) {
	return func(changes SpecChanges) {
		Expect(changes.IsEmpty()).To(BeTrue(), "expected no changes, got:\n%s", changesToYAML(changes))
	}
}

func actionRequired(actionType ActionType) func(changes SpecChanges) {
	return func(changes SpecChanges) {
		Expect(changes.ActionType()).To(Equal(actionType), "changes:\n%s", changesToYAML(changes))
	}
}

func requirePathOperation(path string, operation ChangeOperation) func(changes SpecChanges) {
	return func(changes SpecChanges) {
		change, found := findChange(changes, path)
		Expect(found).To(BeTrue(), "changes should contain path=%s, got:\n%s", path, changesToYAML(changes))
		Expect(change.Operation).To(Equal(operation), "operation for path=%s", path)
	}
}

func requireRestartMessage(path, substr string) func(changes SpecChanges) {
	return func(changes SpecChanges) {
		change, found := findChange(changes, path)
		Expect(found).To(BeTrue(), "changes should contain path=%s, got:\n%s", path, changesToYAML(changes))
		Expect(change.RestartMessage).To(ContainSubstring(substr), "restart message for path=%s", path)
	}
}

func findChange(changes SpecChanges, path string) (FieldChange, bool) {
	for _, change := range changes.GetAll() {
		if change.Path == path {
			return change, true
		}
	}

	return FieldChange{}, false
}

func changesToYAML(changes SpecChanges) string {
	status := map[string]interface{}{
		"status": map[string]interface{}{
			"pendingChanges": json.RawMessage(changes.ToJSON()),
		},
	}

	res, _ := yaml.Marshal(status)

	return string(res)
}

func newFeatureGate(enabled ...featuregate.Feature) featuregate.FeatureGate {
	gate, setFromMap, err := featuregates.NewUnlocked()
	Expect(err).NotTo(HaveOccurred())

	featureMap := map[string]bool{}
	for _, feature := range enabled {
		featureMap[string(feature)] = true
	}

	Expect(setFromMap(featureMap)).To(Succeed())

	return gate
}

// placementActionForEdition mirrors the edition gate: commercial builds (-tags EE) migrate
// on a placement change, the CE build waits for a restart. Run the tests with and without
// -tags EE to cover both branches.
func placementActionForEdition() ActionType {
	if featuregates.Default().Enabled(featuregates.AutoMigrationIfNodePlacementChanged) {
		return ActionApplyImmediate
	}

	return ActionRestart
}
