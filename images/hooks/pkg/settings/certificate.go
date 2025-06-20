package settings

// Service names for various components to use them as
// CNs in certificates.
const (
	ControllerCertCN string = "virtualization-controller"
	DVCRCertCN       string = "dvcr"
	APICertCN        string = "virtualization-api"
	APIProxyCertCN   string = "virtualization-api-proxy"
	AuditCertCN      string = "virtualization-audit"
)
