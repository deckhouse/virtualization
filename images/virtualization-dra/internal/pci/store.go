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

package pci

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/containerd/nri/pkg/api"
	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"

	"github.com/deckhouse/virtualization-dra/internal/cdi"
)

const (
	envClaimUIDPrefix    = "DRA_PCI_CLAIM_UID_"
	envDeviceNameSuffix  = "_DEVICE_NAME"
	defaultRescanTimeout = 5 * time.Minute
)

// NewAllocationStore builds the PCI Allocator: it scans the PCI bus on start
// and on a fixed interval (PCI topology rarely changes, an event source is
// not worth the machinery), publishes passthrough-capable devices, and binds
// IOMMU groups to vfio-pci on Prepare.
func NewAllocationStore(ctx context.Context, nodeName string, cdiManager cdi.Manager, rescanInterval time.Duration) (*AllocationStore, error) {
	if rescanInterval <= 0 {
		rescanInterval = defaultRescanTimeout
	}

	store := &AllocationStore{
		nodeName:                 nodeName,
		cdi:                      cdiManager,
		fs:                       newSysfs(""),
		devRoot:                  defaultDevRoot,
		log:                      slog.With(slog.String("component", "pci-allocation-store")),
		updateChannel:            make(chan resourceslice.DriverResources, 2),
		allocatableDevices:       make(map[string]Device),
		allocatedDevices:         sets.New[string](),
		resourceClaimAllocations: make(map[types.UID][]string),
	}

	if err := store.cdi.CreateCommonSpecFile(); err != nil {
		return nil, fmt.Errorf("failed to create CDI common spec file: %w", err)
	}

	store.startRescan(ctx, rescanInterval)

	return store, nil
}

// AllocationStore is the Allocator for PCI passthrough: discovers host PCI
// devices eligible for VFIO passthrough, publishes them as ResourceSlice
// devices, and on Prepare binds the device's whole IOMMU group to vfio-pci,
// injecting /dev/vfio/* nodes via CDI. Unprepare returns the group to the
// default kernel drivers. Synchronize restores allocations from container
// env after a plugin restart.
type AllocationStore struct {
	nodeName string

	cdi     cdi.Manager
	fs      sysfs
	devRoot string
	log     *slog.Logger

	updateChannel chan resourceslice.DriverResources
	mu            sync.RWMutex

	discoveryInited          bool
	allocatableDevices       map[string]Device
	allocatedDevices         sets.Set[string]
	resourceClaimAllocations map[types.UID][]string

	synchronized bool
}

func (s *AllocationStore) startRescan(ctx context.Context, interval time.Duration) {
	syncFunc := func() {
		if err := s.sync(ctx); err != nil {
			s.log.Error("failed to sync pci state", slog.Any("err", err))
		}
	}
	go func() {
		syncFunc()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncFunc()
			}
		}
	}()
}

func (s *AllocationStore) sync(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	discovered, err := s.fs.discoverDevices(s.log)
	if err != nil {
		return err
	}

	devicesByName := make(map[string]Device, len(discovered))
	for _, device := range discovered {
		devicesByName[device.GetName(s.nodeName)] = device
	}

	if s.discoveryInited && maps.Equal(devicesByName, s.allocatableDevices) {
		return nil
	}

	s.allocatableDevices = devicesByName
	s.discoveryInited = true

	select {
	case s.updateChannel <- s.makeResources(devicesByName):
	case <-ctx.Done():
	}

	return nil
}

func (s *AllocationStore) UpdateChannel() chan resourceslice.DriverResources {
	return s.updateChannel
}

func (s *AllocationStore) Prepare(_ context.Context, claim *resourcev1.ResourceClaim) ([]*drapbv1.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.synchronized {
		return nil, fmt.Errorf("prepare called before synchronize NRI Hook")
	}

	if claim.Status.Allocation == nil {
		return nil, fmt.Errorf("claim %s/%s has no allocation", claim.Namespace, claim.Name)
	}

	claimUID := string(claim.UID)

	preparedDevices := make(cdi.PreparedDevices, 0, len(claim.Status.Allocation.Devices.Results))

	for _, result := range claim.Status.Allocation.Devices.Results {
		if s.allocatedDevices.Has(result.Device) {
			return nil, fmt.Errorf("device %v is already allocated", result.Device)
		}

		device, exists := s.allocatableDevices[result.Device]
		if !exists {
			return nil, fmt.Errorf("requested device is not allocatable: %v", result.Device)
		}

		if err := s.fs.bindGroupToVFIO(device.IOMMUGroup); err != nil {
			return nil, err
		}

		edits, err := s.makeContainerEdits(claimUID, result.Device, &device)
		if err != nil {
			return nil, err
		}

		preparedDevices = append(preparedDevices, &cdi.PreparedDevice{
			Device: drapbv1.Device{
				RequestNames: []string{result.Request},
				PoolName:     result.Pool,
				DeviceName:   result.Device,
				CDIDeviceIDs: s.cdi.GetClaimDevices(claimUID, result.Device),
			},
			ContainerEdits: edits,
		})
	}

	if err := s.cdi.CreateClaimSpecFile(claimUID, preparedDevices); err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for claim: %w", err)
	}

	devices := preparedDevices.GetDevices()
	for _, device := range devices {
		s.allocatedDevices.Insert(device.DeviceName)
		s.resourceClaimAllocations[claim.UID] = append(s.resourceClaimAllocations[claim.UID], device.DeviceName)
	}

	return devices, nil
}

func (s *AllocationStore) makeContainerEdits(claimUID, deviceName string, device *Device) (*cdiapi.ContainerEdits, error) {
	nodes, err := vfioGroupDeviceNodes(s.devRoot, device.IOMMUGroup)
	if err != nil {
		return nil, err
	}

	claimUIDUpper := strings.ToUpper(claimUID)
	deviceNameUpper := strings.ToUpper(deviceName)

	deviceNodes := make([]*cdispec.DeviceNode, 0, len(nodes))
	for _, node := range nodes {
		deviceNodes = append(deviceNodes, &cdispec.DeviceNode{
			Path:        node.Path,
			HostPath:    node.Path,
			Type:        "c",
			Major:       node.Major,
			Minor:       node.Minor,
			Permissions: "mrw",
		})
	}

	return &cdiapi.ContainerEdits{
		ContainerEdits: &cdispec.ContainerEdits{
			Env: []string{
				fmt.Sprintf("%s%s=%s", envClaimUIDPrefix, claimUIDUpper, claimUID),
				fmt.Sprintf("%s%s%s=%s", envClaimUIDPrefix, claimUIDUpper, envDeviceNameSuffix, deviceName),
				fmt.Sprintf("DRA_PCI_%s_ADDRESS=%s", deviceNameUpper, device.Address),
			},
			DeviceNodes: deviceNodes,
		},
	}, nil
}

func (s *AllocationStore) Unprepare(_ context.Context, claimUID types.UID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.synchronized {
		return fmt.Errorf("unprepare called before synchronize NRI Hook")
	}

	allocatedDevices, exists := s.resourceClaimAllocations[claimUID]
	if !exists || len(allocatedDevices) == 0 {
		s.log.Info("Claim has no tracked allocations, skipping device cleanup", slog.String("claimUID", string(claimUID)))
	} else {
		s.log.Info("Unpreparing devices", slog.Any("devices", allocatedDevices), slog.String("claimUID", string(claimUID)))

		for _, deviceName := range allocatedDevices {
			s.allocatedDevices.Delete(deviceName)

			device, known := s.allocatableDevices[deviceName]
			if !known {
				s.log.Warn("Device is no longer present on the host, skipping driver restore", slog.String("device", deviceName))
				continue
			}

			if s.groupInUse(device.IOMMUGroup) {
				s.log.Info("IOMMU group is still used by another allocated device, skipping driver restore", slog.Int("iommuGroup", device.IOMMUGroup))
				continue
			}

			if err := s.fs.restoreGroupDrivers(device.IOMMUGroup); err != nil {
				return err
			}
		}
	}

	s.log.Info("Deleting CDI claim spec file", slog.String("claimUID", string(claimUID)))
	if err := s.cdi.DeleteClaimSpecFile(string(claimUID)); err != nil {
		return fmt.Errorf("unable to delete CDI spec file for claim: %w", err)
	}

	delete(s.resourceClaimAllocations, claimUID)

	return nil
}

func (s *AllocationStore) groupInUse(group int) bool {
	for deviceName := range s.allocatedDevices {
		if device, known := s.allocatableDevices[deviceName]; known && device.IOMMUGroup == group {
			return true
		}
	}
	return false
}

func (s *AllocationStore) Synchronize(_ context.Context, pods []*api.PodSandbox, containers []*api.Container) ([]*api.ContainerUpdate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	containersByPodSandboxID := make(map[string][]*api.Container, len(pods))
	for _, ctr := range containers {
		containersByPodSandboxID[ctr.PodSandboxId] = append(containersByPodSandboxID[ctr.PodSandboxId], ctr)
	}

	for _, pod := range pods {
		for _, ctr := range containersByPodSandboxID[pod.Id] {
			claimUIDDeviceNames, err := parseDraEnvToClaimAllocations(ctr.Env)
			if err != nil {
				s.log.Error("failed to parse dra env", slog.String("name", pod.Name), slog.String("namespace", pod.Namespace), slog.Any("err", err))
				continue
			}
			for claimUID, deviceNames := range claimUIDDeviceNames {
				s.resourceClaimAllocations[claimUID] = append(s.resourceClaimAllocations[claimUID], deviceNames...)
				for _, deviceName := range deviceNames {
					s.log.Info("Found allocated device", slog.String("claimUID", string(claimUID)), slog.String("deviceName", deviceName))
					s.allocatedDevices.Insert(deviceName)
				}
			}
		}
	}

	s.synchronized = true

	return nil, nil
}

func parseDraEnvToClaimAllocations(envs []string) (map[types.UID][]string, error) {
	result := make(map[types.UID][]string)

	for _, env := range envs {
		key, value, found := strings.Cut(env, "=")
		if !found {
			return nil, fmt.Errorf("invalid dra env: %s", env)
		}

		if strings.HasPrefix(key, envClaimUIDPrefix) && strings.HasSuffix(key, envDeviceNameSuffix) {
			uid := strings.TrimPrefix(key, envClaimUIDPrefix)
			uid = strings.TrimSuffix(uid, envDeviceNameSuffix)
			claimUID := types.UID(strings.ToLower(uid))

			result[claimUID] = append(result[claimUID], value)
		}
	}

	return result, nil
}

func (s *AllocationStore) makeResources(devicesByName map[string]Device) resourceslice.DriverResources {
	if len(devicesByName) == 0 {
		return resourceslice.DriverResources{}
	}

	devices := make([]resourcev1.Device, 0, len(devicesByName))
	for _, device := range devicesByName {
		devices = append(devices, *device.ToAPIDevice(s.nodeName))
	}

	return resourceslice.DriverResources{
		Pools: map[string]resourceslice.Pool{
			s.nodeName: {
				Slices: []resourceslice.Slice{
					{
						Devices: devices,
					},
				},
			},
		},
	}
}
