/*
Copyright 2025 Flant JSC

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

package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/cmd/dvcr-cleaner/cmd/kubernetes"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/cmd/dvcr-cleaner/cmd/registry"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var (
	WriteTerminationMessage bool
)

var AutoCleanupCmd = &cobra.Command{
	Use:   "auto-cleanup",
	Short: "Remove manifests and blobs for absent `VirtualImages` and `ClusterVirtualImages` and run garbage collect",
	Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
}

var autoCleanupRunCmd = &cobra.Command{
	Use:           "run [--write-termination-message]",
	Short:         "",
	Args:          cobra.OnlyValidArgs,
	RunE:          autoCleanupRun,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var autoCleanupCheckCmd = &cobra.Command{
	Use:           "check",
	Short:         "",
	Args:          cobra.OnlyValidArgs,
	RunE:          autoCleanupCheck,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// Add 'run' command.
	AutoCleanupCmd.AddCommand(autoCleanupRunCmd)
	autoCleanupRunCmd.Flags().BoolVar(&WriteTerminationMessage, "write-termination-message", false, "write termination report to /dev/termination-log")
	// Add 'check' command.
	AutoCleanupCmd.AddCommand(autoCleanupCheckCmd)
}

func autoCleanupRun(cmd *cobra.Command, args []string) error {
	fsInfoBeforeCleanup, err := registry.StorageStats()
	if err != nil {
		return fmt.Errorf("get repositories filesystem info before cleanup: %w", err)
	}

	err = autoCleanup()

	fsInfoAfterCleanup, errFSInfo := registry.StorageStats()
	if errFSInfo != nil {
		err = fmt.Errorf("%w; %w", err, fmt.Errorf("get repositories filesystem info after cleanup: %w", errFSInfo))
	}

	freedSpace := ""
	availableSpace := ""
	if errFSInfo == nil {
		// Available space after cleanup should be greater than available space before cleanup.
		// The difference is the freed space. Format it with GiB/MiB suffix.
		freedSpaceRaw := fsInfoAfterCleanup.Available - fsInfoBeforeCleanup.Available
		freedSpace = resource.NewQuantity(int64(freedSpaceRaw), resource.BinarySI).String() + "B"
		availableSpace = resource.NewQuantity(int64(fsInfoAfterCleanup.Available), resource.BinarySI).String() + "B"
	}
	fmt.Printf("Freed space during cleanup: %s, available space now: %s\n", freedSpace, availableSpace)

	if WriteTerminationMessage {
		// Calculate freed bytes.
		extraFields := map[string]string{
			"freedSpace":     freedSpace,
			"availableSpace": availableSpace,
		}

		errTerminationMessage := writeTerminationMessage(err, extraFields)
		if errTerminationMessage != nil {
			err = fmt.Errorf("%w; %w", err, errTerminationMessage)
		}
	}

	return err
}

func autoCleanup() error {
	absentImages, err := getAbsentImages()
	if err != nil {
		return err
	}

	// Delete manifests for absent images.
	if len(absentImages) == 0 {
		fmt.Println("No images eligible for cleanup, exiting now.")
		return nil
	}

	err = registry.RemoveImages(absentImages)
	if err != nil {
		return fmt.Errorf("remove manifests: %w", err)
	}

	// Run 'registry garbage-collect' to remove blobs.
	stdout, err := registry.ExecGarbageCollect()
	if err != nil {
		return err
	}

	fmt.Println(string(stdout))
	return nil
}

func autoCleanupCheck(_ *cobra.Command, _ []string) error {
	fsInfo, err := registry.StorageStats()
	if err != nil {
		return fmt.Errorf("get repositories filesystem info before cleanup: %w", err)
	}

	absentImages, err := getAbsentImages()
	if err != nil {
		return err
	}

	availableSpace := resource.NewQuantity(int64(fsInfo.Available), resource.BinarySI).String() + "B"

	fmt.Printf("Available space: %s\n", availableSpace)

	if len(absentImages) == 0 {
		fmt.Println("No images eligible for auto-cleanup.")
	}

	sort.SliceStable(absentImages, func(i, j int) bool {
		return absentImages[i].Path < absentImages[j].Path
	})

	fmt.Println("Images eligible for cleanup:")
	for _, image := range absentImages {
		img := strings.TrimPrefix(image.Path, registry.RepoDir)
		img = strings.TrimPrefix(image.Path, "/")
		fmt.Println(img)
	}

	return nil
}

func getAbsentImages() ([]registry.Image, error) {
	// List all images created for all ClusterVirtualImage and VirtualImage resources.
	images, err := registry.ListImagesAll()
	if err != nil {
		return nil, fmt.Errorf("list all images: %w", err)
	}

	// Get all VirtualImages and ClusterImages
	virtClient, err := kubernetes.NewVirtualizationClient()
	if err != nil {
		return nil, fmt.Errorf("initialize Kubernetes client: %w", err)
	}

	kubeImages, err := virtClient.ListAllPossibleImages(context.Background())
	if err == nil {
		return nil, fmt.Errorf("list images in cluster: %w", err)
	}

	// Compare lists, return images absent in the cluster.
	return compareRegistryAndClusterImages(images, kubeImages), nil
}

// compareRegistryAndClusterImages returns images that has no corresponding resource in the cluster.
// VirtualDisks in Ready phase are considered for cleanup.
func compareRegistryAndClusterImages(images []registry.Image, kubeImages []kubernetes.ImageInfo) []registry.Image {
	// Create indexes for all resources found in cluster.
	// A map for ClusterImages. Keys are names.
	clusterVirtualImages := make(map[string]struct{})
	// A map for virtualImages: namespace -> name
	virtualImages := make(map[string]map[string]struct{})
	// A map for virtualDisks: namespace -> name -> disk phase
	virtualDisks := make(map[string]map[string]v1alpha2.DiskPhase)
	for _, kubeImage := range kubeImages {
		switch kubeImage.Type {
		case v1alpha2.ClusterVirtualImageKind:
			clusterVirtualImages[kubeImage.Name] = struct{}{}
		case v1alpha2.VirtualImageKind:
			if _, ok := virtualImages[kubeImage.Namespace]; !ok {
				virtualImages[kubeImage.Namespace] = make(map[string]struct{})
			}
			virtualImages[kubeImage.Namespace][kubeImage.Name] = struct{}{}
		case v1alpha2.VirtualDiskKind:
			if _, ok := virtualDisks[kubeImage.Namespace]; !ok {
				virtualDisks[kubeImage.Namespace] = make(map[string]v1alpha2.DiskPhase)
			}
			virtualDisks[kubeImage.Namespace][kubeImage.Name] = kubeImage.Phase
		}
	}

	absentImages := make([]registry.Image, 0)
	for _, image := range images {
		switch image.Type {
		case v1alpha2.ClusterVirtualImageKind:
			if _, ok := clusterVirtualImages[image.Name]; !ok {
				absentImages = append(absentImages, image)
			}
		case v1alpha2.VirtualImageKind:
			if _, ok := virtualImages[image.Namespace]; !ok {
				absentImages = append(absentImages, image)
				continue
			}
			if _, ok := virtualImages[image.Namespace][image.Name]; !ok {
				absentImages = append(absentImages, image)
			}
		case v1alpha2.VirtualDiskKind:
			if _, ok := virtualDisks[image.Namespace]; !ok {
				absentImages = append(absentImages, image)
				continue
			}
			if _, ok := virtualDisks[image.Namespace][image.Name]; !ok {
				absentImages = append(absentImages, image)
				continue
			}
			// Images for disks in Ready phase are eligible for cleanup.
			if virtualDisks[image.Namespace][image.Name] == v1alpha2.DiskReady {
				absentImages = append(absentImages, image)
			}
		}
	}

	return absentImages
}

func writeTerminationMessage(err error, extra map[string]string) error {
	report := map[string]string{
		"result": "success",
	}
	if err != nil {
		report["result"] = "fail"
	}
	return kubernetes.ReportTerminationMessage(err, report, extra)
}
