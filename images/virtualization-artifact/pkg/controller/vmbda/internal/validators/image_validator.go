package validators

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ImageValidator struct {
	service *service.AttachmentService
}

func NewImageValidator(service *service.AttachmentService) *ImageValidator {
	return &ImageValidator{
		service: service,
	}
}

func (v *ImageValidator) ValidateCreate(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	return v.validateImage(ctx, vmbda)
}

func (v *ImageValidator) ValidateUpdate(_ context.Context, _, _ *virtv2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	return nil, nil
}

func (v *ImageValidator) validateImage(ctx context.Context, vmbda *virtv2.VirtualMachineBlockDeviceAttachment) (admission.Warnings, error) {
	vi, err := v.service.GetVirtualImage(ctx, vmbda.GetName(), vmbda.GetNamespace())
	if err != nil {
		return nil, fmt.Errorf("failed to get virtual image: %w", err)
	}
	if vi == nil {
		return admission.Warnings{"VirtualImage not found"}, nil
	}
	if vi.Spec.Storage != virtv2.StoragePersistentVolumeClaim {
		return nil, fmt.Errorf("the virtual image type is not supported. The acceptable type is only %q", virtv2.StoragePersistentVolumeClaim)
	}
	return nil, nil
}
