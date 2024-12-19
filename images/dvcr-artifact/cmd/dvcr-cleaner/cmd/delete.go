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

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var DeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete virtual images",
	Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
}

var deleteViCmd = &cobra.Command{
	Use:   "vi",
	Short: "Delete virtual image or all virtual images in namespace",
	Args: cobra.MatchAll(
		cobra.MaximumNArgs(1),
		cobra.OnlyValidArgs,
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && !AllImagesFlag {
			return cmd.Help()
		}

		if len(args) != 0 && AllImagesFlag {
			return errors.New("flag `all` cannot be used with `imageName`")
		}

		err := Confirm()
		if errors.Is(err, promptui.ErrAbort) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("confirm is failed: %w", err)
		}

		if len(args) != 0 {
			image := args[0]
			filePath := fmt.Sprintf("%s/vi/%s/%s", ImageRepoDir, NamespaceFlag, image)
			err := os.RemoveAll(filePath)
			if err != nil {
				return fmt.Errorf("cannot delete virtual image %q in %q namespace: %w", image, NamespaceFlag, err)
			}
			fmt.Println("Successful")
			return nil
		}

		if AllImagesFlag {
			viPath := fmt.Sprintf("%s/vi/%s", ImageRepoDir, NamespaceFlag)
			err := os.RemoveAll(viPath)
			if err != nil {
				return fmt.Errorf("cannot delete all images in %q namespace: %w", NamespaceFlag, err)
			}
			fmt.Println("Successful")
			return nil
		}

		return cmd.Help()
	},
}

var deleteCviCmd = &cobra.Command{
	Use:   "cvi",
	Short: "Delete clusterVirtual image",
	Args: cobra.MatchAll(
		cobra.MaximumNArgs(1),
		cobra.OnlyValidArgs,
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && !AllImagesFlag {
			return cmd.Help()
		}

		if len(args) != 0 && AllImagesFlag {
			return errors.New("flag `all` cannot be used with `imageName`")
		}

		err := Confirm()
		if errors.Is(err, promptui.ErrAbort) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("confirm is failed: %w", err)
		}

		if len(args) != 0 {
			image := args[0]
			filePath := fmt.Sprintf("%s/cvi/%s", ImageRepoDir, image)
			err := os.RemoveAll(filePath)
			if err != nil {
				return fmt.Errorf("cannot delete cluster virtual image %q: %w", image, err)
			}
			fmt.Println("Successful")
			return nil
		}

		if AllImagesFlag {
			cviPath := fmt.Sprintf("%s/cvi", ImageRepoDir)
			err := os.RemoveAll(cviPath)
			if err != nil {
				return fmt.Errorf("cannot delete all images: %w", err)
			}
			fmt.Println("Successful")
			return nil
		}

		return cmd.Help()
	},
}

func init() {
	DeleteCmd.AddCommand(deleteCviCmd, deleteViCmd)
	deleteViCmd.Flags().StringVarP(&NamespaceFlag, "namespace", "n", "default", "namespace of virtual images")
	deleteViCmd.Flags().BoolVar(&AllImagesFlag, "all", false, "delete all virtual images")
}
