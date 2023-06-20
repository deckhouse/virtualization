package v2alpha1

// TODO: more fields from the CRD
type DataSource struct {
	HTTP *DataSourceHTTP `json:"http,omitempty"`
}

type DataSourceHTTP struct {
	URL string `json:"url"`
}
