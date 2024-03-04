package e2e

import (
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	. "github.com/onsi/ginkgo/v2"
	"io/fs"
	"path/filepath"
	"strings"
)

var _ = Describe("Label and Annotation", Ordered, ContinueOnFailure, func() {
	imageManifest := vmPath("image.yaml")
	manifestVM := vmPath("vm_02_connectivity_service.yaml")

	BeforeAll(func() {
		By("Apply image for vms")
		ApplyFromFile(imageManifest)
		WaitFromFile(imageManifest, PhaseReady, LongWaitDuration)
	})
	AfterAll(func() {
		By("Delete all manifests")
		files := make([]string, 0)
		err := filepath.Walk(conf.VM.TestDataDir, func(path string, info fs.FileInfo, err error) error {
			if err == nil && strings.HasSuffix(info.Name(), "yaml") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil || len(files) == 0 {
			kubectl.Delete(imageManifest, kc.DeleteOptions{})
			kubectl.Delete(conf.VM.TestDataDir, kc.DeleteOptions{})
		} else {
			for _, f := range files {
				kubectl.Delete(f, kc.DeleteOptions{})
			}
		}
	})

	// test here
})
