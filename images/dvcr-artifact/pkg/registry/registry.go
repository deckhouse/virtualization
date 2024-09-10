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

package registry

import (
	"archive/tar"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/stream"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	"k8s.io/klog/v2"
	"kubevirt.io/containerized-data-importer/pkg/importer"
	"kubevirt.io/containerized-data-importer/pkg/util"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/datasource"
	importerrs "github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/errors"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/monitoring"
)

// FIXME(ilya-lesikov): certdir

const (
	imageLabelSourceImageSize        = "source-image-size"
	imageLabelSourceImageVirtualSize = "source-image-virtual-size"
	imageLabelSourceImageFormat      = "source-image-format"
)

type ImportRes struct {
	SourceImageSize uint64
	VirtualSize     uint64
	AvgSpeed        uint64
	Format          string
}

type ImageInfo struct {
	VirtualSize uint64 `json:"virtual-size"`
	Format      string `json:"format"`
}

const (
	imageInfoSize        = 64 * 1024 * 1024
	pipeBufSize          = 64 * 1024 * 1024
	tempImageInfoPattern = "tempfile"
	isoImageType         = "iso"
)

type DataProcessor struct {
	ds            datasource.DataSourceInterface
	destUsername  string
	destPassword  string
	destImageName string
	sha256Sum     string
	md5Sum        string
	destInsecure  bool
}

type DestinationRegistry struct {
	ImageName string
	Username  string
	Password  string
	Insecure  bool
}

func NewDataProcessor(ds datasource.DataSourceInterface, dest DestinationRegistry, sha256Sum, md5Sum string) (*DataProcessor, error) {
	return &DataProcessor{
		ds,
		dest.Username,
		dest.Password,
		dest.ImageName,
		sha256Sum,
		md5Sum,
		dest.Insecure,
	}, nil
}

func createImgFromDevice(deviceName, imgName string) error {
	blockDevice, err := os.Open(deviceName)
	if err != nil {
		fmt.Errorf("can not get open device")
		return err
	}
	defer blockDevice.Close()

	imgFile, err := os.Create(imgName)
	if err != nil {
		fmt.Errorf("can not get create image")
		return err
	}
	defer imgFile.Close()

	buf := make([]byte, 1024*1024)
	_, err = io.CopyBuffer(imgFile, blockDevice, buf)
	if err != nil {
		fmt.Errorf("can not get copy to image")
		return err
	}

	return nil
}

func (p DataProcessor) ProcessFromBlockDevice(ctx context.Context) (ImportRes, error) {
	blockDevicePath := "/dev/xvda"
	uuid, err := uuid.NewUUID()
	imgPath := "/tmp/" + uuid.String() + ".raw"

	err = createImgFromDevice(blockDevicePath, imgPath)
	if err != nil {
		fmt.Printf("Error creating image: %v\n", err)
		return ImportRes{}, err
	}

	sourceImageFilename := uuid.String() + ".raw"

	fileInfo, err := os.Stat(imgPath)
	if err != nil {
		fmt.Println("get size error:", err)
		return ImportRes{}, err
	}

	sourceImageSize := fileInfo.Size()

	file, err := os.Open(imgPath)
	if err != nil {
		fmt.Printf("Error opening img file: %v\n", err)
		return ImportRes{}, err
	}
	defer file.Close()

	progressMeter := monitoring.NewProgressMeter(file, uint64(sourceImageSize))
	progressMeter.Start()
	defer progressMeter.Stop()

	pipeReader, pipeWriter := io.Pipe()

	informer := NewImageInformer()

	errsGroup, ctx := errgroup.WithContext(ctx)
	errsGroup.Go(func() error {
		return p.inspectAndStreamSourceImage(ctx, sourceImageFilename, int(sourceImageSize), progressMeter, pipeWriter, informer)
	})
	errsGroup.Go(func() error {
		defer pipeReader.Close()
		return p.uploadLayersAndImage(ctx, pipeReader, int(sourceImageSize), informer)
	})

	err = errsGroup.Wait()
	if err != nil {
		return ImportRes{}, err
	}

	select {
	case <-informer.Wait():
	default:
		return ImportRes{}, errors.New("unexpected waiting for the informer, please report a bug")
	}

	return ImportRes{
		SourceImageSize: uint64(sourceImageSize),
		VirtualSize:     informer.GetVirtualSize(),
		AvgSpeed:        progressMeter.GetAvgSpeed(),
		Format:          informer.GetFormat(),
	}, nil
}

func (p DataProcessor) Process(ctx context.Context) (ImportRes, error) {
	sourceImageFilename, err := p.ds.Filename()
	if err != nil {
		return ImportRes{}, fmt.Errorf("error getting source filename: %w", err)
	}

	sourceImageSize, err := p.ds.Length()
	if err != nil {
		return ImportRes{}, fmt.Errorf("error getting source image size: %w", err)
	}

	if sourceImageSize == 0 {
		return ImportRes{}, fmt.Errorf("zero data source image size")
	}

	sourceImageReader, err := p.ds.ReadCloser()
	if err != nil {
		return ImportRes{}, fmt.Errorf("error getting source image reader: %w", err)
	}

	// Wrap data source reader with progress and speed metrics.
	progressMeter := monitoring.NewProgressMeter(sourceImageReader, uint64(sourceImageSize))
	progressMeter.Start()
	defer progressMeter.Stop()

	pipeReader, pipeWriter := io.Pipe()

	informer := NewImageInformer()

	errsGroup, ctx := errgroup.WithContext(ctx)
	errsGroup.Go(func() error {
		return p.inspectAndStreamSourceImage(ctx, sourceImageFilename, sourceImageSize, progressMeter, pipeWriter, informer)
	})
	errsGroup.Go(func() error {
		defer pipeReader.Close()
		return p.uploadLayersAndImage(ctx, pipeReader, sourceImageSize, informer)
	})

	err = errsGroup.Wait()
	if err != nil {
		return ImportRes{}, err
	}

	select {
	case <-informer.Wait():
	default:
		return ImportRes{}, errors.New("unexpected waiting for the informer, please report a bug")
	}

	return ImportRes{
		SourceImageSize: uint64(sourceImageSize),
		VirtualSize:     informer.GetVirtualSize(),
		AvgSpeed:        progressMeter.GetAvgSpeed(),
		Format:          informer.GetFormat(),
	}, nil
}

func (p DataProcessor) inspectAndStreamSourceImage(
	ctx context.Context,
	sourceImageFilename string,
	sourceImageSize int,
	sourceImageReader io.ReadCloser,
	pipeWriter io.WriteCloser,
	informer *ImageInformer,
) error {
	var tarWriter *tar.Writer
	{
		tarWriter = tar.NewWriter(pipeWriter)
		header := &tar.Header{
			Name:     path.Join("disk", sourceImageFilename),
			Size:     int64(sourceImageSize),
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("error writing tar header: %w", err)
		}
	}

	var checksumWriters []io.Writer
	var checksumCheckFuncList []func() error
	{
		if p.sha256Sum != "" {
			hash := sha256.New()
			checksumWriters = append(checksumWriters, hash)
			checksumCheckFuncList = append(checksumCheckFuncList, func() error {
				sum := hex.EncodeToString(hash.Sum(nil))
				if sum != p.sha256Sum {
					return importerrs.NewBadImageChecksumError("sha256", p.sha256Sum, sum)
				}

				return nil
			})
		}

		if p.md5Sum != "" {
			hash := md5.New()
			checksumWriters = append(checksumWriters, hash)
			checksumCheckFuncList = append(checksumCheckFuncList, func() error {
				sum := hex.EncodeToString(hash.Sum(nil))
				if sum != p.md5Sum {
					return importerrs.NewBadImageChecksumError("md5", p.md5Sum, sum)
				}

				return nil
			})
		}
	}

	var streamWriter io.Writer
	{
		writers := []io.Writer{tarWriter}
		writers = append(writers, checksumWriters...)
		streamWriter = io.MultiWriter(writers...)
	}

	errsGroup, ctx := errgroup.WithContext(ctx)

	imageInfoReader, imageInfoWriter := io.Pipe()

	errsGroup.Go(func() error {
		defer tarWriter.Close()
		defer pipeWriter.Close()
		defer sourceImageReader.Close()
		defer imageInfoWriter.Close()

		klog.Infoln("Streaming from the source")
		doneSize, err := io.Copy(streamWriter, io.TeeReader(sourceImageReader, imageInfoWriter))
		if err != nil {
			return fmt.Errorf("error copying from the source: %w", err)
		}

		if doneSize != int64(sourceImageSize) {
			return fmt.Errorf("source image size mismatch: %d != %d", doneSize, sourceImageSize)
		}

		for _, checksumCheckFunc := range checksumCheckFuncList {
			if err = checksumCheckFunc(); err != nil {
				return err
			}
		}

		klog.Infoln("Source streaming completed")

		return nil
	})

	errsGroup.Go(func() error {
		defer imageInfoReader.Close()

		info, err := getImageInfo(ctx, imageInfoReader)
		if err != nil {
			return err
		}

		informer.Set(info.VirtualSize, info.Format)

		return nil
	})

	return errsGroup.Wait()
}

func (p DataProcessor) uploadLayersAndImage(
	ctx context.Context,
	pipeReader io.ReadCloser,
	sourceImageSize int,
	informer *ImageInformer,
) error {
	nameOpts := destNameOptions(p.destInsecure)
	remoteOpts := destRemoteOptions(ctx, p.destUsername, p.destPassword, p.destInsecure)
	image := empty.Image

	ref, err := name.ParseReference(p.destImageName, nameOpts...)
	if err != nil {
		return fmt.Errorf("error parsing image name: %w", err)
	}

	repo, err := name.NewRepository(ref.Context().Name(), nameOpts...)
	if err != nil {
		return fmt.Errorf("error constructing new repository: %w", err)
	}

	layer := stream.NewLayer(pipeReader)

	klog.Infoln("Uploading layer to registry")
	if err := remote.WriteLayer(repo, layer, remoteOpts...); err != nil {
		return fmt.Errorf("error uploading layer: %w", err)
	}
	klog.Infoln("Layer uploaded")

	cnf, err := image.ConfigFile()
	if err != nil {
		return fmt.Errorf("error getting image config: %w", err)
	}

	informer.Wait()

	klog.Infof("Got image info: virtual size: %d, format: %s", informer.GetVirtualSize(), informer.GetFormat())

	cnf.Config.Labels = map[string]string{}
	cnf.Config.Labels[imageLabelSourceImageVirtualSize] = fmt.Sprintf("%d", informer.GetVirtualSize())
	cnf.Config.Labels[imageLabelSourceImageSize] = fmt.Sprintf("%d", sourceImageSize)
	cnf.Config.Labels[imageLabelSourceImageFormat] = informer.GetFormat()

	image, err = mutate.ConfigFile(image, cnf)
	if err != nil {
		return fmt.Errorf("error mutating image config: %w", err)
	}

	image, err = mutate.AppendLayers(image, layer)
	if err != nil {
		return fmt.Errorf("error appending layer to image: %w", err)
	}

	klog.Infof("Uploading image %q to registry", p.destImageName)
	if err = remote.Write(ref, image, remoteOpts...); err != nil {
		return fmt.Errorf("error uploading image: %w", err)
	}

	return nil
}

func getImageInfo(ctx context.Context, sourceReader io.ReadCloser) (ImageInfo, error) {
	formatSourceReaders, err := importer.NewFormatReaders(sourceReader, 0)
	if err != nil {
		return ImageInfo{}, fmt.Errorf("error creating format readers: %w", err)
	}

	var uncompressedN int64
	var tempImageInfoFile *os.File

	klog.Infoln("Write image info to temp file")
	{
		tempImageInfoFile, err = os.CreateTemp("", tempImageInfoPattern)
		if err != nil {
			return ImageInfo{}, fmt.Errorf("error creating temp file: %w", err)
		}

		uncompressedN, err = io.CopyN(tempImageInfoFile, formatSourceReaders.TopReader(), imageInfoSize)
		if err != nil && !errors.Is(err, io.EOF) {
			return ImageInfo{}, fmt.Errorf("error writing to temp file: %w", err)
		}

		if err = tempImageInfoFile.Close(); err != nil {
			return ImageInfo{}, fmt.Errorf("error closing temp file: %w", err)
		}
	}

	klog.Infoln("Get image info from temp file")
	var imageInfo ImageInfo
	{
		cmd := exec.CommandContext(ctx, "qemu-img", "info", "--output=json", tempImageInfoFile.Name())
		rawOut, err := cmd.Output()
		if err != nil {
			return ImageInfo{}, fmt.Errorf("error running qemu-img info: %w", err)
		}

		klog.Infoln("Qemu-img command output:", string(rawOut))

		if err = json.Unmarshal(rawOut, &imageInfo); err != nil {
			return ImageInfo{}, fmt.Errorf("error parsing qemu-img info output: %w", err)
		}

		if imageInfo.Format != "raw" {
			// It's necessary to read everything from the original image to avoid blocking.
			_, err = io.Copy(&util.EmptyWriter{}, sourceReader)
			if err != nil {
				return ImageInfo{}, fmt.Errorf("error copying to nowhere: %w", err)
			}

			return imageInfo, nil
		}
	}

	// `qemu-img` command does not support getting information about iso files.
	// It is necessary to obtain this information in another way (using the `file` command).
	klog.Infoln("Check the image as it may be an iso")
	{
		cmd := exec.CommandContext(ctx, "file", "-b", tempImageInfoFile.Name())
		rawOut, err := cmd.Output()
		if err != nil {
			return ImageInfo{}, fmt.Errorf("error running file info: %w", err)
		}

		out := string(rawOut)

		klog.Infoln("File command output:", out)

		if strings.HasPrefix(strings.ToLower(out), isoImageType) {
			imageInfo.Format = isoImageType
		}

		// Count uncompressed size of source image.
		n, err := io.Copy(&util.EmptyWriter{}, formatSourceReaders.TopReader())
		if err != nil {
			return ImageInfo{}, fmt.Errorf("error copying to nowhere: %w", err)
		}

		imageInfo.VirtualSize = uint64(uncompressedN + n)

		return imageInfo, nil
	}
}

func destNameOptions(destInsecure bool) []name.Option {
	nameOpts := []name.Option{}

	if destInsecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	return nameOpts
}

func destRemoteOptions(ctx context.Context, destUsername, destPassword string, destInsecure bool) []remote.Option {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: destInsecure,
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig

	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithTransport(transport),
		remote.WithAuth(&authn.Basic{Username: destUsername, Password: destPassword}),
	}

	return remoteOpts
}
