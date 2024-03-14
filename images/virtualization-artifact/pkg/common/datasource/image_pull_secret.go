package datasource

import virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"

func ShouldCopyImagePullSecret(ctrImg *virtv2alpha1.DataSourceContainerRegistry, targetNS string) bool {
	if ctrImg == nil || ctrImg.ImagePullSecret.Name == "" {
		return false
	}

	imgPullNS := ctrImg.ImagePullSecret.Namespace

	// Should copy imagePullSecret if namespace differs from the specified namespace.
	return imgPullNS != "" && imgPullNS != targetNS
}
