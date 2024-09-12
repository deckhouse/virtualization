package vmop

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newUnsafeIterator(reader client.Reader) *iterator {
	return &iterator{
		reader: reader,
	}
}

type iterator struct {
	reader client.Reader
}

// Iter implements iteration on objects VirtualMachineOperation and create new DTO.
// DO NOT mutate VirtualMachineOperation!
func (l *iterator) Iter(ctx context.Context, h handler) error {
	vmops := virtv2.VirtualMachineOperationList{}
	if err := l.reader.List(ctx, &vmops, client.UnsafeDisableDeepCopy); err != nil {
		return err
	}
	for _, vmop := range vmops.Items {
		m := newDataMetric(&vmop)
		if stop := h(m); stop {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			continue
		}
	}
	return nil
}
