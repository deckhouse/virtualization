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
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vdsnapshotbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vdsnapshot"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vdobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vd"
	viobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vi"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// vdCreationStorageClass is the storage class used by VirtualDiskCreation tests until
// the e2e environment exposes this as a configurable parameter.
const vdCreationStorageClass = "linstor-thin-r1-immediate"

const vdCreationBlankSize = "64Mi"

var _ = Describe("VirtualDiskCreation", Ordered, Label(precheck.NoPrecheck), func() {
	var (
		f   *framework.Framework
		ctx context.Context

		scPtr *string
	)

	BeforeAll(func() {
		ctx = context.Background()
		f = framework.NewFramework("vd-creation")
		f.Before()
		DeferCleanup(f.After)

		scPtr = ptr.To(vdCreationStorageClass)
	})

	It("provisions a VirtualDisk from HTTP data source", Label(precheck.NoPrecheck), func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-http"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{URL: object.ImageTestDataQCOW}),
			vdbuilder.WithStorageClass(scPtr),
		)

		createVirtualDiskAndWait(ctx, f, vd)
	})

	It("provisions a VirtualDisk from Upload data source", Label(precheck.NoPrecheck), func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-upload"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDatasource(&v1alpha2.VirtualDiskDataSource{
				Type: v1alpha2.DataSourceTypeUpload,
			}),
			vdbuilder.WithStorageClass(scPtr),
		)

		var uploadFilePath string
		By("Downloading source image to upload", func() {
			var err error
			uploadFilePath, err = downloadImageToTempFile(object.ImageTestDataQCOW)
			Expect(err).NotTo(HaveOccurred(), "failed to download upload source image")
			DeferCleanup(func() {
				removeErr := os.Remove(uploadFilePath)
				Expect(removeErr == nil || errors.Is(removeErr, os.ErrNotExist)).To(BeTrue(),
					"failed to remove upload source file %q: %v", uploadFilePath, removeErr)
			})
		})

		obs := vdobs.StartObserver(ctx, f, vd)
		obs.Never(vdobs.BeFailed())
		obs.Always(vdobs.BeStorageClassReady())
		obs.Always(vdobs.BeDataSourceReady())
		obs.Always(vdobs.HaveNonDecreasingProgress())

		By("Creating VirtualDisk", func() {
			err := f.CreateWithDeferredDeletion(ctx, vd)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Waiting for the VirtualDisk to expose upload URLs", func() {
			err := obs.WaitFor(vdobs.BeReadyForUserUpload(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Allowing ingress-nginx to reach the uploader pod (workaround)", func() {
			err := allowIngressNginxToUploaderNetworkPolicy(ctx, f, vd.Namespace, vd.UID)
			Expect(err).NotTo(HaveOccurred(), "failed to patch uploader NetworkPolicy")
		})

		By("Uploading data to the VirtualDisk", func() {
			err := f.Clients.GenericClient().Get(ctx, crclient.ObjectKeyFromObject(vd), vd)
			Expect(err).NotTo(HaveOccurred())
			Expect(vd.Status.ImageUploadURLs).NotTo(BeNil())
			Expect(vd.Status.ImageUploadURLs.External).NotTo(BeEmpty())

			err = doRetriableUploadAttempt(vd.Status.ImageUploadURLs.External, uploadFilePath)
			Expect(err).NotTo(HaveOccurred(), "upload should succeed")
		})

		err := obs.WaitFor(vdobs.BeReady(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	It("provisions a VirtualDisk from ContainerImage (registry) data source", Label(precheck.NoPrecheck), func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-registry"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceContainerImage(object.ImageURLContainerImage, "", nil),
			vdbuilder.WithStorageClass(scPtr),
		)

		createVirtualDiskAndWait(ctx, f, vd)
	})

	It("provisions a VirtualDisk from a VirtualImage on DVCR", Label(precheck.NoPrecheck), func() {
		baseVI := vibuilder.New(
			vibuilder.WithName("vi-source-dvcr"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
		)

		viObs := viobs.StartObserver(ctx, f, baseVI)
		viObs.Never(viobs.BeFailed())

		By("Creating base VirtualImage on DVCR", func() {
			err := f.CreateWithDeferredDeletion(ctx, baseVI)
			Expect(err).NotTo(HaveOccurred())

			err = viObs.WaitFor(viobs.BeReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		vd := vdbuilder.New(
			vdbuilder.WithName("vd-from-vi"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualImage, baseVI.Name),
			vdbuilder.WithStorageClass(scPtr),
		)

		createVirtualDiskAndWait(ctx, f, vd)
	})

	It("provisions a VirtualDisk from a VirtualImage on PVC", Label(precheck.NoPrecheck), func() {
		baseVI := vibuilder.New(
			vibuilder.WithName("vi-source-pvc"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StoragePersistentVolumeClaim),
			vibuilder.WithDataSourceHTTP(object.ImageTestDataQCOW, nil, nil),
		)
		baseVI.Spec.PersistentVolumeClaim.StorageClass = scPtr

		viObs := viobs.StartObserver(ctx, f, baseVI)
		viObs.Never(viobs.BeFailed())

		By("Creating base VirtualImage on PVC", func() {
			err := f.CreateWithDeferredDeletion(ctx, baseVI)
			Expect(err).NotTo(HaveOccurred())

			err = viObs.WaitFor(viobs.BeReady(), framework.LongTimeout)
			Expect(err).NotTo(HaveOccurred())
		})

		vd := vdbuilder.New(
			vdbuilder.WithName("vd-from-vi-pvc"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualImage, baseVI.Name),
			vdbuilder.WithStorageClass(scPtr),
		)

		createVirtualDiskAndWait(ctx, f, vd)
	})

	It("provisions a VirtualDisk from a ClusterVirtualImage", Label(precheck.NoPrecheck), func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-from-cvi"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindClusterVirtualImage, object.PrecreatedCVITestDataQCOW),
			vdbuilder.WithStorageClass(scPtr),
		)

		createVirtualDiskAndWait(ctx, f, vd)
	})

	It("provisions a blank VirtualDisk", Label(precheck.NoPrecheck), func() {
		vd := vdbuilder.New(
			vdbuilder.WithName("vd-blank"),
			vdbuilder.WithNamespace(f.Namespace().Name),
			vdbuilder.WithPersistentVolumeClaim(scPtr, ptr.To(resource.MustParse(vdCreationBlankSize))),
		)

		createVirtualDiskAndWait(ctx, f, vd)
	})

	Context("with snapshots", Label(precheck.PrecheckSnapshot), func() {
		It("provisions a VirtualDisk from a VirtualDiskSnapshot", func() {
			baseVD := vdbuilder.New(
				vdbuilder.WithName("vd-source-for-snapshot"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithDataSourceHTTP(&v1alpha2.DataSourceHTTP{URL: object.ImageTestDataQCOW}),
				vdbuilder.WithStorageClass(scPtr),
			)

			createVirtualDiskAndWait(ctx, f, baseVD)

			vdSnapshot := vdsnapshotbuilder.New(
				vdsnapshotbuilder.WithName("vd-snapshot"),
				vdsnapshotbuilder.WithNamespace(f.Namespace().Name),
				vdsnapshotbuilder.WithVirtualDiskName(baseVD.Name),
				vdsnapshotbuilder.WithRequiredConsistency(true),
			)

			By("Creating VirtualDiskSnapshot", func() {
				err := f.CreateWithDeferredDeletion(ctx, vdSnapshot)
				Expect(err).NotTo(HaveOccurred())

				util.UntilObjectPhase(ctx, string(v1alpha2.VirtualDiskSnapshotPhaseReady), framework.LongTimeout, vdSnapshot)
			})

			vd := vdbuilder.New(
				vdbuilder.WithName("vd-from-snapshot"),
				vdbuilder.WithNamespace(f.Namespace().Name),
				vdbuilder.WithDataSourceObjectRef(v1alpha2.VirtualDiskObjectRefKindVirtualDiskSnapshot, vdSnapshot.Name),
				vdbuilder.WithStorageClass(scPtr),
			)

			createVirtualDiskAndWait(ctx, f, vd)
		})
	})
})

func createVirtualDiskAndWait(ctx context.Context, f *framework.Framework, vd *v1alpha2.VirtualDisk) {
	GinkgoHelper()

	obs := vdobs.StartObserver(ctx, f, vd)
	obs.Never(vdobs.BeFailed())
	obs.Always(vdobs.BeStorageClassReady())
	obs.Always(vdobs.BeDataSourceReady())
	obs.Always(vdobs.HaveNonDecreasingProgress())

	By("Creating VirtualDisk", func() {
		err := f.CreateWithDeferredDeletion(ctx, vd)
		Expect(err).NotTo(HaveOccurred())
	})

	err := obs.WaitFor(vdobs.BeReady(), framework.LongTimeout)
	Expect(err).NotTo(HaveOccurred())
}

func doRetriableUploadAttempt(url, filePath string) error {
	const maxAttempts = 12
	const retryDelay = 5 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := doVirtualDiskUploadAttempt(url, filePath)
		if err == nil {
			return nil
		}
		if !isRetriableUploadError(err) {
			return err
		}

		lastErr = err
		time.Sleep(retryDelay)
	}

	return fmt.Errorf("upload failed after %d attempts: %w", maxAttempts, lastErr)
}

func doVirtualDiskUploadAttempt(url, filePath string) error {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			Expect(closeErr).NotTo(HaveOccurred(), "Failed to close file %s", filePath)
		}
	}()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", filePath, err)
	}
	if stat.Size() == 0 {
		return fmt.Errorf("file %s is empty", filePath)
	}

	req, err := http.NewRequest(http.MethodPut, url, file)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.ContentLength = stat.Size()

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			Expect(closeErr).NotTo(HaveOccurred(), "Failed to close response body")
		}
	}()

	return handleUploadResponse(resp)
}

func isRetriableUploadError(err error) bool {
	message := err.Error()
	return !strings.Contains(message, "upload failed with status ") ||
		strings.Contains(message, "upload failed with status 5")
}

// downloadImageToTempFile downloads url into a temporary file and returns its path.
// The caller is responsible for removing the file when finished.
func downloadImageToTempFile(url string) (string, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %q: %w", url, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			Expect(closeErr).NotTo(HaveOccurred(), "failed to close response body")
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download %q: unexpected status %d", url, resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", filepath.Base(url)+"-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	closed := false
	defer func() {
		if closed {
			return
		}
		if closeErr := tmpFile.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			Expect(closeErr).NotTo(HaveOccurred(), "failed to close temp file")
		}
	}()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return "", fmt.Errorf("copy to temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}
	closed = true

	return tmpFile.Name(), nil
}

// uploaderIngressNginxNamespaceLabel is the namespace label used to match the
// Deckhouse ingress-nginx controller namespace.
const uploaderIngressNginxNamespaceLabel = "module"

// uploaderIngressNginxNamespaceLabelValue is the value of the namespace label
// for the Deckhouse ingress-nginx controller namespace (d8-ingress-nginx).
const uploaderIngressNginxNamespaceLabelValue = "ingress-nginx"

// allowIngressNginxToUploaderNetworkPolicy patches the NetworkPolicy created by
// the virtualization-controller for the uploader pod owned by vd, so that
// traffic from the Deckhouse ingress-nginx controller namespace
// (d8-ingress-nginx) is allowed to reach the uploader pod.
//
// Without this patch external uploads via the Ingress URL fail with a 504
// Gateway Time-out because the NetworkPolicy currently only allows ingress
// from namespaces with the label "module=virtualization", while the ingress
// controller pod lives in "d8-ingress-nginx" (label "module=ingress-nginx").
func allowIngressNginxToUploaderNetworkPolicy(ctx context.Context, f *framework.Framework, namespace string, ownerUID types.UID) error {
	var policies netv1.NetworkPolicyList
	if err := f.Clients.GenericClient().List(ctx, &policies, crclient.InNamespace(namespace)); err != nil {
		return fmt.Errorf("list network policies in %q: %w", namespace, err)
	}

	peer := netv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				uploaderIngressNginxNamespaceLabel: uploaderIngressNginxNamespaceLabelValue,
			},
		},
	}

	var patched int
	for i := range policies.Items {
		np := &policies.Items[i]
		if !isOwnedByUID(np.OwnerReferences, ownerUID) {
			continue
		}
		if hasNamespaceSelectorPeer(np.Spec.Ingress, peer.NamespaceSelector.MatchLabels) {
			patched++
			continue
		}

		if len(np.Spec.Ingress) == 0 {
			np.Spec.Ingress = []netv1.NetworkPolicyIngressRule{{}}
		}
		np.Spec.Ingress[0].From = append(np.Spec.Ingress[0].From, peer)

		if err := f.Clients.GenericClient().Update(ctx, np); err != nil {
			return fmt.Errorf("update network policy %q: %w", np.Name, err)
		}
		patched++
	}

	if patched == 0 {
		return fmt.Errorf("no NetworkPolicy owned by UID %q found in %q", ownerUID, namespace)
	}
	return nil
}

func isOwnedByUID(refs []metav1.OwnerReference, uid types.UID) bool {
	for _, ref := range refs {
		if ref.UID == uid {
			return true
		}
	}
	return false
}

func hasNamespaceSelectorPeer(rules []netv1.NetworkPolicyIngressRule, labels map[string]string) bool {
	for _, rule := range rules {
		for _, from := range rule.From {
			if from.NamespaceSelector == nil {
				continue
			}
			if equalLabels(from.NamespaceSelector.MatchLabels, labels) {
				return true
			}
		}
	}
	return false
}

func equalLabels(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
