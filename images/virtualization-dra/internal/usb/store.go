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
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/containerd/nri/pkg/api"
	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"k8s.io/utils/ptr"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"

	"github.com/deckhouse/virtualization-dra/internal/cdi"
	"github.com/deckhouse/virtualization-dra/internal/consts"
	"github.com/deckhouse/virtualization-dra/internal/featuregates"
	usbgateway "github.com/deckhouse/virtualization-dra/internal/usb-gateway"
	"github.com/deckhouse/virtualization-dra/pkg/libusb"
	"github.com/deckhouse/virtualization-dra/pkg/patch"
	"github.com/deckhouse/virtualization-dra/pkg/usbip"
)

func NewAllocationStore(ctx context.Context, nodeName string, cdiManager cdi.Manager, monitor libusb.Monitor, usbGateway usbgateway.USBGateway, kubeClient kubernetes.Interface) (*AllocationStore, error) {
	store := &AllocationStore{
		nodeName:                   nodeName,
		cdi:                        cdiManager,
		monitor:                    monitor,
		usbGateway:                 usbGateway,
		kubeClient:                 kubeClient,
		log:                        slog.With(slog.String("component", "usb-allocation-store")),
		updateChannel:              make(chan resourceslice.DriverResources, 2),
		discoverPluggedUSBDevices:  NewDeviceSet(),
		allocatableDevices:         make(map[string]resourcev1.Device),
		allocatedDevices:           sets.New[string](),
		usbipAllocatedDevicesCount: make(map[string]int),
		resourceClaimAllocations:   make(map[types.UID][]string),
		usbipInfoGetter:            usbip.NewUSBAttacher(),
	}

	store.subscribeToDeviceChanges(ctx)

	if err := store.cdi.CreateCommonSpecFile(); err != nil {
		return nil, fmt.Errorf("failed to create CDI common spec file: %w", err)
	}

	return store, nil
}

// AllocationStore is the Allocator for USB: discovers plugged devices, maintains allocatable pool, writes CDI specs on Prepare/Unprepare, pushes updates to ResourceSlice via UpdateChannel. Synchronize restores state from container env (e.g. after restart).
type AllocationStore struct {
	nodeName string

	cdi cdi.Manager
	log *slog.Logger

	updateChannel chan resourceslice.DriverResources
	mu            sync.RWMutex

	usbGateway      usbgateway.USBGateway
	usbipInfoGetter usbip.AttachInfoGetter
	monitor         libusb.Monitor
	kubeClient      kubernetes.Interface

	discoverPluggedUSBDevices      DeviceSet
	discoverUsbIpPluggedUSBDevices DeviceSet
	allocatableDevices             map[string]resourcev1.Device

	allocatedDevices           sets.Set[string]
	usbipAllocatedDevicesCount map[string]int
	resourceClaimAllocations   map[types.UID][]string
}

func (s *AllocationStore) sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	discoverPluggedUSBDevices, discoverUsbIpPluggedUSBDevices, err := s.discoveryPluggedUSBDevices()
	if err != nil {
		return err
	}

	s.discoverUsbIpPluggedUSBDevices = discoverUsbIpPluggedUSBDevices

	if discoverPluggedUSBDevices.Equal(s.discoverPluggedUSBDevices) {
		return nil
	}

	s.discoverPluggedUSBDevices = discoverPluggedUSBDevices

	allocatableDevices := make([]resourcev1.Device, discoverPluggedUSBDevices.Len())
	for i, usbDevice := range discoverPluggedUSBDevices.UnsortedList() {
		allocatableDevices[i] = *convertToAPIDevice(usbDevice, s.nodeName)
	}

	allocatableDevicesByName := make(map[string]resourcev1.Device, len(allocatableDevices))
	for _, device := range allocatableDevices {
		allocatableDevicesByName[device.Name] = device
	}

	s.allocatableDevices = allocatableDevicesByName

	s.updateChannel <- s.makeResources(allocatableDevices)

	return nil
}

func (s *AllocationStore) subscribeToDeviceChanges(ctx context.Context) {
	syncFunc := func() {
		if err := s.sync(); err != nil {
			s.log.Error("failed to sync usb state", slog.Any("err", err))
		}
	}
	go func() {
		syncFunc()
		changes := s.monitor.DeviceChanges()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-changes:
				if !ok {
					return
				}
				syncFunc()
			}
		}
	}()
}

func (s *AllocationStore) UpdateChannel() chan resourceslice.DriverResources {
	return s.updateChannel
}

func (s *AllocationStore) Prepare(ctx context.Context, claim *resourcev1.ResourceClaim) ([]*drapbv1.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if claim.Status.Allocation == nil {
		return nil, fmt.Errorf("claim %s/%s has no allocation", claim.Namespace, claim.Name)
	}

	claimUID := string(claim.UID)

	preparedDevices := make(cdi.PreparedDevices, len(claim.Status.Allocation.Devices.Results))

	usbGatewayEnabled := featuregates.Default().USBGatewayEnabled()
	usbNodeLocalMultiAllocationEnabled := featuregates.Default().USBNodeLocalMultiAllocationEnabled()

	usbIPAllocatedDevices := make(map[string]struct{})
	usbDeviceInfos := make([]usbDeviceInfo, 0)

	for i, result := range claim.Status.Allocation.Devices.Results {
		allocated := s.allocatedDevices.Has(result.Device)
		if allocated && !usbNodeLocalMultiAllocationEnabled {
			return nil, fmt.Errorf("device %v is already allocated. For USB node local multi allocation, please set feature gate %q to true", result.Device, featuregates.USBNodeLocalMultiAllocation)
		}

		isUSBGatewayRequest := s.isUSBGatewayRequest(&result)

		if !usbGatewayEnabled && isUSBGatewayRequest {
			return nil, fmt.Errorf("claim %s/%s has usb gateway request but usb gateway is disabled", claim.Namespace, claim.Name)
		}

		var containerEditsOptions containerEditsOptions

		if isUSBGatewayRequest {
			if !allocated {
				err := s.usbGateway.Attach(ctx, result.Device)
				if err != nil {
					return nil, err
				}
			}

			busID, err := s.usbGateway.GetAttachedBusID(result.Device)
			if err != nil {
				return nil, err
			}

			usbDevice := s.getUsbGatewayUsbDevice(busID)
			if usbDevice == nil {
				return nil, fmt.Errorf("usb device %s is not found", busID)
			}

			containerEditsOptions = newContainerEditsOptionsForUSBGateway(result.Device, usbDevice).withUserGroup(claim)

			usbIPAllocatedDevices[result.Device] = struct{}{}
			usbDeviceInfos = append(usbDeviceInfos, usbDeviceInfo{
				DeviceName: result.Device,
				UsbAddress: usbAddressFromDev(usbDevice),
			})
		} else {
			usbDevice, exists := s.allocatableDevices[result.Device]
			if !exists {
				return nil, fmt.Errorf("requested device is not allocatable: %v", result.Device)
			}

			opts, err := newContainerEditsOptions(&usbDevice)
			if err != nil {
				return nil, err
			}
			containerEditsOptions = opts.withUserGroup(claim)
			usbDeviceInfos = append(usbDeviceInfos, usbDeviceInfo{
				DeviceName: result.Device,
				UsbAddress: usbAddress(opts.Bus, opts.DeviceNum),
			})
		}

		edits := s.makeContainerEdits(claimUID, containerEditsOptions)

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
		return nil, fmt.Errorf("unable to create CDI spec file for claim: %w", err)
	}

	err = s.ensureAnnotationDeviceAddresses(ctx, claim, usbDeviceInfos)
	if err != nil {
		return nil, err
	}

	devices := preparedDevices.GetDevices()
	for _, device := range devices {
		s.allocatedDevices.Insert(device.DeviceName)
		s.resourceClaimAllocations[claim.UID] = append(s.resourceClaimAllocations[claim.UID], device.DeviceName)

		if _, ok := usbIPAllocatedDevices[device.DeviceName]; ok {
			s.usbipAllocatedDevicesCount[device.DeviceName]++
		}
	}

	return devices, nil
}

func (s *AllocationStore) getUsbGatewayUsbDevice(busID string) *Device {
	for _, device := range s.discoverUsbIpPluggedUSBDevices.UnsortedList() {
		if device.BusID == busID {
			return &device
		}
	}
	// usb device is not found in cache
	// load usb device from sysfs
	dev, err := libusb.LoadUSBDevice(filepath.Join(libusb.PathToUSBDevices, busID))
	if err == nil {
		return ptr.To(toDevice(&dev))
	}

	return nil
}

func (s *AllocationStore) ensureAnnotationDeviceAddresses(ctx context.Context, claim *resourcev1.ResourceClaim, usbDeviceInfos []usbDeviceInfo) error {
	path := fmt.Sprintf("/metadata/annotations/%s", patch.EscapeJSONPointer(consts.AnnUSBDeviceAddresses))

	slices.SortFunc(usbDeviceInfos, func(a, b usbDeviceInfo) int {
		return cmp.Compare(a.DeviceName, b.DeviceName)
	})

	oldAnno, oldUsbDeviceInfos, err := loadUsbDeviceInfos(claim)
	if err != nil {
		return err
	}

	jp := patch.NewJSONPatch()

	if oldUsbDeviceInfos == nil {
		jp.Append(patch.WithAdd(path, patch.AsJsonString{Data: usbDeviceInfos}))
	} else {
		slices.SortFunc(oldUsbDeviceInfos, func(a, b usbDeviceInfo) int {
			return cmp.Compare(a.DeviceName, b.DeviceName)
		})
		if slices.Equal(oldUsbDeviceInfos, usbDeviceInfos) {
			return nil
		}

		jp.Append(
			patch.WithTest(path, oldAnno),
			patch.WithReplace(path, patch.AsJsonString{Data: usbDeviceInfos}),
		)
	}

	bytes, err := jp.Bytes()
	if err != nil {
		return fmt.Errorf("failed to generate patch: %w", err)
	}

	s.log.Debug("Patching resource claim", slog.String("uid", string(claim.UID)), slog.Any("patch", string(bytes)))
	_, err = s.kubeClient.ResourceV1().ResourceClaims(claim.Namespace).Patch(ctx, claim.Name, types.JSONPatchType, bytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch resource claim: %w", err)
	}
	return nil
}

type usbDeviceInfo struct {
	DeviceName string `json:"deviceName"`
	UsbAddress string `json:"usbAddress"`
}

func loadUsbDeviceInfos(obj metav1.Object) (string, []usbDeviceInfo, error) {
	var usbDeviceInfos []usbDeviceInfo
	if data, ok := obj.GetAnnotations()[consts.AnnUSBDeviceAddresses]; ok {
		err := json.Unmarshal([]byte(data), &usbDeviceInfos)
		if err != nil {
			return "", nil, fmt.Errorf("failed to unmarshal annotation %s: %w", consts.AnnUSBDeviceAddresses, err)
		}
		return data, usbDeviceInfos, nil
	}
	return "", nil, nil
}

func newContainerEditsOptionsForUSBGateway(deviceName string, usbDevice *Device) containerEditsOptions {
	return containerEditsOptions{
		Name:       deviceName,
		DevicePath: usbDevice.DevicePath,
		DeviceNum:  usbDevice.DeviceNumber.String(),
		Bus:        usbDevice.Bus.String(),
		Major:      int64(usbDevice.Major),
		Minor:      int64(usbDevice.Minor),
	}
}

func (c containerEditsOptions) withUserGroup(claim *resourcev1.ResourceClaim) containerEditsOptions {
	if anno := claim.GetAnnotations()[consts.AnnUSBDeviceUser]; anno != "" {
		uid, err := strconv.ParseUint(anno, 10, 32)
		if err != nil {
			slog.Warn("Failed to parse annotation", slog.String("annotation", consts.AnnUSBDeviceUser), slog.String("value", anno), slog.Any("error", err))
		} else {
			c.UID = ptr.To(uint32(uid))
		}
	}
	if claim.GetAnnotations()[consts.AnnUSBDeviceGroup] != "" {
		gid, err := strconv.ParseUint(claim.GetAnnotations()[consts.AnnUSBDeviceGroup], 10, 32)
		if err != nil {
			slog.Warn("Failed to parse annotation", slog.String("annotation", consts.AnnUSBDeviceGroup), slog.String("value", claim.GetAnnotations()[consts.AnnUSBDeviceGroup]), slog.Any("error", err))
		}
		c.GID = ptr.To(uint32(gid))
	}
	return c
}

func newContainerEditsOptions(device *resourcev1.Device) (containerEditsOptions, error) {
	opts := containerEditsOptions{
		Name: device.Name,
	}

	if attr, ok := device.Attributes[consts.AttrDevicePath]; ok {
		if val := attr.StringValue; val != nil {
			opts.DevicePath = *val
		} else {
			return opts, fmt.Errorf("devicePath attribute is not exist")
		}
	}

	if attr, ok := device.Attributes[consts.AttrDeviceNumber]; ok {
		if val := attr.StringValue; val != nil {
			opts.DeviceNum = *val
		} else {
			return opts, fmt.Errorf("deviceNum attribute is not exist")
		}
	}

	if attr, ok := device.Attributes[consts.AttrBus]; ok {
		if val := attr.StringValue; val != nil {
			opts.Bus = *val
		} else {
			return opts, fmt.Errorf("bus attribute is not exist")
		}
	}

	if attr, ok := device.Attributes[consts.AttrMajor]; ok {
		if val := attr.IntValue; val != nil {
			opts.Major = *val
		} else {
			return opts, fmt.Errorf("major attribute is not exist")
		}
	}

	if attr, ok := device.Attributes[consts.AttrMinor]; ok {
		if val := attr.IntValue; val != nil {
			opts.Minor = *val
		} else {
			return opts, fmt.Errorf("minor attribute is not exist")
		}
	}

	return opts, nil
}

func (s *AllocationStore) isUSBGatewayRequest(result *resourcev1.DeviceRequestAllocationResult) bool {
	// virtualization-dra creates slices with pool name by node name
	// if pool not equal our node name, it is usb gateway request
	return result.Pool != s.nodeName
}

type containerEditsOptions struct {
	Name       string
	DevicePath string
	DeviceNum  string
	Bus        string
	Major      int64
	Minor      int64
	UID        *uint32
	GID        *uint32
}

func (s *AllocationStore) makeContainerEdits(claimUID string, opts containerEditsOptions) *cdiapi.ContainerEdits {
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
					UID:         opts.UID,
					GID:         opts.GID,
				},
			},
		},
	}

	return edits
}

func (s *AllocationStore) Unprepare(_ context.Context, claimUID types.UID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.cdi.DeleteClaimSpecFile(string(claimUID)); err != nil {
		return fmt.Errorf("unable to delete CDI spec file for claim: %w", err)
	}

	usbGatewayEnabled := featuregates.Default().USBGatewayEnabled()

	allocatedDevices := s.resourceClaimAllocations[claimUID]
	for _, device := range allocatedDevices {
		if usbGatewayEnabled {
			if count := s.usbipAllocatedDevicesCount[device]; count == 1 {
				if err := s.usbGateway.Detach(device); err != nil {
					return fmt.Errorf("failed to detach device %s: %w", device, err)
				}
				delete(s.usbipAllocatedDevicesCount, device)
			} else if count > 1 {
				s.usbipAllocatedDevicesCount[device]--
			}
		}

		s.allocatedDevices.Delete(device)
	}
	delete(s.resourceClaimAllocations, claimUID)

	return nil
}

func (s *AllocationStore) Synchronize(_ context.Context, pods []*api.PodSandbox, containers []*api.Container) ([]*api.ContainerUpdate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	containersByPodSandboxID := make(map[string][]*api.Container, len(pods))
	for _, ctr := range containers {
		containersByPodSandboxID[ctr.PodSandboxId] = append(containersByPodSandboxID[ctr.PodSandboxId], ctr)
	}

	var uspIPDeviceNames map[string]struct{}
	if featuregates.Default().USBGatewayEnabled() {
		names, err := s.usbGateway.GetAttachedDeviceNames()
		if err != nil {
			return nil, fmt.Errorf("failed to get attached device names: %w", err)
		}
		uspIPDeviceNames = names
	}

	for _, pod := range pods {
		s.log.Info("Synchronize pod", slog.String("name", pod.Name), slog.String("namespace", pod.Namespace))
		ctrs := containersByPodSandboxID[pod.Id]

		for _, ctr := range ctrs {
			claimUIDDeviceNames, err := parseDraEnvToClaimAllocations(ctr.Env)
			if err != nil {
				s.log.Error("failed to parse dra env", slog.String("name", pod.Name), slog.String("namespace", pod.Namespace), slog.Any("err", err))
				continue
			}
			for claimUID, deviceNames := range claimUIDDeviceNames {
				s.resourceClaimAllocations[claimUID] = append(s.resourceClaimAllocations[claimUID], deviceNames...)
				for _, deviceName := range deviceNames {
					s.allocatedDevices.Insert(deviceName)

					if _, ok := uspIPDeviceNames[deviceName]; ok {
						s.usbipAllocatedDevicesCount[deviceName]++
					}
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

		if strings.HasPrefix(key, "DRA_USB_CLAIM_UID_") && strings.HasSuffix(key, "_DEVICE_NAME") {
			uid := strings.TrimPrefix(key, "DRA_USB_CLAIM_UID_")
			uid = strings.TrimSuffix(uid, "_DEVICE_NAME")
			uid = strings.ToLower(uid)
			claimUID := types.UID(uid)

			deviceName := value

			result[claimUID] = append(result[claimUID], deviceName)
		}
	}

	return result, nil
}

func (s *AllocationStore) makeResources(devices []resourcev1.Device) resourceslice.DriverResources {
	poolName := s.nodeName

	pool := resourceslice.Pool{
		Slices: []resourceslice.Slice{
			{
				Devices: devices,
			},
		},
	}

	if featuregates.Default().USBGatewayEnabled() {
		pool.NodeSelector = getNodeSelector(s.nodeName)
	}

	return resourceslice.DriverResources{
		Pools: map[string]resourceslice.Pool{
			poolName: pool,
		},
	}
}
