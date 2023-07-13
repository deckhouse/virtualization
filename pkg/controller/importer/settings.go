package importer

import (
	virtv2alpha1 "github.com/deckhouse/virtualization-controller/api/v2alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
)

// Settings stores all possible settings for registry-importer binary.
// Fields from this struct are passed via environment variables.
type Settings struct {
	Verbose                string
	Endpoint               string
	SecretName             string
	Source                 string
	ContentType            string
	ImageSize              string
	CertConfigMap          string
	DiskID                 string
	UUID                   string
	ReadyFile              string
	DoneFile               string
	BackingFile            string
	Thumbprint             string
	FilesystemOverhead     string
	InsecureTLS            bool
	HTTPProxy              string
	HTTPSProxy             string
	NoProxy                string
	CertConfigMapProxy     string
	ExtraHeaders           []string
	SecretExtraHeaders     []string
	DestinationEndpoint    string
	DestinationInsecureTLS string
	DestinationAuthSecret  string
}

func UpdateDVCRSettings(podEnvVars *Settings, dvcrSettings *common.DVCRSettings, endpoint string) {
	podEnvVars.DestinationAuthSecret = dvcrSettings.AuthSecret
	podEnvVars.DestinationInsecureTLS = dvcrSettings.InsecureTLS
	podEnvVars.DestinationEndpoint = endpoint
}

func UpdateHTTPSettings(podEnvVars *Settings, http *virtv2alpha1.DataSourceHTTP) {
	podEnvVars.Endpoint = http.URL

	// if http.SecretRef != "" {
	//	annotations[AnnSecret] = http.SecretRef
	// }
	// if http.CertConfigMap != "" {
	//	annotations[AnnCertConfigMap] = http.CertConfigMap
	// }
	// for index, header := range http.ExtraHeaders {
	//	annotations[fmt.Sprintf("%s.%d", AnnExtraHeaders, index)] = header
	// }
	// for index, header := range http.SecretExtraHeaders {
	//	annotations[fmt.Sprintf("%s.%d", AnnSecretExtraHeaders, index)] = header
	// }
}
