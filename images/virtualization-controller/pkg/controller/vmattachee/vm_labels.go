package vmattachee

import (
	"fmt"
	"strings"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

const (
	AttachedResourceLabelKeyFormat   = virtv2.APIGroup + "/%s.%s.attached"
	AttachedLabelValue               = "true"
	HotpluggedResourceLabelKeyFormat = virtv2.APIGroup + "/%s.%s.hotplugged"
	HotpluggedLabelValue             = "true"
)

func MakeAttachedResourceLabelKeyFormat(kind, name string) string {
	kind = strings.ToLower(kind)
	return fmt.Sprintf(AttachedResourceLabelKeyFormat, kind, name)
}

func MakeHotpluggedResourceLabelKeyFormat(kind, name string) string {
	kind = strings.ToLower(kind)
	return fmt.Sprintf(HotpluggedResourceLabelKeyFormat, kind, name)
}

// ExtractAttachedResourceName extracts attched CVMI/VMI/VMD name from the label name.
// kind input is one of "cvmi", "vmi" or "vmd".
func ExtractAttachedResourceName(kind, labelKey string) (string, bool) {
	return extractAttachedResourceName(kind, labelKey, ".attached")
}

// ExtractHotpluggedResourceName extracts hotplugged CVMI/VMI/VMD name from the label name.
// kind input is one of "cvmi", "vmi" or "vmd".
func ExtractHotpluggedResourceName(kind, labelKey string) (string, bool) {
	return extractAttachedResourceName(kind, labelKey, ".hotplugged")
}

func extractAttachedResourceName(kind, labelKey, suffix string) (string, bool) {
	kind = strings.ToLower(kind)
	parts := strings.SplitN(labelKey, "/", 2)
	if len(parts) != 2 {
		return "", false
	}

	if strings.HasPrefix(parts[1], kind+".") && strings.HasSuffix(parts[1], suffix) {
		res := parts[1]
		res = strings.TrimPrefix(res, kind+".")
		res = strings.TrimSuffix(res, suffix)
		return res, true
	}
	return "", false
}
