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

package validators

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/deckhouse/pkg/log"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vmbda/internal/service"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type AttachmentConflictValidator struct {
	log     *log.Logger
	service *intsvc.AttachmentService
}

func NewAttachmentConflictValidator(service *intsvc.AttachmentService, log *log.Logger) *AttachmentConflictValidator {
	return &AttachmentConflictValidator{
		log:     log,
		service: service,
	}
}

func (v *AttachmentConflictValidator) ValidateCreate(ctx context.Context, vmbda *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	isConflicted, conflictWithName, err := v.service.IsConflictedAttachment(ctx, vmbda)
	if err != nil {
		v.log.Error("Failed to validate a VirtualMachineBlockDeviceAttachment creation", "err", err)
		return nil, nil
	}

	if isConflicted {
		return nil, fmt.Errorf(
			"another VirtualMachineBlockDeviceAttachment %s/%s already exists "+
				"with the same block device %s for hot-plugging",
			vmbda.Namespace, conflictWithName, vmbda.Spec.BlockDeviceRef.Name,
		)
	}

	return nil, nil
}

func (v *AttachmentConflictValidator) ValidateUpdate(_ context.Context, _, _ *v1alpha2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	return nil, nil
}
