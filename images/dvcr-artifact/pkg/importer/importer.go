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

package importer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"kubevirt.io/containerized-data-importer/pkg/common"
	cc "kubevirt.io/containerized-data-importer/pkg/controller/common"
	"kubevirt.io/containerized-data-importer/pkg/importer"
	"kubevirt.io/containerized-data-importer/pkg/util"
	prometheusutil "kubevirt.io/containerized-data-importer/pkg/util/prometheus"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/auth"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/datasource"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/monitoring"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/registry"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/retry"
)

// FIXME(ilya-lesikov): certdir

const (
	DockerRegistrySchemePrefix = "docker://"
	DVCRSource                 = "dvcr"
	BlockDeviceSource          = "blockDevice"
	FilesystemSource          = "filesystem"
)

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

	if err = i.parseOptions(); err != nil {
		return fmt.Errorf("error parsing options: %w", err)
	}
	if i.srcType == DVCRSource {
		return i.runForDVCRSource(ctx)
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
			img := strings.TrimPrefix(i.src, DockerRegistrySchemePrefix)
			i.srcUsername, i.srcPassword, err = auth.CredsFromRegistryAuthFile(authFile, img)
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

func (i *Importer) runForDVCRSource(ctx context.Context) error {
	durCollector := monitoring.NewDurationCollector()

	craneOpts := i.destCraneOptions(ctx)
	err := crane.Copy(i.src, i.destImageName, craneOpts...)
	if err != nil {
		return fmt.Errorf("error copy repository: %w", err)
	}

	return monitoring.WriteDVCRSourceImportCompleteMessage(durCollector.Collect())
}

func (i *Importer) runForDataSource(ctx context.Context) error {
	durCollector := monitoring.NewDurationCollector()

	var res registry.ImportRes

	err := retry.Retry(ctx, func(ctx context.Context) error {
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

		res, err = processor.Process(ctx)
		return err
	})

	if err != nil {
		return monitoring.WriteImportFailureMessage(err)
	}

	return monitoring.WriteImportCompleteMessage(res.SourceImageSize, res.VirtualSize, res.AvgSpeed, res.Format, durCollector.Collect())
}

func (i *Importer) newDataSource(_ context.Context) (datasource.DataSourceInterface, error) {
	var result datasource.DataSourceInterface
	switch i.srcType {
	case cc.SourceHTTP:
		var err error
		result, err = importer.NewHTTPDataSource(i.src, i.srcUsername, i.srcPassword, i.certDir, cdiv1.DataVolumeContentType(i.srcContentType))
		if err != nil {
			return nil, fmt.Errorf("error creating HTTP data source: %w", err)
		}
	case cc.SourceRegistry:
		var err error
		result, err = datasource.NewContainerRegistryDataSource(i.src, i.srcUsername, i.srcPassword, i.certDir, i.srcInsecure)
		if err != nil {
			return nil, fmt.Errorf("error creating container registry data source: %w", err)
		}
	case BlockDeviceSource:
		var err error
		result, err = datasource.NewBlockDeviceDataSource()
		if err != nil {
			return nil, fmt.Errorf("error creating block device data source: %w", err)
		}
	case FilesystemSource:
		var err error
		result, err = datasource.NewFilesystemDataSource()
		if err != nil {
			return nil, fmt.Errorf("error creating filesystem data source: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown source type: %s", i.srcType)
	}

	return result, nil
}

func (i *Importer) destNameOptions() []name.Option {
	var nameOpts []name.Option

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

func (i *Importer) destCraneOptions(ctx context.Context) []crane.Option {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: i.destInsecure,
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig

	craneOpts := []crane.Option{
		crane.WithContext(ctx),
		crane.WithTransport(transport),
		crane.WithAuth(&authn.Basic{Username: i.destUsername, Password: i.destPassword}),
	}

	return craneOpts
}
