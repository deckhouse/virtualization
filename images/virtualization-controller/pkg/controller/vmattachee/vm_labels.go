package vmattachee

import (
	"fmt"
	"strings"

	virtv2 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
)

const (
	AttachedResourceLabelKeyFormat = virtv2.APIGroup + "/%s.%s.attached"
	AttachedLabelValue             = "true"
)

func MakeAttachedResourceLabelKeyFormat(kind, name string) string {
	kind = strings.ToLower(kind)
	return fmt.Sprintf(AttachedResourceLabelKeyFormat, kind, name)
}

func ExtractAttachedResourceName(kind, labelKey string) (string, bool) {
	kind = strings.ToLower(kind)
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
