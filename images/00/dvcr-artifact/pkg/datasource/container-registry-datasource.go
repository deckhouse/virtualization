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

package datasource

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/containers/image/v5/image"
	"github.com/containers/image/v5/pkg/blobinfocache"
	"github.com/containers/image/v5/types"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	klog "k8s.io/klog/v2"
	"kubevirt.io/containerized-data-importer/pkg/importer"
)

func NewContainerRegistryDataSource(endpoint, accessKey, secKey, certDir string, insecureTLS bool) (*ContainerRegistryDataSource, error) {
	// Certs dir always mount from secret.
	rrc, err := NewRegistryReadCloser(endpoint, accessKey, secKey, certDir, insecureTLS)
	if err != nil {
		return nil, err
	}
	return &ContainerRegistryDataSource{
		registryReadCloser: rrc,
	}, nil
}

type ContainerRegistryDataSource struct {
	registryReadCloser *registryReadCloser
}

func (crd *ContainerRegistryDataSource) ReadCloser() (io.ReadCloser, error) {
	return crd.registryReadCloser, nil
}

func (crd *ContainerRegistryDataSource) Length() (int, error) {
	return int(crd.registryReadCloser.tarHeader.Size), nil
}

func (crd *ContainerRegistryDataSource) Filename() (string, error) {
	path := strings.Split(crd.registryReadCloser.tarHeader.Name, "/")
	return path[len(path)-1], nil
}

func (r ContainerRegistryDataSource) Close() error {
	return r.registryReadCloser.Close()
}

type registryReadCloser struct {
	context   *context.Context
	cancel    context.CancelFunc
	tarHeader *tar.Header
	tarReader *tar.Reader
	closers   []io.Closer
}

func (r registryReadCloser) Read(p []byte) (n int, err error) {
	return r.tarReader.Read(p)
}

func (r registryReadCloser) Close() error {
	var errs error
	for _, c := range r.closers {
		if err := c.Close(); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	r.cancel()
	return errs
}

func NewRegistryReadCloser(endpoint, accessKey, secKey, certDir string, insecureTLS bool) (*registryReadCloser, error) {
	ctx, cancel := context.WithCancel(context.Background())
	srcCtx := importer.BuildSourceContext(accessKey, secKey, certDir, insecureTLS)
	src, err := importer.ReadImageSource(ctx, srcCtx, endpoint)
	if err != nil {
		return nil, err
	}
	imgCloser, err := image.FromSource(ctx, srcCtx, src)
	if err != nil {
		klog.Errorf("Error retrieving image: %v", err)
		return nil, errors.Wrap(err, "Error retrieving image")
	}
	cache := blobinfocache.DefaultCache(srcCtx)
	layers := imgCloser.LayerInfos()

	for _, layer := range layers {
		klog.Infof("Processing layer %+v", layer)
		hdr, tarReader, formatReaders, _ := parseLayer(ctx, srcCtx, src, layer, containerDiskImageDir, cache)
		if hdr != nil && tarReader != nil && formatReaders != nil {
			return &registryReadCloser{
				context:   &ctx,
				cancel:    cancel,
				tarHeader: hdr,
				tarReader: tarReader,
				closers: []io.Closer{
					formatReaders,
					imgCloser,
				},
			}, nil
		}
	}
	errs := fmt.Errorf("no files found in directory %s", containerDiskImageDir)
	if err := imgCloser.Close(); err != nil {
		multierror.Append(errs, err)
	}
	cancel()
	return nil, errs
}

func parseLayer(ctx context.Context,
	sys *types.SystemContext,
	src types.ImageSource,
	layer types.BlobInfo,
	pathPrefix string,
	cache types.BlobInfoCache) (*tar.Header, *tar.Reader, *importer.FormatReaders, error) {

	var reader io.ReadCloser
	reader, _, err := src.GetBlob(ctx, layer, cache)
	if err != nil {
		klog.Errorf("Could not read layer: %v", err)
		return nil, nil, nil, errors.Wrap(err, "Could not read layer")
	}
	fr, err := importer.NewFormatReaders(reader, 0)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "Could not read layer")
	}
	tarReader := tar.NewReader(fr.TopReader())
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			klog.Infof("No disk file found in layer %s", layer.Digest)
			break // End of archive
		}
		if err != nil {
			klog.Errorf("Error reading layer: %v", err)
			return nil, nil, nil, errors.Wrap(err, "Error reading layer")
		}
		if importer.HasPrefix(hdr.Name, pathPrefix) && !importer.IsWhiteout(hdr.Name) && !importer.IsDir(hdr) {
			klog.Infof("Disk file '%v' found in the layer", hdr.Name)
			return hdr, tarReader, fr, nil
		} else {
			klog.Infof("Ignore non-disk file '%v'", hdr.Name)
		}
	}
	return nil, nil, nil, fr.Close()
}
