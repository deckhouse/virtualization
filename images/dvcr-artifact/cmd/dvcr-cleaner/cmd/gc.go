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

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/cleaner/kubernetes"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/cleaner/registry"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/cleaner/signal"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/cleaner/storage"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var GcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Garbage collector",
	Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
}

var gcRunCmd = &cobra.Command{
	Use:   "run",
	Short: "`garbage-collect` deletes cache data and layers not referenced by any manifests",
	Args:  cobra.OnlyValidArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		confirm, err := Confirm()
		if err != nil {
			return fmt.Errorf("confirm is failed: %w", err)
		}
		if !confirm {
			return nil
		}

		vdCachePath := fmt.Sprintf("%s/vd", RepoDir)
		err = os.RemoveAll(vdCachePath)
		if err != nil {
			return fmt.Errorf("cache data cannot be deleted: %w", err)
		}

		stdout, err := registry.ExecGarbageCollect()
		if err != nil {
			fmt.Println(err.Error())
			return nil
		}

		fmt.Println(string(stdout))
		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

var (
	MaintenanceSecretName string
)

var autoCleanupCmd = &cobra.Command{
	Use:           "auto-cleanup [--maintenance-secret-name secret]",
	Short:         "`auto-cleanup` deletes all stale images that have no corresponding resource in the cluster and then runs garbage-collect to remove underlying blobs (Note: not for manual run unless you 100% sure what are you doing)",
	Args:          cobra.OnlyValidArgs,
	RunE:          autoCleanupHandler,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var checkCmd = &cobra.Command{
	Use:           "check",
	Short:         "`check` reports stale images that have no corresponding resource in the cluster",
	Args:          cobra.OnlyValidArgs,
	RunE:          checkCleanupHandler,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	GcCmd.AddCommand(gcRunCmd)

	// Add 'run' command.
	GcCmd.AddCommand(autoCleanupCmd)
	autoCleanupCmd.Flags().StringVar(&MaintenanceSecretName, "maintenance-secret-name", "", "update secret with result and annotation after the cleanup")
	// Add 'check' command.
	GcCmd.AddCommand(checkCmd)
}

func Confirm() (bool, error) {
	prompt := promptui.Prompt{
		Label:     "Confirm",
		IsConfirm: true,
	}

	_, err := prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrAbort) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func autoCleanupHandler(cmd *cobra.Command, args []string) error {
	fsInfoBeforeCleanup, err := registry.StorageStats()
	if err != nil {
		return fmt.Errorf("get repositories filesystem info before cleanup: %w", err)
	}

	var errs *multierror.Error

	cleanupErr := performAutoCleanup()
	if cleanupErr != nil {
		errs = multierror.Append(errs, cleanupErr)
	}

	// Report disk usage.
	fsInfoAfterCleanup, errFSInfo := registry.StorageStats()
	if errFSInfo != nil {
		errs = multierror.Append(errs, fmt.Errorf("get repositories filesystem info after cleanup: %w", errFSInfo))
	}
	freedSpace := ""
	availableSpace := ""
	usedSpace := ""
	totalSpace := ""
	if errFSInfo == nil {
		// Available space after cleanup should be greater than available space before cleanup.
		// The difference is the freed space. Format it with GiB/MiB suffix.
		freedSpaceRaw := fsInfoAfterCleanup.Available - fsInfoBeforeCleanup.Available
		freedSpace = storage.HumanizeQuantity(freedSpaceRaw) + "B"
		availableSpace = storage.HumanizeQuantity(fsInfoAfterCleanup.Available) + "B"
		usedSpace = storage.HumanizeQuantity(fsInfoAfterCleanup.Total-fsInfoAfterCleanup.Available) + "B"
		totalSpace = storage.HumanizeQuantity(fsInfoAfterCleanup.Total) + "B"
	}
	fmt.Printf("Freed space during cleanup: %s, available space now: %s\n", freedSpace, availableSpace)
	fmt.Printf("%7s  %7s  %7s\n", "Total", "Used", "Avail")
	fmt.Printf("%7s  %7s  %7s\n", totalSpace, usedSpace, availableSpace)

	// Terminate without waiting if no secret name was provided.
	if MaintenanceSecretName == "" {
		return errs.ErrorOrNil()
	}

	// Update maintenance secret and wait for termination signal.
	result := map[string]string{
		"result":         "success",
		"freedSpace":     freedSpace,
		"availableSpace": availableSpace,
	}
	if cleanupErr != nil {
		result["result"] = "fail"
		result["error"] = cleanupErr.Error()
	}

	secretErr := annotateMaintenanceSecretOnCleanupDone(context.Background(), result)
	if secretErr != nil {
		errs = multierror.Append(errs, secretErr)
	}

	// Return previous errors, so Pod will be restarted without waiting.
	err = errs.ErrorOrNil()
	if err != nil {
		return err
	}

	// Wait until termination.
	fmt.Println("Wait for signal before terminate.")
	signal.WaitForTermination()
	return nil
}

func performAutoCleanup() error {
	absentImages, err := getAbsentImages()
	if err != nil {
		return err
	}

	// Delete manifests for absent images.
	if len(absentImages) == 0 {
		fmt.Println("No images eligible for cleanup.")
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

func checkCleanupHandler(_ *cobra.Command, _ []string) error {
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

	// Get all VirtualDisks, VirtualImages, and ClusterVirtualImages
	virtClient, err := kubernetes.NewVirtualizationClient()
	if err != nil {
		return nil, fmt.Errorf("initialize Kubernetes client: %w", err)
	}

	kubeImages, err := virtClient.ListAllPossibleImages(context.Background())
	if err != nil {
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

const (
	garbageCollectionDoneAnno = "virtualization.deckhouse.io/dvcr-garbage-collection-done"
	switchToMaintenanceAnno   = "virtualization.deckhouse.io/dvcr-deployment-switch-to-maintenance-mode"
)

func annotateMaintenanceSecretOnCleanupDone(ctx context.Context, result map[string]string) error {
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result to json: %w", err)
	}

	// Get all VirtualImages and ClusterImages
	virtClient, err := kubernetes.NewVirtualizationClient()
	if err != nil {
		return fmt.Errorf("initialize Kubernetes client: %w", err)
	}

	secret, err := virtClient.GetMaintenanceSecret(ctx)
	if err != nil {
		return err
	}

	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations[garbageCollectionDoneAnno] = ""
	delete(secret.Annotations, switchToMaintenanceAnno)

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data["result"] = resultBytes

	err = virtClient.UpdateMaintenanceSecret(ctx, secret)
	if err != nil {
		return fmt.Errorf("update secret on cleanup done: %w", err)
	}

	return nil
}
