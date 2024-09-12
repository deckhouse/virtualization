package vd

import (
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type dataMetric struct {
	Name      string
	Namespace string
	UID       string
	Phase     virtv2.DiskPhase
}

// DO NOT mutate VirtualDisk!
func newDataMetric(vd *virtv2.VirtualDisk) *dataMetric {
	if vd == nil {
		return nil
	}

	return &dataMetric{
		Name:      vd.Name,
		Namespace: vd.Namespace,
		UID:       string(vd.UID),
		Phase:     vd.Status.Phase,
	}
}
