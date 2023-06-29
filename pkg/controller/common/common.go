package common

import (
	"k8s.io/apimachinery/pkg/api/errors"

	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

// TODO make it work properly.
func IsCVMIComplete(cvmi *virtv2alpha1.ClusterVirtualMachineImage) bool {
	_, ok := cvmi.Annotations[AnnImportDone]
	return ok
}

// IgnoreNotFound returns nil if the error is a NotFound error.
// We generally want to ignore (not requeue) NotFound errors, since we'll get a reconciliation request once the
// object exists, and requeuing in the meantime won't help.
func IgnoreNotFound(err error) error {
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}
