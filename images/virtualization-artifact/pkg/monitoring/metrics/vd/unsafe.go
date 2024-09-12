package vd

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

// Iter implements iteration on objects VirtualDisk and create new DTO.
// DO NOT mutate VirtualDisk!
func (l *iterator) Iter(ctx context.Context, h handler) error {
	vds := virtv2.VirtualDiskList{}
	if err := l.reader.List(ctx, &vds, client.UnsafeDisableDeepCopy); err != nil {
		return err
	}
	for _, vd := range vds.Items {
		m := newDataMetric(&vd)
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
