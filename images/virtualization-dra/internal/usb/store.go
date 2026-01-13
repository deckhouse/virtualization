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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containerd/nri/pkg/api"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"k8s.io/utils/ptr"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"

	vdraapi "github.com/deckhouse/virtualization-dra/api"
	"github.com/deckhouse/virtualization-dra/internal/common"
	"github.com/deckhouse/virtualization-dra/internal/featuregates"

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
		updateChannel:             make(chan resourceslice.DriverResources, 2),
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

	updateChannel chan resourceslice.DriverResources
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
		allocatableDevices[i] = *convertToAPIDevice(usbDevice, s.nodeName)
	}

	allocatableDevicesByName := make(map[string]resourceapi.Device, len(allocatableDevices))
	for _, device := range allocatableDevices {
		allocatableDevicesByName[device.Name] = device
	}

	s.allocatableDevices = allocatableDevicesByName

	s.updateChannel <- s.makeResources(allocatableDevices)

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

func (s *AllocationStore) UpdateChannel() chan resourceslice.DriverResources {
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

		containerEditsOptions, err := newContainerEditsOptions(&usbDevice, claim)
		if err != nil {
			return nil, err
		}

		edits, err := s.makeContainerEdits(claimUID, containerEditsOptions)
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

func newContainerEditsOptions(device *resourceapi.Device, claim *resourceapi.ResourceClaim) (containerEditsOptions, error) {
	opts := containerEditsOptions{
		Name: device.Name,
	}

	if featuregates.Default().USBGatewayEnabled() && isUSBGateway(claim) {
		var data *runtime.RawExtension
		for _, deviceStatus := range claim.Status.Devices {
			if deviceStatus.Device == device.Name {
				data = deviceStatus.Data
				break
			}
		}
		if data == nil {
			return opts, fmt.Errorf("device status data is not found")
		}

		usbGatewayStatus, ok := data.Object.(*vdraapi.USBGatewayStatus)
		if !ok {
			return opts, fmt.Errorf("device status data is not a USBGatewayStatus")
		}
		if usbGatewayStatus == nil {
			return opts, fmt.Errorf("device status data Object is nil")
		}

		opts.Bus = strconv.Itoa(usbGatewayStatus.BusNum)
		opts.DeviceNum = strconv.Itoa(usbGatewayStatus.DeviceNum)
		opts.DevicePath = usbGatewayStatus.DevicePath

	} else {
		if attr, ok := device.Attributes["devicePath"]; ok {
			if val := attr.StringValue; val != nil {
				opts.DevicePath = *val
			} else {
				return opts, fmt.Errorf("devicePath attribute is not exist")
			}
		}

		if attr, ok := device.Attributes["deviceNumber"]; ok {
			if val := attr.StringValue; val != nil {
				opts.DeviceNum = *val
			} else {
				return opts, fmt.Errorf("deviceNum attribute is not exist")
			}
		}

		if attr, ok := device.Attributes["bus"]; ok {
			if val := attr.StringValue; val != nil {
				opts.Bus = *val
			} else {
				return opts, fmt.Errorf("bus attribute is not exist")
			}
		}
	}

	if attr, ok := device.Attributes["major"]; ok {
		if val := attr.IntValue; val != nil {
			opts.Major = *val
		} else {
			return opts, fmt.Errorf("major attribute is not exist")
		}
	}

	if attr, ok := device.Attributes["minor"]; ok {
		if val := attr.IntValue; val != nil {
			opts.Minor = *val
		} else {
			return opts, fmt.Errorf("minor attribute is not exist")
		}
	}

	return opts, nil
}

func isUSBGateway(claim *resourceapi.ResourceClaim) bool {
	return claim.Annotations[common.USBGatewayAnnotation] == "true"
}

type containerEditsOptions struct {
	Name       string
	DevicePath string
	DeviceNum  string
	Bus        string
	Major      int64
	Minor      int64
}

// TODO: refactor me
func (s *AllocationStore) makeContainerEdits(claimUID string, opts containerEditsOptions) (*cdiapi.ContainerEdits, error) {
	claimUIDUpper := strings.ToUpper(claimUID)
	deviceNameUpper := strings.ToUpper(opts.Name)

	edits := &cdiapi.ContainerEdits{
		ContainerEdits: &cdispec.ContainerEdits{
			Env: []string{
				fmt.Sprintf("DRA_USB_CLAIM_UID_%s=%s", claimUIDUpper, claimUID),
				fmt.Sprintf("DRA_USB_DEVICE_NAME_%s=%s", deviceNameUpper, opts.Name),
				fmt.Sprintf("DRA_USB_CLAIM_UID_%s_DEVICE_NAME=%s", claimUIDUpper, opts.Name),
				fmt.Sprintf("DRA_USB_%s_DEVICE_PATH=%s", deviceNameUpper, opts.DevicePath),
				fmt.Sprintf("DRA_USB_%s_BUS_DEVICENUMBER=%s:%s", deviceNameUpper, opts.Bus, opts.DeviceNum),
			},
			DeviceNodes: []*cdispec.DeviceNode{
				{
					Path:        opts.DevicePath,
					HostPath:    opts.DevicePath,
					Type:        "c",
					Major:       opts.Major,
					Minor:       opts.Minor,
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

func (s *AllocationStore) makeResources(devices []resourceapi.Device) resourceslice.DriverResources {
	slice := resourceslice.Slice{
		Devices: devices,
	}
	poolName := s.nodeName

	if featuregates.Default().USBGatewayEnabled() {
		slice.PerDeviceNodeSelection = ptr.To(true)
		poolName = "usb-gateway"
	}

	return resourceslice.DriverResources{
		Pools: map[string]resourceslice.Pool{
			poolName: {
				Slices: []resourceslice.Slice{slice},
			},
		},
	}
}
