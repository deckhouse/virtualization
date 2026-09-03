/*
Copyright 2026 Flant JSC

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

package blockdevice

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

// The suite verifies the checksum section end to end, for both data sources that
// have one - HTTP and Upload: the sums are computed here from the very file the
// importer downloads and the uploader receives, so the specs stay correct when
// the source image is replaced.
//
// The source is the small non-bootable test image: nothing boots from it, and
// the specs only care about the checksum verdict, so a 12 MiB file keeps even
// the Streebog specs cheap - cheap enough for every spec to download and hash
// its own copy in BeforeEach, instead of sharing one through an Ordered
// container and serializing the whole suite.
var _ = Describe("ImageChecksums", Label(
	label.SIGStorage,
	precheck.PrecheckDefaultStorageClass,
), func() {
	var (
		f *framework.Framework

		scPtr *string
		// checksum holds every checksum of the source image, and imagePath the
		// local copy the upload specs send to the uploader.
		checksum  *v1alpha2.Checksum
		imagePath string
	)

	BeforeEach(func(ctx context.Context) {
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
		setupProject(ctx, f, "image-checksums")

		scPtr = defaultStorageClass()

		By("Computing the checksums of the source image", func() {
			var err error
			imagePath, err = downloadImageToTempFile(object.ImageTestDataQCOW)
			Expect(err).NotTo(HaveOccurred(), "failed to download the source image")
			DeferCleanup(func() {
				removeErr := os.Remove(imagePath)
				Expect(removeErr == nil || errors.Is(removeErr, os.ErrNotExist)).To(BeTrue(),
					"failed to remove the source image %q: %v", imagePath, removeErr)
			})

			checksum, err = computeChecksums(imagePath)
			Expect(err).NotTo(HaveOccurred(), "failed to compute the checksums of the source image")
		})
	})

	It("provisions a VirtualImage verified with every supported algorithm", func(ctx context.Context) {
		vi := newVirtualImageOnDVCR("vi-checksum-every-algorithm",
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, checksum, nil),
		)
		vi.Namespace = f.Namespace().Name

		obs := viobs.StartObserver(ctx, f, vi)
		obs.Never(viobs.BeFailed())

		By("Creating the VirtualImage", func() {
			Expect(f.CreateWithDeferredDeletion(ctx, vi)).To(Succeed())
		})

		By("Waiting for the VirtualImage to be Ready", func() {
			Expect(obs.WaitFor(viobs.BeReady(), framework.LongTimeout)).To(Succeed())
		})
	})

	It("fails a VirtualImage whose SHA-512 checksum does not match", func(ctx context.Context) {
		vi := newVirtualImageOnDVCR("vi-checksum-mismatch",
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, &v1alpha2.Checksum{
				SHA512: mismatchedSum(checksum.SHA512),
			}, nil),
		)
		vi.Namespace = f.Namespace().Name

		// BeFailed is expected here, so the invariant the other specs assert is
		// deliberately not registered on this observer, and the fail-fast rules
		// are told to leave the image and its importer pod alone: the Failed
		// phase is terminal and the pod keeps crash-looping until the cleanup,
		// so a rule would otherwise fail the spec once its grace elapses.
		f.ExpectFailure(vi)
		obs := viobs.StartObserver(ctx, f, vi)

		By("Creating the VirtualImage", func() {
			Expect(f.CreateWithDeferredDeletion(ctx, vi)).To(Succeed())
		})

		// A mismatch is a permanent error for the importer: it is reported after
		// the single pass over the source, without spending the retry backoff on
		// re-downloads, so the ordinary timeout of an import applies here too.
		By("Waiting for the VirtualImage to report the checksum mismatch", func() {
			Expect(obs.WaitFor(viobs.BeChecksumMismatch("sha512"), framework.LongTimeout)).To(Succeed())
		})
	})

	It("provisions an uploaded VirtualImage verified with every supported algorithm", func(ctx context.Context) {
		vi := newVirtualImageOnDVCR("vi-upload-checksum",
			vibuilder.WithDatasource(v1alpha2.VirtualImageDataSource{
				Type:   v1alpha2.DataSourceTypeUpload,
				Upload: &v1alpha2.DataSourceUpload{Checksum: checksum},
			}),
		)

		uploadVirtualImageAndWait(ctx, f, vi, imagePath)
	})

	It("fails an uploaded VirtualImage whose SHA-512 checksum does not match", func(ctx context.Context) {
		vi := newVirtualImageOnDVCR("vi-upload-checksum-mismatch",
			vibuilder.WithDatasource(v1alpha2.VirtualImageDataSource{
				Type: v1alpha2.DataSourceTypeUpload,
				Upload: &v1alpha2.DataSourceUpload{Checksum: &v1alpha2.Checksum{
					SHA512: mismatchedSum(checksum.SHA512),
				}},
			}),
		)
		vi.Namespace = f.Namespace().Name

		// BeFailed is expected here, so the invariant the other specs assert is
		// deliberately not registered on this observer, and the fail-fast rules
		// are told to leave the image and its uploader pod alone (see the HTTP
		// mismatch spec above).
		f.ExpectFailure(vi)
		obs := viobs.StartObserver(ctx, f, vi)

		By("Creating the VirtualImage", func() {
			Expect(f.CreateWithDeferredDeletion(ctx, vi)).To(Succeed())
		})

		By("Waiting for the VirtualImage to expose upload URLs", func() {
			Expect(obs.WaitFor(viobs.BeReadyForUserUpload(), framework.LongTimeout)).To(Succeed())
		})

		By("Uploading data that does not match the checksum", func() {
			Expect(f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vi), vi)).To(Succeed())
			Expect(vi.Status.ImageUploadURLs).NotTo(BeNil())
			Expect(vi.Status.ImageUploadURLs.External).NotTo(BeEmpty())

			// The error is expected and deliberately ignored: the uploader answers
			// a mismatch with a 5xx, and the verdict this spec is after is the one
			// on the resource. Retrying is still required, since the very same 5xx
			// is what an ingress that is not ready yet returns, and a single
			// attempt would leave the spec waiting for data that never arrived.
			if err := doRetriableUploadAttempt(vi.Status.ImageUploadURLs.External, imagePath); err != nil {
				GinkgoWriter.Printf("upload rejected as expected: %v\n", err)
			}
		})

		By("Waiting for the VirtualImage to report the checksum mismatch", func() {
			Expect(obs.WaitFor(viobs.BeChecksumMismatch("sha512"), framework.LongTimeout)).To(Succeed())
		})
	})

	It("provisions a VirtualDisk verified with a GOST R 34.11-2012 checksum", func(ctx context.Context) {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-checksum-gost"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{
				URL: object.ImageTestDataQCOW,
				Checksum: &v1alpha2.Checksum{
					Streebog256: checksum.Streebog256,
					Streebog512: checksum.Streebog512,
				},
			}),
			vdbuilder.WithStorageClass(scPtr),
			vdbuilder.WithSize(ptr.To(resource.MustParse(vdCreationImageSize))),
		)

		obs := vdobs.StartObserver(ctx, f, vd)
		obs.Never(vdobs.BeFailed())

		By("Creating the VirtualDisk", func() {
			Expect(f.CreateWithDeferredDeletion(ctx, vd)).To(Succeed())
		})

		// The checksums are verified while the source is downloaded into DVCR, but
		// on a WaitForFirstConsumer storage class the disk only reaches Ready once
		// it has a consumer, so a VirtualMachine is created below to be one.
		By("Waiting for the VirtualDisk to settle before creating the consumer", func() {
			Expect(obs.WaitFor(expectedDiskPhaseBeforeVM(ctx, f, vd), framework.LongTimeout)).To(Succeed())
		})

		// The source of this suite is the non-bootable test image, so the guest
		// never boots and its agent never reports ready. Running is all this spec
		// needs: by then the volume has been provisioned from the verified data.
		vm := object.NewMinimalVM("vm-checksum-consumer-", f.Namespace().Name,
			vmbuilder.WithDisks(vd),
			vmbuilder.WithProvisioning(nil),
		)

		By("Creating a VirtualMachine to consume the VirtualDisk", func() {
			Expect(f.CreateWithDeferredDeletion(ctx, vm)).To(Succeed())
		})

		vmObs := vmobs.StartObserver(ctx, f, vm)
		vmObs.Never(vmobs.BeFailed())

		By("Waiting for the VirtualDisk to be Ready", func() {
			Expect(obs.WaitFor(vdobs.BeReady(), framework.LongTimeout)).To(Succeed())
		})

		By("Waiting for the VirtualMachine to be Running", func() {
			Expect(vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
		})
	})
})

// GOST R 34.11-2012 checksums of the source image.
//
// Unlike the algorithms below, Streebog has no implementation in the standard
// library, and pulling a GOST crypto library into the test module only to hash
// a fixed file is not worth it. The sums are pinned instead, and
// sourceImageSHA256 guards them: replace the source image, and the guard fails
// with the sum to put here, instead of the specs failing as a checksum
// mismatch the module never produced.
const (
	sourceImageSHA256      = "107aa7c11ee731c7867a5b037e9af373857aa246a4b63f7f06a2fcd61e5770d5"
	sourceImageStreebog256 = "1fc7fb18abbd6e8b3477c0ca5b5e6e55b2e1dac4a50d64201fd3872fe206cc41"
	sourceImageStreebog512 = "dd257ee0c4c38dfa0328457b8cd94663021cfc75c1525ae4ff78d8063a9dd437c63b67d4ced10ba8c025b51cc02127cf231348c76f5f8c29bcf9fe473edd4036"
)

// computeChecksums hashes the file with every algorithm the standard library
// provides, in a single pass, and completes the section with the pinned
// Streebog sums after checking the file is the one they were computed from.
func computeChecksums(path string) (*v1alpha2.Checksum, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			Expect(closeErr).NotTo(HaveOccurred(), "failed to close the source image")
		}
	}()

	hashes := map[string]hash.Hash{
		"md5":    md5.New(),
		"sha1":   sha1.New(),
		"sha256": sha256.New(),
		"sha512": sha512.New(),
	}

	writers := make([]io.Writer, 0, len(hashes))
	for _, h := range hashes {
		writers = append(writers, h)
	}

	if _, err = io.Copy(io.MultiWriter(writers...), file); err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}

	sum := func(algorithm string) string {
		return hex.EncodeToString(hashes[algorithm].Sum(nil))
	}

	if got := sum("sha256"); got != sourceImageSHA256 {
		return nil, fmt.Errorf(
			"the source image has changed (sha256 %s, expected %s): recompute the pinned Streebog sums, e.g. with `gostsum --mode 256` and `--mode 512`, and update sourceImageSHA256 as well",
			got, sourceImageSHA256,
		)
	}

	return &v1alpha2.Checksum{
		MD5:         sum("md5"),
		SHA1:        sum("sha1"),
		SHA256:      sum("sha256"),
		SHA512:      sum("sha512"),
		Streebog256: sourceImageStreebog256,
		Streebog512: sourceImageStreebog512,
	}, nil
}

// mismatchedSum turns a checksum into one the image cannot match. The CRD
// validates both the length and the alphabet, so a wrong sum still has to be a
// well-formed hex string of the same size — otherwise the resource is rejected
// on apply and the importer never runs.
func mismatchedSum(sum string) string {
	Expect(sum).NotTo(BeEmpty(), "the source checksum to spoil must not be empty")

	if sum[0] == '0' {
		return "1" + sum[1:]
	}

	return "0" + sum[1:]
}
