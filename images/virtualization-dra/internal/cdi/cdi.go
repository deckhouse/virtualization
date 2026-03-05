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

package cdi

import (
	"fmt"

	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdiparser "tags.cncf.io/container-device-interface/pkg/parser"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

const SpecDir = cdiapi.DefaultDynamicDir

const (
	cdiVendor           = "dra.virtualization.deckhouse.io"
	cdiCommonDeviceName = "common"
)

// Manager writes and removes CDI spec files so the container runtime can inject device nodes and env into pods.
// DRA returns CDI device names to the kubelet; runtime resolves them using these specs.
type Manager interface {
	CreateCommonSpecFile() error
	CreateClaimSpecFile(claimUID string, devices PreparedDevices) error
	DeleteClaimSpecFile(claimUID string) error
	GetClaimDevices(claimUID string, devices ...string) []string
}

type manager struct {
	cache        *cdiapi.Cache
	cdiClass     string
	cdiKind      string
	driverName   string
	nodeName     string
	cdiEnvPrefix string
}

func NewManager(cdiSpecDir, cdiClass, driverName, nodeName, cdiEnvPrefix string) (Manager, error) {
	if cdiSpecDir == "" {
		cdiSpecDir = SpecDir
	}

	cache, err := cdiapi.NewCache(cdiapi.WithSpecDirs(cdiSpecDir))
	if err != nil {
		return nil, err
	}

	return &manager{
		cache:        cache,
		cdiClass:     cdiClass,
		cdiKind:      fmt.Sprintf("%s/%s", cdiVendor, cdiClass),
		driverName:   driverName,
		nodeName:     nodeName,
		cdiEnvPrefix: cdiEnvPrefix,
	}, nil
}

func (cdi *manager) CreateCommonSpecFile() error {
	spec := &cdispec.Spec{
		Kind: cdi.cdiKind,
		Devices: []cdispec.Device{
			{
				Name: cdiCommonDeviceName,
				ContainerEdits: cdispec.ContainerEdits{
					Env: []string{
						fmt.Sprintf("KUBERNETES_NODE_NAME=%s", cdi.nodeName),
						fmt.Sprintf("DRA_RESOURCE_DRIVER_NAME=%s", cdi.driverName),
					},
				},
			},
		},
	}

	minVersion, err := cdispec.MinimumRequiredVersion(spec)
	if err != nil {
		return fmt.Errorf("failed to get minimum required CDI spec version: %w", err)
	}
	spec.Version = minVersion

	specName, err := cdiapi.GenerateNameForTransientSpec(spec, cdiCommonDeviceName)
	if err != nil {
		return fmt.Errorf("failed to generate Spec name: %w", err)
	}

	return cdi.cache.WriteSpec(spec, specName)
}

func (cdi *manager) CreateClaimSpecFile(claimUID string, devices PreparedDevices) error {
	specName := cdiapi.GenerateTransientSpecName(cdiVendor, cdi.cdiClass, claimUID)

	spec := &cdispec.Spec{
		Kind:    cdi.cdiKind,
		Devices: []cdispec.Device{},
	}

	for _, device := range devices {
		claimEdits := cdiapi.ContainerEdits{
			ContainerEdits: &cdispec.ContainerEdits{
				Env: []string{
					fmt.Sprintf("%s_%s_RESOURCE_CLAIM=%s", cdi.cdiEnvPrefix, device.DeviceName[4:], claimUID),
				},
			},
		}
		claimEdits.Append(device.ContainerEdits)

		cdiDevice := cdispec.Device{
			Name:           fmt.Sprintf("%s-%s", claimUID, device.DeviceName),
			ContainerEdits: *claimEdits.ContainerEdits,
		}

		spec.Devices = append(spec.Devices, cdiDevice)
	}

	minVersion, err := cdispec.MinimumRequiredVersion(spec)
	if err != nil {
		return fmt.Errorf("failed to get minimum required CDI spec version: %w", err)
	}
	spec.Version = minVersion

	return cdi.cache.WriteSpec(spec, specName)
}

func (cdi *manager) DeleteClaimSpecFile(claimUID string) error {
	specName := cdiapi.GenerateTransientSpecName(cdiVendor, cdi.cdiClass, claimUID)
	return cdi.cache.RemoveSpec(specName)
}

func (cdi *manager) GetClaimDevices(claimUID string, devices ...string) []string {
	cdiDevices := []string{
		cdiparser.QualifiedName(cdiVendor, cdi.cdiClass, cdiCommonDeviceName),
	}

	for _, device := range devices {
		cdiDevice := cdiparser.QualifiedName(cdiVendor, cdi.cdiClass, fmt.Sprintf("%s-%s", claimUID, device))
		cdiDevices = append(cdiDevices, cdiDevice)
	}

	return cdiDevices
}

// PreparedDevices carries DRA device info and CDI edits for one claim; used when creating the claim's CDI spec.
type PreparedDevices []*PreparedDevice

// PreparedDevice is one device plus the container edits (env, device nodes) that CDI will apply.
type PreparedDevice struct {
	drapbv1.Device
	ContainerEdits *cdiapi.ContainerEdits
}

func (pds PreparedDevices) GetDevices() []*drapbv1.Device {
	var devices []*drapbv1.Device
	for _, pd := range pds {
		devices = append(devices, &pd.Device)
	}
	return devices
}
