package datasource

import virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"

func HasCABundle(ds *virtv2alpha1.DataSource) bool {
	return len(GetCABundle(ds)) > 0
}

func GetCABundle(ds *virtv2alpha1.DataSource) string {
	if ds == nil {
		return ""
	}
	switch ds.Type {
	case virtv2alpha1.DataSourceTypeHTTP:
		if http := ds.HTTP; http != nil {
			return string(http.CABundle)
		}
	case virtv2alpha1.DataSourceTypeContainerImage:
		if img := ds.ContainerImage; img != nil {
			return string(img.CABundle)
		}
	}
	return ""
}
