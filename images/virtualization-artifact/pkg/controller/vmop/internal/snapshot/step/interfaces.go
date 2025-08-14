package step

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

//go:generate go tool moq -rm -out mock.go . Restorer
type Restorer interface {
	RestoreVirtualMachine(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachine, error)
	RestoreProvisioner(ctx context.Context, secret *corev1.Secret) (*corev1.Secret, error)
	RestoreVirtualMachineIPAddress(ctx context.Context, secret *corev1.Secret) (*virtv2.VirtualMachineIPAddress, error)
	RestoreVirtualMachineBlockDeviceAttachments(ctx context.Context, secret *corev1.Secret) ([]*virtv2.VirtualMachineBlockDeviceAttachment, error)
}
