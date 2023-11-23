package importer

type CABundleSettings struct {
	CABundle      string
	ConfigMapName string
}

func NewCABundleSettings(caBundle, caBundleConfigMapName string) *CABundleSettings {
	if caBundle == "" {
		return nil
	}
	return &CABundleSettings{
		CABundle:      caBundle,
		ConfigMapName: caBundleConfigMapName,
	}
}
