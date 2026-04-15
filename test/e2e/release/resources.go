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

package release

import (
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (t *currentReleaseSmokeTest) diskObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.vms)+len(t.dataDisks))
	for _, vmScenario := range t.vms {
		objects = append(objects, vmScenario.rootDisk)
	}
	for _, diskScenario := range t.dataDisks {
		objects = append(objects, diskScenario.disk)
	}
	return objects
}

func (t *currentReleaseSmokeTest) vmObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.vms))
	for _, vmScenario := range t.vms {
		objects = append(objects, vmScenario.vm)
	}
	return objects
}

func (t *currentReleaseSmokeTest) attachmentObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.attachments))
	for _, attachmentScenario := range t.attachments {
		objects = append(objects, attachmentScenario.attachment)
	}
	return objects
}

func (t *currentReleaseSmokeTest) initialRunningVMObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.vms))
	for _, vmScenario := range t.vms {
		if vmScenario.expectedInitialPhase() == string(v1alpha2.MachineRunning) {
			objects = append(objects, vmScenario.vm)
		}
	}
	return objects
}

func (t *currentReleaseSmokeTest) initialStoppedVMObjects() []crclient.Object {
	objects := make([]crclient.Object, 0, len(t.vms))
	for _, vmScenario := range t.vms {
		if vmScenario.expectedInitialPhase() == string(v1alpha2.MachineStopped) {
			objects = append(objects, vmScenario.vm)
		}
	}
	return objects
}

func (t *currentReleaseSmokeTest) manualStartVMs() []*vmScenario {
	manualVMs := make([]*vmScenario, 0)
	for _, vmScenario := range t.vms {
		if vmScenario.runPolicy == v1alpha2.ManualPolicy {
			manualVMs = append(manualVMs, vmScenario)
		}
	}
	return manualVMs
}

func (t *currentReleaseSmokeTest) manualStartVMObjects() []crclient.Object {
	objects := make([]crclient.Object, 0)
	for _, vmScenario := range t.manualStartVMs() {
		objects = append(objects, vmScenario.vm)
	}
	return objects
}
