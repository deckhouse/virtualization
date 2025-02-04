package hookinfolder

import (
	"fmt"

	tlscertificate "github.com/deckhouse/module-sdk/common-hooks/tls-certificate"
)

const (
	MODULE_NAME      string = "virtualization"
	MODULE_NAMESPACE string = "d8-virtualization"

	CONTOLLER_CERT_CN string = "virtualization-controller"
	DVCR_CERT_CN      string = "dvcr"
	API_CERT_CN       string = "virtualization-api"
	API_PROXY_CERT_CN string = "virtualization-api-proxy"
)

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:            CONTOLLER_CERT_CN,
	TLSSecretName: "virtualization-controller-tls",
	Namespace:     MODULE_NAMESPACE,
	SANs: tlscertificate.DefaultSANs([]string{
		// virtualization
		CONTOLLER_CERT_CN,
		// virtualization.d8-virtualization
		fmt.Sprintf("%s.%s", CONTOLLER_CERT_CN, MODULE_NAMESPACE),
		// virtualization.d8-virtualization.svc
		fmt.Sprintf("%s.%s.svc", CONTOLLER_CERT_CN, MODULE_NAMESPACE),
		// %CLUSTER_DOMAIN%:// is a special value to generate SAN like 'virtualization.d8-virtualization.svc.cluster.local'
		fmt.Sprintf("%%CLUSTER_DOMAIN%%://%s.%s.svc", CONTOLLER_CERT_CN, MODULE_NAMESPACE),
	}),

	FullValuesPathPrefix: fmt.Sprintf("%s.internal.controller.cert", MODULE_NAME),
})

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:            DVCR_CERT_CN,
	TLSSecretName: "dvcr-tls",
	Namespace:     MODULE_NAMESPACE,
	SANs: tlscertificate.DefaultSANs([]string{
		// dvcr
		DVCR_CERT_CN,
		// dvcr.d8-virtualization
		fmt.Sprintf("%s.%s", DVCR_CERT_CN, MODULE_NAMESPACE),
		// dvcr.d8-virtualization.svc
		fmt.Sprintf("%s.%s.svc", DVCR_CERT_CN, MODULE_NAMESPACE),
		// %CLUSTER_DOMAIN%:// is a special value to generate SAN like 'dvcr.d8-virtualization.svc.cluster.local'
		fmt.Sprintf("%%CLUSTER_DOMAIN%%://%s.%s.svc", DVCR_CERT_CN, MODULE_NAMESPACE),
	}),

	FullValuesPathPrefix: fmt.Sprintf("%s.internal.controller.cert", MODULE_NAME),
})

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:            API_CERT_CN,
	TLSSecretName: "virtualization-api-tls",
	Namespace:     MODULE_NAMESPACE,
	SANs: tlscertificate.DefaultSANs([]string{
		// virtualization-api
		DVCR_CERT_CN,
		// virtualization-api.d8-virtualization
		fmt.Sprintf("%s.%s", DVCR_CERT_CN, MODULE_NAMESPACE),
		// virtualization-api.d8-virtualization.svc
		fmt.Sprintf("%s.%s.svc", DVCR_CERT_CN, MODULE_NAMESPACE),
		// %CLUSTER_DOMAIN%:// is a special value to generate SAN like 'virtualization-api.d8-virtualization.svc.cluster.local'
		fmt.Sprintf("%%CLUSTER_DOMAIN%%://%s.%s.svc", DVCR_CERT_CN, MODULE_NAMESPACE),
	}),

	FullValuesPathPrefix: fmt.Sprintf("%s.internal.controller.cert", MODULE_NAME),
})

var _ = tlscertificate.RegisterInternalTLSHookEM(tlscertificate.GenSelfSignedTLSHookConf{
	CN:            API_PROXY_CERT_CN,
	TLSSecretName: "virtualization-api-proxy-tls",
	Namespace:     MODULE_NAMESPACE,
	SANs: tlscertificate.DefaultSANs([]string{
		// virtualization-api-proxy
		DVCR_CERT_CN,
		// virtualization-api-proxy.d8-virtualization
		fmt.Sprintf("%s.%s", DVCR_CERT_CN, MODULE_NAMESPACE),
		// virtualization-api-proxy.d8-virtualization.svc
		fmt.Sprintf("%s.%s.svc", DVCR_CERT_CN, MODULE_NAMESPACE),
		// %CLUSTER_DOMAIN%:// is a special value to generate SAN like 'virtualization-api-proxy.d8-virtualization.svc.cluster.local'
		fmt.Sprintf("%%CLUSTER_DOMAIN%%://%s.%s.svc", DVCR_CERT_CN, MODULE_NAMESPACE),
	}),

	FullValuesPathPrefix: fmt.Sprintf("%s.internal.controller.cert", MODULE_NAME),
})
