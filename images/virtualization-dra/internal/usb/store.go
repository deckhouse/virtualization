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

package usb

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/containerd/nri/pkg/api"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"k8s.io/utils/ptr"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"

	"github.com/deckhouse/virtualization-dra/internal/cdi"
	"github.com/deckhouse/virtualization-dra/pkg/set"
)

const DefaultResyncPeriod = 10 * time.Minute

func NewAllocationStore(nodeName, devicesPath string, resyncPeriod time.Duration, cdiManager cdi.Manager, log *slog.Logger) *AllocationStore {
	if resyncPeriod == 0 {
		resyncPeriod = DefaultResyncPeriod
	}
	if devicesPath == "" {
		devicesPath = PathToUSBDevices
	}
	store := &AllocationStore{
		nodeName:                  nodeName,
		devicesPath:               devicesPath,
		resyncPeriod:              resyncPeriod,
		cdi:                       cdiManager,
		log:                       log.With(slog.String("component", "usb-allocation-store")),
		updateChannel:             make(chan []resourceapi.Device, 2),
		discoverPluggedUSBDevices: NewDeviceSet(),
		allocatableDevices:        make(map[string]resourceapi.Device),
		allocatedDevices:          set.New[string](),
		resourceClaimAllocations:  make(map[types.UID][]string),
	}

	monitor := newUSBMonitor(monitorCallback{
		Add:    store.genericCallback,
		Update: store.genericCallback,
		Delete: store.genericCallback,
	})

	store.monitor = monitor

	return store
}

type AllocationStore struct {
	nodeName     string
	devicesPath  string
	resyncPeriod time.Duration

	cdi cdi.Manager
	log *slog.Logger

	monitor *monitor

	updateChannel chan []resourceapi.Device
	mu            sync.RWMutex

	discoverPluggedUSBDevices *DeviceSet
	allocatableDevices        map[string]resourceapi.Device
	allocatedDevices          *set.Set[string]
	resourceClaimAllocations  map[types.UID][]string
}

func (s *AllocationStore) sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	discoverPluggedUSBDevices, err := discoverPluggedUSBDevices(s.devicesPath)
	if err != nil {
		return err
	}
	if discoverPluggedUSBDevices.Equal(s.discoverPluggedUSBDevices) {
		return nil
	}
	s.discoverPluggedUSBDevices = discoverPluggedUSBDevices

	allocatableDevices := make([]resourceapi.Device, discoverPluggedUSBDevices.Len())
	for i, usbDevice := range discoverPluggedUSBDevices.Slice() {
		allocatableDevices[i] = *convertToAPIDevice(usbDevice)
	}

	allocatableDevicesByName := make(map[string]resourceapi.Device, len(allocatableDevices))
	for _, device := range allocatableDevices {
		allocatableDevicesByName[device.Name] = device
	}

	s.allocatableDevices = allocatableDevicesByName

	s.updateChannel <- allocatableDevices

	return nil
}

func (s *AllocationStore) genericCallback() {
	if err := s.sync(); err != nil {
		s.log.Error("failed to sync usb state", slog.Any("err", err))
	}
}

func (s *AllocationStore) Start(ctx context.Context) error {
	if err := s.cdi.CreateCommonSpecFile(); err != nil {
		return fmt.Errorf("failed to create CDI common spec file: %w", err)
	}

	doSync := func() {
		err := s.sync()
		if err != nil {
			s.log.Error("failed to sync usb state", slog.Any("err", err))
		}
	}
	ticker := time.NewTicker(s.resyncPeriod)
	go func() {
		doSync()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				doSync()
			}
		}
	}()

	s.monitor.Start(ctx)

	return nil
}

func (s *AllocationStore) UpdateChannel() chan []resourceapi.Device {
	return s.updateChannel
}

func (s *AllocationStore) Prepare(_ context.Context, claim *resourceapi.ResourceClaim) ([]*drapbv1.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if claim.Status.Allocation == nil {
		return nil, fmt.Errorf("claim %s/%s has no allocation", claim.Namespace, claim.Name)
	}

	claimUID := string(claim.UID)

	preparedDevices := make(cdi.PreparedDevices, len(claim.Status.Allocation.Devices.Results))
	for i, result := range claim.Status.Allocation.Devices.Results {
		usbDevice, exists := s.allocatableDevices[result.Device]
		if !exists {
			return nil, fmt.Errorf("requested device is not allocatable: %v", result.Device)
		}
		// TODO: unnecessary?
		//  kubernetes check allocatable devices
		//  Warning  FailedScheduling  8s    default-scheduler  0/3 nodes are available:
		//  1 node(s) had tolerated taint {node-role.kubernetes.io/control-plane: },
		//  2 cannot allocate all claims.
		//  still not schedulable, preemption: 0/3 nodes are available: 3 Preemption is not helpful for scheduling.
		if s.allocatedDevices.Contains(result.Device) {
			return nil, fmt.Errorf("device %v is already allocated", result.Device)
		}

		edits, err := s.makeContainerEdits(claimUID, &usbDevice)
		if err != nil {
			return nil, err
		}
		device := cdi.PreparedDevice{
			Device: drapbv1.Device{
				RequestNames: []string{result.Request},
				PoolName:     result.Pool,
				DeviceName:   result.Device,
				CDIDeviceIDs: s.cdi.GetClaimDevices(claimUID, result.Device),
			},
			ContainerEdits: edits,
		}
		preparedDevices[i] = &device
	}

	err := s.cdi.CreateClaimSpecFile(claimUID, preparedDevices)
	if err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for claim: %v", err)
	}

	devices := preparedDevices.GetDevices()
	for _, device := range devices {
		s.allocatedDevices.Add(device.DeviceName)
		s.resourceClaimAllocations[claim.UID] = append(s.resourceClaimAllocations[claim.UID], device.DeviceName)
	}

	return devices, nil
}

// TODO: refactor me
func (s *AllocationStore) makeContainerEdits(claimUID string, device *resourceapi.Device) (*cdiapi.ContainerEdits, error) {
	var (
		devicePath string
		deviceNum  string
		bus        string
		major      int64
		minor      int64
	)

	if attr, ok := device.Attributes["devicePath"]; ok {
		if val := attr.StringValue; val != nil {
			devicePath = *val
		} else {
			return nil, fmt.Errorf("devicePath attribute is not exist")
		}
	}

	if attr, ok := device.Attributes["deviceNumber"]; ok {
		if val := attr.StringValue; val != nil {
			deviceNum = *val
		} else {
			return nil, fmt.Errorf("deviceNum attribute is not exist")
		}
	}

	if attr, ok := device.Attributes["bus"]; ok {
		if val := attr.StringValue; val != nil {
			bus = *val
		} else {
			return nil, fmt.Errorf("bus attribute is not exist")
		}
	}

	if attr, ok := device.Attributes["major"]; ok {
		if val := attr.IntValue; val != nil {
			major = *val
		} else {
			return nil, fmt.Errorf("major attribute is not exist")
		}
	}

	if attr, ok := device.Attributes["minor"]; ok {
		if val := attr.IntValue; val != nil {
			minor = *val
		} else {
			return nil, fmt.Errorf("minor attribute is not exist")
		}
	}

	claimUIDUpper := strings.ToUpper(claimUID)
	deviceNameUpper := strings.ToUpper(device.Name)

	edits := &cdiapi.ContainerEdits{
		ContainerEdits: &cdispec.ContainerEdits{
			Env: []string{
				fmt.Sprintf("DRA_USB_CLAIM_UID_%s=%s", claimUIDUpper, claimUID),
				fmt.Sprintf("DRA_USB_DEVICE_NAME_%s=%s", deviceNameUpper, device.Name),
				fmt.Sprintf("DRA_USB_CLAIM_UID_%s_DEVICE_NAME=%s", claimUIDUpper, device.Name),
				fmt.Sprintf("DRA_USB_%s_DEVICE_PATH=%s", deviceNameUpper, devicePath),
				fmt.Sprintf("DRA_USB_%s_BUS_DEVICENUMBER=%s:%s", deviceNameUpper, bus, deviceNum),
			},
			DeviceNodes: []*cdispec.DeviceNode{
				{
					Path:        devicePath,
					HostPath:    devicePath,
					Type:        "c",
					Major:       major,
					Minor:       minor,
					Permissions: "mrw",
					UID:         ptr.To(uint32(107)), // qemu user. TODO: make this configurable
					GID:         ptr.To(uint32(107)), // qemu group. TODO: make this configurable
				},
			},
		},
	}

	return edits, nil
}

func (s *AllocationStore) Unprepare(_ context.Context, claimUID types.UID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.cdi.DeleteClaimSpecFile(string(claimUID)); err != nil {
		return fmt.Errorf("unable to delete CDI spec file for claim: %w", err)
	}

	allocatedDevices := s.resourceClaimAllocations[claimUID]
	for _, device := range allocatedDevices {
		s.allocatedDevices.Remove(device)
	}
	delete(s.resourceClaimAllocations, claimUID)

	return nil
}

func (s *AllocationStore) Synchronize(_ context.Context, pods []*api.PodSandbox, containers []*api.Container) ([]*api.ContainerUpdate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	containersByPodSandboxId := make(map[string][]*api.Container, len(pods))
	for _, ctr := range containers {
		containersByPodSandboxId[ctr.PodSandboxId] = append(containersByPodSandboxId[ctr.PodSandboxId], ctr)
	}

	for _, pod := range pods {
		s.log.Info("Synchronize pod", slog.String("name", pod.Name), slog.String("namespace", pod.Namespace))
		ctrs := containersByPodSandboxId[pod.Id]

		for _, ctr := range ctrs {
			claimUIDDeviceNames, err := parseDraEnvToClaimAllocations(ctr.Env)
			if err != nil {
				s.log.Error("failed to parse dra env", slog.String("name", pod.Name), slog.String("namespace", pod.Namespace), slog.Any("err", err))
				continue
			}
			for claimUID, deviceNames := range claimUIDDeviceNames {
				s.resourceClaimAllocations[claimUID] = append(s.resourceClaimAllocations[claimUID], deviceNames...)
				for _, deviceName := range deviceNames {
					s.allocatedDevices.Add(deviceName)
				}
			}

		}
	}
	return nil, nil
}

func parseDraEnvToClaimAllocations(envs []string) (map[types.UID][]string, error) {
	result := make(map[types.UID][]string)

	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid dra env: %s", env)
		}
		key := parts[0]
		value := parts[1]

		if !strings.HasPrefix(key, "DRA_USB_CLAIM_UID_") || !strings.HasSuffix(key, "_DEVICE_NAME") {
			continue
		}
		uid := strings.TrimPrefix(key, "DRA_USB_CLAIM_UID_")
		uid = strings.TrimSuffix(uid, "_DEVICE_NAME")
		uid = strings.ToLower(uid)
		claimUID := types.UID(uid)

		deviceName := value

		result[claimUID] = append(result[claimUID], deviceName)
	}

	return result, nil
}
