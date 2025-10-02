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
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var DeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete `VirtualImages` or `ClusterVirtualImages`",
	Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
}

var deleteViCmd = &cobra.Command{
	Use:   "vi",
	Short: "Delete a `VirtualImage` or all `VirtualImages` in namespace",
	Args: cobra.MatchAll(
		cobra.MaximumNArgs(1),
		cobra.OnlyValidArgs,
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		imgsDir := fmt.Sprintf("%s/vi/%s", RepoDir, NamespaceFlag)
		err := DeleteImage(v1alpha2.VirtualImageKind, imgsDir, cmd, args)
		if err != nil {
			return err
		}

		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

var deleteCviCmd = &cobra.Command{
	Use:   "cvi",
	Short: "Delete a `ClusterVirtualImage` or all `ClusterVirtualImages`",
	Args: cobra.MatchAll(
		cobra.MaximumNArgs(1),
		cobra.OnlyValidArgs,
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		imgsDir := fmt.Sprintf("%s/cvi", RepoDir)
		err := DeleteImage(v1alpha2.ClusterVirtualImageKind, imgsDir, cmd, args)
		if err != nil {
			return err
		}

		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func DeleteImage(imageType, imgsDir string, cmd *cobra.Command, args []string) error {
	if len(args) == 0 && !AllImagesFlag {
		return cmd.Help()
	}

	if len(args) != 0 && AllImagesFlag {
		return errors.New("flag `all` cannot be used with `imageName`")
	}

	if !YesFlag {
		confirm, err := Confirm()
		if err != nil {
			return fmt.Errorf("confirm is failed: %w", err)
		}
		if !confirm {
			return nil
		}
	}

	if len(args) != 0 {
		imgName := args[0]
		err := removeImageDir(imageType, imgsDir, imgName)
		if err != nil {
			return err
		}
		fmt.Println("Successful")
		return nil
	}

	if AllImagesFlag {
		imgs, err := os.ReadDir(imgsDir)
		if err != nil {
			return fmt.Errorf("cannot get the list of all images: %w", err)
		}
		for _, img := range imgs {
			err := removeImageDir(imageType, imgsDir, img.Name())
			if err != nil {
				return err
			}

		}
		fmt.Println("Successful")
		return nil
	}

	return cmd.Help()
}

func removeImageDir(imgType, imgsDir, imgName string) error {
	imgDir := fmt.Sprintf("%s/%s", imgsDir, imgName)
	if _, err := os.Stat(imgDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("the `%s` %q is not found", imgType, imgName)
		}
	}

	err := os.RemoveAll(imgDir)
	if err != nil {
		switch imgType {
		case v1alpha2.VirtualImageKind:
			return fmt.Errorf("cannot delete `%s` %q in %q namespace: %w", imgType, imgName, NamespaceFlag, err)
		case v1alpha2.ClusterVirtualImageKind:
			return fmt.Errorf("cannot delete `%s` %q: %w", imgType, imgName, err)
		default:
			return fmt.Errorf("unknown image type: %s", imgType)
		}
	}

	return nil
}

func init() {
	DeleteCmd.AddCommand(deleteCviCmd, deleteViCmd)
	deleteViCmd.Flags().StringVarP(&NamespaceFlag, "namespace", "n", "default", "a namespace of VirtualImages")
	deleteViCmd.Flags().BoolVar(&AllImagesFlag, "all", false, "delete all VirtualImages")
	deleteViCmd.Flags().BoolVarP(&YesFlag, "yes", "y", false, "Auto confirm delete VirtualImages")

	deleteCviCmd.Flags().BoolVar(&AllImagesFlag, "all", false, "delete all ClusterVirtualImages")
	deleteCviCmd.Flags().BoolVarP(&YesFlag, "yes", "y", false, "Auto confirm delete ClusterVirtualImages")
}
