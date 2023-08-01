package controller

import (
	"fmt"
	"strings"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

const (
	AttachedCVMILabelKeyFormat = virtv2.APIGroup + "/cvmi.%s.attached"
	AttachedVMILabelKeyFormat  = virtv2.APIGroup + "/vmi.%s.attached"
	AttachedVMDLabelKeyFormat  = virtv2.APIGroup + "/vmd.%s.attached"
)

func MakeAttachedCVMILabelKey(cvmiName string) string {
	return fmt.Sprintf(AttachedCVMILabelKeyFormat, cvmiName)
}

func ExtractAttachedCVMIName(labelKey string) (string, bool) {
	return extractAttachedObjectName(labelKey, "cvmi")
}

func MakeAttachedVMILabelKey(vmiName string) string {
	return fmt.Sprintf(AttachedVMILabelKeyFormat, vmiName)
}

func ExtractAttachedVMIName(labelKey string) (string, bool) {
	return extractAttachedObjectName(labelKey, "vmi")
}

func MakeAttachedVMDLabelKey(vmdName string) string {
	return fmt.Sprintf(AttachedVMDLabelKeyFormat, vmdName)
}

func ExtractAttachedVMDName(labelKey string) (string, bool) {
	return extractAttachedObjectName(labelKey, "vmd")
}

func extractAttachedObjectName(labelKey, kind string) (string, bool) {
	parts := strings.SplitN(labelKey, "/", 2)
	if len(parts) != 2 {
		return "", false
	}

	if strings.HasPrefix(parts[1], kind+".") && strings.HasSuffix(parts[1], ".attached") {
		res := parts[1]
		res = strings.TrimPrefix(res, kind+".")
		res = strings.TrimSuffix(res, ".attached")
		return res, true
	}
	return "", false
}
