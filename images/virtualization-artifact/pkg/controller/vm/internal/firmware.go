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

package internal

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vm/internal/state"
	"github.com/deckhouse/virtualization-controller/pkg/version"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmcondition"
)

const firmwareHandler = "FirmwareHandler"

func NewFirmwareHandler(firmwareImage string) *FirmwareHandler {
	return &FirmwareHandler{
		firmwareVersion:             version.GetFirmwareVersion(),
		firmwareMinSupportedVersion: version.GetFirmwareMinSupportedVersion(),
		firmwareImage:               firmwareImage,
	}
}

type FirmwareHandler struct {
	firmwareVersion             version.Version
	firmwareMinSupportedVersion version.Version
	firmwareImage               string
}

func (f FirmwareHandler) Handle(ctx context.Context, s state.VirtualMachineState) (reconcile.Result, error) {
	if s.VirtualMachine().IsEmpty() {
		return reconcile.Result{}, nil
	}
	changed := s.VirtualMachine().Changed()

	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return reconcile.Result{}, err
	}

	if kvvmi == nil || kvvmi.Status.LauncherContainerImageVersion == f.firmwareImage {
		// If kvvmi does not exist, update the firmware version,
		// as any newly created kvvmi will use the currently available firmware version.
		changed.Status.FirmwareVersion = f.firmwareVersion.String()
		f.removeCondition(changed)
		return reconcile.Result{}, nil
	}
	if f.needUpdate(changed.Status.FirmwareVersion, kvvmi.Status.LauncherContainerImageVersion) {
		f.addCondition(changed)
		return reconcile.Result{}, nil
	}

	f.removeCondition(changed)

	return reconcile.Result{}, nil
}

func (f FirmwareHandler) Name() string {
	return firmwareHandler
}

func (f FirmwareHandler) needUpdate(currentVersion, firmwareImage string) bool {
	if currentVersion == "" {
		return true
	}
	currVersion := version.Version(currentVersion)

	if !currVersion.IsValid() {
		return true
	}

	if f.firmwareVersion.Compare(currVersion) == 0 {
		// Need update if versions is main but has different virt-launcher images
		if f.firmwareVersion.IsMain() {
			return f.firmwareImage != firmwareImage
		}
		return false
	}

	// Need update if curr version less than min supported version
	if currVersion.Compare(f.firmwareMinSupportedVersion) == -1 {
		return true
	}

	// Need update if curr version bigger than firmware version
	return currVersion.Compare(f.firmwareVersion) == 1
}

func (f FirmwareHandler) addCondition(changed *virtv2.VirtualMachine) {
	conditions.SetCondition(conditions.NewConditionBuilder(vmcondition.TypeFirmwareNeedUpdate).
		Generation(changed.GetGeneration()).
		Status(metav1.ConditionTrue).
		Reason(vmcondition.TypeFirmwareNeedUpdate).
		Message("The VM firmware is outdated and not recommended for use by the current version of the module, please migrate or reboot the VM to upgrade to the new firmware version."),
		&changed.Status.Conditions,
	)
}

func (f FirmwareHandler) removeCondition(changed *virtv2.VirtualMachine) {
	conditions.RemoveCondition(vmcondition.TypeFirmwareNeedUpdate, &changed.Status.Conditions)
}
