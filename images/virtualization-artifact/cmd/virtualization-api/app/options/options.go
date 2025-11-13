/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package options

import (
	"fmt"
	"net"
	"strings"

	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/util/compatibility"
	"k8s.io/client-go/rest"
	"k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/deckhouse/virtualization-controller/pkg/apiserver/api"
	generatedopenapi "github.com/deckhouse/virtualization-controller/pkg/apiserver/api/generated/openapi"
	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/server"
	vconf "github.com/deckhouse/virtualization-controller/pkg/config"
)

type Options struct {
	// genericoptions.RecomendedOptions - EtcdOptions
	SecureServing  *genericoptions.SecureServingOptionsWithLoopback
	Authentication *genericoptions.DelegatingAuthenticationOptions
	Authorization  *genericoptions.DelegatingAuthorizationOptions
	Audit          *genericoptions.AuditOptions
	Features       *genericoptions.FeatureOptions
	Logging        *logs.Options

	Kubevirt vmrest.KubevirtAPIServerConfig

	ProxyClientCertFile string
	ProxyClientKeyFile  string

	ShowVersion bool
	// Only to be used to for testing
	DisableAuthForTesting bool
}

func (o *Options) Validate() error {
	return logsapi.ValidateAndApply(o.Logging, nil)
}

func (o *Options) Flags() (fs flag.NamedFlagSets) {
	msfs := fs.FlagSet("virtualization-api server")
	msfs.BoolVar(&o.ShowVersion, "version", false, "Show version")
	msfs.StringVar(&o.Kubevirt.Endpoint, "kubevirt-endpoint", "", "Kubevirt APIServer endpoint")
	msfs.StringVar(&o.Kubevirt.CaBundlePath, "kubevirt-cabundle", "", "Kubevirt CaBundle path")
	// proxy flags for autentification in virt-api.
	msfs.StringVar(&o.ProxyClientCertFile, "proxy-client-cert-file", "", "The client certificate used to verify the identity of the virtualization-api.")
	msfs.StringVar(&o.ProxyClientKeyFile, "proxy-client-key-file", "", "Private key for the client certificate used to prove the identity of the virtualization-api.")
	// flags for authorization sa
	msfs.StringVar(&o.Kubevirt.ServiceAccount.Name, "service-account-name", "", "The service-account name for authorization in kubevirt apiserver")
	msfs.StringVar(&o.Kubevirt.ServiceAccount.Namespace, "service-account-namespace", "", "The service-account namespace for authorization in kubevirt apiserver")

	o.SecureServing.AddFlags(fs.FlagSet("virtualization-api secure serving"))
	o.Authentication.AddFlags(fs.FlagSet("virtualization-api authentication"))
	o.Authorization.AddFlags(fs.FlagSet("virtualization-api authorization"))
	o.Audit.AddFlags(fs.FlagSet("virtualization-api audit log"))
	o.Features.AddFlags(fs.FlagSet("features"))
	logsapi.AddFlags(o.Logging, fs.FlagSet("logging"))

	return fs
}

func NewOptions() *Options {
	return &Options{
		SecureServing:  genericoptions.NewSecureServingOptions().WithLoopback(),
		Authentication: genericoptions.NewDelegatingAuthenticationOptions(),
		Authorization:  genericoptions.NewDelegatingAuthorizationOptions(),
		Features:       genericoptions.NewFeatureOptions(),
		Audit:          genericoptions.NewAuditOptions(),

		Logging: logs.NewOptions(),
	}
}

func (o *Options) ServerConfig() (*server.Config, error) {
	apiserver, err := o.ApiserverConfig()
	if err != nil {
		return nil, err
	}
	restConfig, err := o.RestConfig()
	if err != nil {
		return nil, err
	}

	conf := &server.Config{
		Apiserver:           apiserver,
		Rest:                restConfig,
		Kubevirt:            vconf.LoadKubevirtAPIServerFromEnv(),
		ProxyClientCertFile: o.ProxyClientCertFile,
		ProxyClientKeyFile:  o.ProxyClientKeyFile,
	}
	if o.Kubevirt.Endpoint != "" {
		conf.Kubevirt.Endpoint = o.Kubevirt.Endpoint
	}
	if o.Kubevirt.CaBundlePath != "" {
		conf.Kubevirt.CaBundlePath = o.Kubevirt.CaBundlePath
	}
	if o.Kubevirt.ServiceAccount.Name != "" {
		conf.Kubevirt.ServiceAccount.Name = o.Kubevirt.ServiceAccount.Name
	}
	if o.Kubevirt.ServiceAccount.Namespace != "" {
		conf.Kubevirt.ServiceAccount.Namespace = o.Kubevirt.ServiceAccount.Namespace
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return conf, nil
}

func (o *Options) ApiserverConfig() (*genericapiserver.Config, error) {
	if err := o.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %w", err)
	}

	serverConfig := genericapiserver.NewConfig(api.Codecs)
	if err := o.SecureServing.ApplyTo(&serverConfig.SecureServing, &serverConfig.LoopbackClientConfig); err != nil {
		return nil, err
	}

	if !o.DisableAuthForTesting {
		if err := o.Authentication.ApplyTo(&serverConfig.Authentication, serverConfig.SecureServing, nil); err != nil {
			return nil, err
		}
		if err := o.Authorization.ApplyTo(&serverConfig.Authorization); err != nil {
			return nil, err
		}
	}

	if err := o.Audit.ApplyTo(serverConfig); err != nil {
		return nil, err
	}

	versionGet := compatibility.DefaultBuildEffectiveVersion()
	serverConfig.EffectiveVersion = versionGet
	serverConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(generatedopenapi.GetOpenAPIDefinitions, openapinamer.NewDefinitionNamer(api.Scheme))
	serverConfig.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(generatedopenapi.GetOpenAPIDefinitions, openapinamer.NewDefinitionNamer(api.Scheme))
	serverConfig.OpenAPIConfig.Info.Title = "VirtualizationAPI"
	serverConfig.OpenAPIV3Config.Info.Title = "VirtualizationAPI"
	serverConfig.OpenAPIConfig.Info.Version = strings.Split(versionGet.String(), "-")[0]
	serverConfig.OpenAPIV3Config.Info.Version = strings.Split(versionGet.String(), "-")[0]

	return serverConfig, nil
}

func (o *Options) RestConfig() (*rest.Config, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	// Use protobufs for communication with apiserver
	// cfg.ContentType = "application/vnd.kubernetes.protobuf"
	err = rest.SetKubernetesDefaults(cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
