package importer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"k8s.io/klog/v2"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"kubevirt.io/containerized-data-importer/pkg/common"
	cc "kubevirt.io/containerized-data-importer/pkg/controller/common"
	"kubevirt.io/containerized-data-importer/pkg/importer"
	"kubevirt.io/containerized-data-importer/pkg/util"
	prometheusutil "kubevirt.io/containerized-data-importer/pkg/util/prometheus"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/auth"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/registry"
)

// FIXME(ilya-lesikov): certdir

func New() *Importer {
	return &Importer{}
}

type Importer struct {
	src            string
	srcType        string
	srcContentType string
	srcUsername    string
	srcPassword    string
	srcInsecure    bool
	destImageName  string
	destUsername   string
	destPassword   string
	destInsecure   bool
	certDir        string
	sha256Sum      string
	md5Sum         string
}

func (i *Importer) Run(ctx context.Context) error {
	promCertsDir, err := os.MkdirTemp("", "certsdir")
	if err != nil {
		return fmt.Errorf("error creating prometheus certs directory: %w", err)
	}
	defer os.RemoveAll(promCertsDir)
	prometheusutil.StartPrometheusEndpoint(promCertsDir)

	if err := i.parseOptions(); err != nil {
		return fmt.Errorf("error parsing options: %w", err)
	}

	if i.srcType == cc.SourceRegistry {
		return i.runForRegistry(ctx)
	}

	return i.runForDataSource(ctx)
}

func (i *Importer) parseOptions() error {
	i.src, _ = util.ParseEnvVar(common.ImporterEndpoint, false)
	i.srcType, _ = util.ParseEnvVar(common.ImporterSource, false)
	i.srcContentType, _ = util.ParseEnvVar(common.ImporterContentType, false)
	i.srcInsecure, _ = strconv.ParseBool(os.Getenv(common.InsecureTLSVar))
	i.destImageName, _ = util.ParseEnvVar(common.ImporterDestinationEndpoint, false)
	i.destInsecure, _ = strconv.ParseBool(os.Getenv(common.DestinationInsecureTLSVar))
	i.sha256Sum, _ = util.ParseEnvVar(common.ImporterSHA256Sum, false)
	i.md5Sum, _ = util.ParseEnvVar(common.ImporterMD5Sum, false)
	i.certDir, _ = util.ParseEnvVar(common.ImporterCertDirVar, false)

	i.srcUsername, _ = util.ParseEnvVar(common.ImporterAccessKeyID, false)
	i.srcPassword, _ = util.ParseEnvVar(common.ImporterSecretKey, false)
	if i.srcUsername == "" && i.srcPassword == "" && i.srcType == cc.SourceRegistry {
		srcAuthConfig, _ := util.ParseEnvVar(common.ImporterAuthConfig, false)
		if srcAuthConfig != "" {
			authFile, err := auth.RegistryAuthFile(srcAuthConfig)
			if err != nil {
				return fmt.Errorf("error parsing source auth config: %w", err)
			}

			i.srcUsername, i.srcPassword, err = auth.CredsFromRegistryAuthFile(authFile, i.src)
			if err != nil {
				return fmt.Errorf("error getting creds from source auth config: %w", err)
			}
		}
	}

	i.destUsername, _ = util.ParseEnvVar(common.ImporterDestinationAccessKeyID, false)
	i.destPassword, _ = util.ParseEnvVar(common.ImporterDestinationSecretKey, false)
	if i.destUsername == "" && i.destPassword == "" {
		destAuthConfig, _ := util.ParseEnvVar(common.ImporterDestinationAuthConfig, false)
		if destAuthConfig != "" {
			authFile, err := auth.RegistryAuthFile(destAuthConfig)
			if err != nil {
				return fmt.Errorf("error parsing destination auth config: %w", err)
			}

			i.destUsername, i.destPassword, err = auth.CredsFromRegistryAuthFile(authFile, i.destImageName)
			if err != nil {
				return fmt.Errorf("error getting creds from destination auth config: %w", err)
			}
		}
	}

	return nil
}

func (i *Importer) runForRegistry(ctx context.Context) error {
	srcNameOpts := i.srcNameOptions()
	srcRemoteOpts := i.srcRemoteOptions(ctx)
	destNameOpts := i.destNameOptions()
	destRemoteOpts := i.destRemoteOptions(ctx)

	srcRef, err := name.ParseReference(i.src, srcNameOpts...)
	if err != nil {
		return fmt.Errorf("error parsing source image name: %w", err)
	}

	srcDesc, err := remote.Get(srcRef, srcRemoteOpts...)
	if err != nil {
		return fmt.Errorf("error getting source image descriptor: %w", err)
	}

	srcImage, err := srcDesc.Image()
	if err != nil {
		return fmt.Errorf("error getting source image from descriptor: %w", err)
	}

	destRef, err := name.ParseReference(i.destImageName, destNameOpts...)
	if err != nil {
		return fmt.Errorf("error parsing destination image name: %w", err)
	}

	klog.Infof("Writing image %q to registry", i.destImageName)
	if err := remote.Write(destRef, srcImage, destRemoteOpts...); err != nil {
		return fmt.Errorf("error writing image to registry: %w", err)
	}

	klog.Infoln("Image upload completed")
	return nil
}

func (i *Importer) runForDataSource(ctx context.Context) error {
	ds, err := i.newDataSource(ctx)
	if err != nil {
		return fmt.Errorf("error creating data source: %w", err)
	}
	defer ds.Close()

	processor, err := registry.NewDataProcessor(ds, registry.DestinationRegistry{
		ImageName: i.destImageName,
		Username:  i.destUsername,
		Password:  i.destPassword,
		Insecure:  i.destInsecure,
	}, i.sha256Sum, i.md5Sum)
	if err != nil {
		return err
	}

	return processor.Process(context.Background())
}

func (i *Importer) newDataSource(_ context.Context) (importer.DataSourceInterface, error) {
	var result importer.DataSourceInterface

	switch i.srcType {
	case cc.SourceHTTP:
		var err error
		result, err = importer.NewHTTPDataSource(i.src, i.srcUsername, i.srcPassword, i.certDir, cdiv1.DataVolumeContentType(i.srcContentType))
		if err != nil {
			return nil, fmt.Errorf("error creating HTTP data source: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown source type: %s", i.srcType)
	}

	return result, nil
}

func (i *Importer) srcNameOptions() []name.Option {
	nameOpts := []name.Option{}

	if i.srcInsecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	return nameOpts
}

func (i *Importer) destNameOptions() []name.Option {
	nameOpts := []name.Option{}

	if i.destInsecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	return nameOpts
}

func (i *Importer) srcRemoteOptions(ctx context.Context) []remote.Option {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: i.srcInsecure,
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig

	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(transport),
		remote.WithAuth(&authn.Basic{Username: i.srcUsername, Password: i.srcPassword}),
	}

	return remoteOpts
}

func (i *Importer) destRemoteOptions(ctx context.Context) []remote.Option {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: i.destInsecure,
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig

	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(transport),
		remote.WithAuth(&authn.Basic{Username: i.destUsername, Password: i.destPassword}),
	}

	return remoteOpts
}
