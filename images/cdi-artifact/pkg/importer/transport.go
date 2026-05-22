/*
Copyright 2020 The CDI Authors.

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
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/image"
	"github.com/containers/image/v5/oci/archive"
	"github.com/containers/image/v5/pkg/blobinfocache"
	"github.com/containers/image/v5/types"
	"github.com/pkg/errors"

	"k8s.io/klog/v2"

	"kubevirt.io/containerized-data-importer/pkg/common"
	metrics "kubevirt.io/containerized-data-importer/pkg/monitoring/metrics/cdi-importer"
	"kubevirt.io/containerized-data-importer/pkg/util"
	prometheusutil "kubevirt.io/containerized-data-importer/pkg/util/prometheus"
)

const (
	whFilePrefix = ".wh."

	registrySchemeDocker = "docker"
	registrySchemeOCI    = "oci-archive"

	// transferScratchMaxProgress is the upper bound of the TransferScratch
	// phase contribution to the overall import progress (0..100). The remaining
	// half (50..100) is occupied by the Convert phase, see qemu.reportProgress.
	transferScratchMaxProgress = 49.0
)

// scaledProgressMetric adapts a 0..100 prometheus.ProgressMetric so that the
// underlying counter advances only within the [low, high] sub-range. It is
// used to project the per-layer download progress reported by
// prometheusutil.ProgressReader (which always works in 0..100) into the
// 0..transferScratchMaxProgress slice of the shared import_progress counter.
type scaledProgressMetric struct {
	base      prometheusutil.ProgressMetric
	low, high float64
}

func (s scaledProgressMetric) Get() (float64, error) {
	cur, err := s.base.Get()
	if err != nil {
		return 0, err
	}
	if cur <= s.low {
		return 0, nil
	}
	if cur >= s.high {
		return 100, nil
	}
	return (cur - s.low) * 100.0 / (s.high - s.low), nil
}

func (s scaledProgressMetric) Add(delta float64) {
	if delta <= 0 {
		return
	}
	s.base.Add(delta * (s.high - s.low) / 100.0)
}

func (s scaledProgressMetric) Delete() { s.base.Delete() }

// transferProgressMetric returns a 0..transferScratchMaxProgress projection of
// the shared import_progress counter that ProgressReader can drive in 0..100.
func transferProgressMetric() prometheusutil.ProgressMetric {
	ownerUID, _ := util.ParseEnvVar(common.OwnerUID, false)
	return scaledProgressMetric{
		base: metrics.Progress(ownerUID),
		low:  0,
		high: transferScratchMaxProgress,
	}
}

func commandTimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func buildSourceContext(accessKey, secKey, certDir string, insecureRegistry bool) *types.SystemContext {
	ctx := &types.SystemContext{}
	if accessKey != "" && secKey != "" {
		ctx.DockerAuthConfig = &types.DockerAuthConfig{
			Username: accessKey,
			Password: secKey,
		}
	}
	if certDir != "" {
		ctx.DockerCertPath = certDir
		ctx.DockerDaemonCertPath = certDir
	}

	if insecureRegistry {
		ctx.DockerDaemonInsecureSkipTLSVerify = true
		ctx.DockerInsecureSkipTLSVerify = types.NewOptionalBool(true)
	}

	return ctx
}

func readImageSource(ctx context.Context, sys *types.SystemContext, img string) (types.ImageSource, error) {
	ref, err := parseImageName(img)
	if err != nil {
		klog.Errorf("Could not parse image: %v", err)
		return nil, errors.Wrap(err, "Could not parse image")
	}

	src, err := ref.NewImageSource(ctx, sys)
	if err != nil {
		klog.Errorf("Could not create image reference: %v", err)
		return nil, NewImagePullFailedError(err)
	}

	return src, nil
}

func parseImageName(img string) (types.ImageReference, error) {
	parts := strings.SplitN(img, ":", 2)
	if len(parts) != 2 {
		return nil, errors.Errorf(`Invalid image name "%s", expected colon-separated transport:reference`, img)
	}
	switch parts[0] {
	case registrySchemeDocker:
		return docker.ParseReference(parts[1])
	case registrySchemeOCI:
		return archive.ParseReference(parts[1])
	}
	return nil, errors.Errorf(`Invalid image name "%s", unknown transport`, img)
}

func closeImage(src types.ImageSource) {
	if err := src.Close(); err != nil {
		klog.Warningf("Could not close image source: %v ", err)
	}
}

func hasPrefix(path string, pathPrefix string) bool {
	return strings.HasPrefix(path, pathPrefix) ||
		strings.HasPrefix(path, "./"+pathPrefix)
}

func isWhiteout(path string) bool {
	return strings.HasPrefix(filepath.Base(path), whFilePrefix)
}

func isDir(hdr *tar.Header) bool {
	return hdr.Typeflag == tar.TypeDir
}

func processLayer(ctx context.Context,
	sys *types.SystemContext,
	src types.ImageSource,
	layer types.BlobInfo,
	destDir string,
	pathPrefix string,
	cache types.BlobInfoCache,
	stopAtFirst bool) (bool, error) {
	var reader io.ReadCloser
	reader, _, err := src.GetBlob(ctx, layer, cache)
	if err != nil {
		klog.Errorf("Could not read layer: %v", err)
		return false, errors.Wrap(err, "Could not read layer")
	}
	// Track download progress of the current layer in the lower half of the
	// shared import_progress counter (0..transferScratchMaxProgress). The
	// Convert phase later fills 50..100.
	if layer.Size > 0 {
		progressReader := prometheusutil.NewProgressReader(reader, transferProgressMetric(), uint64(layer.Size))
		progressReader.StartTimedUpdate()
		reader = progressReader
	}
	fr, err := NewFormatReaders(reader, 0)
	if err != nil {
		return false, errors.Wrap(err, "Could not read layer")
	}
	defer fr.Close()

	tarReader := tar.NewReader(fr.TopReader())
	found := false
	for {
		hdr, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break // End of archive
		}
		if err != nil {
			klog.Errorf("Error reading layer: %v", err)
			return false, errors.Wrap(err, "Error reading layer")
		}

		if hasPrefix(hdr.Name, pathPrefix) && !isWhiteout(hdr.Name) && !isDir(hdr) {
			klog.Infof("File '%v' found in the layer", hdr.Name)
			destFile, err := safeJoinPaths(destDir, hdr.Name)
			if err != nil {
				klog.Errorf("Error sanitizing archive path: %v", err)
				return false, errors.Wrap(err, "Error sanitizing archive path")
			}

			if err = os.MkdirAll(filepath.Dir(destFile), os.ModePerm); err != nil {
				klog.Errorf("Error creating output file's directory: %v", err)
				return false, errors.Wrap(err, "Error creating output file's directory")
			}

			if err := streamDataToFile(tarReader, destFile); err != nil {
				klog.Errorf("Error copying file: %v", err)
				return false, errors.Wrap(err, "Error copying file")
			}

			found = true
			if stopAtFirst {
				return found, nil
			}
		}
	}

	return found, nil
}

// Sanitize archive file pathing from "G305: Zip Slip vulnerability"
// https://security.snyk.io/research/zip-slip-vulnerability
func safeJoinPaths(dir, path string) (v string, err error) {
	v = filepath.Join(dir, path)
	wantPrefix := filepath.Clean(dir) + string(os.PathSeparator)

	if strings.HasPrefix(v, wantPrefix) {
		return v, nil
	}

	return "", fmt.Errorf("%s: %s", "content filepath is tainted", path)
}

func copyRegistryImage(url, destDir, pathPrefix, accessKey, secKey, certDir string, insecureRegistry, stopAtFirst bool) (*types.ImageInspectInfo, error) {
	klog.Infof("Downloading image from '%v', copying file from '%v' to '%v'", url, pathPrefix, destDir)

	ctx, cancel := commandTimeoutContext()
	defer cancel()
	srcCtx := buildSourceContext(accessKey, secKey, certDir, insecureRegistry)

	src, err := readImageSource(ctx, srcCtx, url)
	if err != nil {
		return nil, err
	}
	defer closeImage(src)

	imgCloser, err := image.FromSource(ctx, srcCtx, src)
	if err != nil {
		klog.Errorf("Error retrieving image: %v", err)
		return nil, errors.Wrap(err, "Error retrieving image")
	}
	defer imgCloser.Close()

	cache := blobinfocache.DefaultCache(srcCtx)
	found := false
	layers := imgCloser.LayerInfos()

	for _, layer := range layers {
		klog.Infof("Processing layer %+v", layer)

		found, err = processLayer(ctx, srcCtx, src, layer, destDir, pathPrefix, cache, stopAtFirst)
		if found {
			break
		}
		if err != nil {
			// Skipping layer and trying the next one.
			// Error already logged in processLayer
			continue
		}
	}

	if !found {
		klog.Errorf("Failed to find VM disk image file in the container image")
		return nil, errors.New("Failed to find VM disk image file in the container image")
	}

	info, err := imgCloser.Inspect(ctx)
	if err != nil {
		return nil, err
	}

	return info, nil
}

// CopyRegistryImage download image from registry with docker image API. It will extract first file under the pathPrefix
// url: source registry url.
// destDir: the scratch space destination.
// pathPrefix: path to extract files from.
// accessKey: accessKey for the registry described in url.
// secKey: secretKey for the registry described in url.
// certDir: directory public CA keys are stored for registry identity verification
// insecureRegistry: boolean if true will allow insecure registries.
func CopyRegistryImage(url, destDir, pathPrefix, accessKey, secKey, certDir string, insecureRegistry bool) (*types.ImageInspectInfo, error) {
	return copyRegistryImage(url, destDir, pathPrefix, accessKey, secKey, certDir, insecureRegistry, true)
}
