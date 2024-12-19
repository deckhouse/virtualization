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
	"strings"

	"github.com/spf13/cobra"
)

var (
	NamespaceFlag     string
	AllImagesFlag     bool
	AllNamespacesFlag bool
	ImageRepoDir      = "/var/lib/registry/docker/registry/v2/repositories"
)

var LsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List of virtual images",
	Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
}

var lsViCmd = &cobra.Command{
	Use:   "vi [imageName]",
	Short: "Get virtual image or list of images",
	Args: cobra.MatchAll(
		cobra.MaximumNArgs(1),
		cobra.OnlyValidArgs,
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && !AllImagesFlag && !AllNamespacesFlag {
			return cmd.Help()
		}

		if len(args) != 0 && AllImagesFlag {
			return errors.New("flag `all` cannot be used with `imageName`")
		}

		if len(args) != 0 {
			image := args[0]
			info, err := os.Stat(fmt.Sprintf("%s/vi/%s/%s", ImageRepoDir, NamespaceFlag, image))
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("virtual image %q not found in %q namespace", image, NamespaceFlag)
			}
			if err != nil {
				return fmt.Errorf("cannot get virtual image %q in %q namespace: %w", image, NamespaceFlag, err)
			}
			fmt.Println(info.Name())
			return nil
		}

		if AllImagesFlag {
			images, err := ListVirtualImages(NamespaceFlag)
			if err != nil {
				return err
			}
			fmt.Println(strings.Join(images, "\n"))
			return nil
		}

		if AllNamespacesFlag {
			namespacesPath := fmt.Sprintf("%s/vi", ImageRepoDir)
			namespaces, err := os.ReadDir(namespacesPath)
			if err != nil {
				return fmt.Errorf("cannot get list of all images in all namespaces: %w", err)
			}
			for _, ns := range namespaces {
				images, err := ListVirtualImages(ns.Name())
				if err != nil {
					return err
				}
				fmt.Println(strings.Join(images, "\n"))
			}
			return nil
		}

		return cmd.Help()
	},
}

var lsCviCmd = &cobra.Command{
	Use:   "cvi [imageName]",
	Short: "Get cluster virtual image or list of images",
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

		if len(args) != 0 {
			image := args[0]
			info, err := os.Stat(fmt.Sprintf("%s/cvi/%s", ImageRepoDir, image))
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("cluster virtual image %q not found", image)
			}
			if err != nil {
				return fmt.Errorf("cannot get cluster virtual image %q: %w", image, err)
			}
			fmt.Println(info.Name())
			return nil
		}

		if AllImagesFlag {
			cviPath := fmt.Sprintf("%s/cvi", ImageRepoDir)
			files, err := os.ReadDir(cviPath)
			if err != nil {
				return fmt.Errorf("cannot get list of all images: %w", err)
			}
			images := make([]string, 0, len(files))
			for _, f := range files {
				images = append(images, f.Name())
			}
			fmt.Println(strings.Join(images, "\n"))
			return nil
		}

		return cmd.Help()
	},
}

func ListVirtualImages(namespace string) ([]string, error) {
	nsPath := fmt.Sprintf("%s/vi/%s", ImageRepoDir, namespace)
	files, err := os.ReadDir(nsPath)
	if err != nil {
		return nil, fmt.Errorf("cannot get list of all images in %q namespace: %w", namespace, err)
	}
	images := make([]string, 0, len(files))
	for _, f := range files {
		images = append(images, fmt.Sprintf("%s/%s", namespace, f.Name()))
	}
	return images, nil
}

func init() {
	LsCmd.AddCommand(lsCviCmd, lsViCmd)
	lsViCmd.Flags().StringVarP(&NamespaceFlag, "namespace", "n", "default", "namespace of virtual images")
	lsViCmd.Flags().BoolVar(&AllImagesFlag, "all", false, "list of all virtual images")
	lsViCmd.Flags().BoolVar(&AllNamespacesFlag, "all-namespaces", false, "list of all virtual images in all namespaces")
	lsViCmd.MarkFlagsMutuallyExclusive("all", "all-namespaces")
	lsCviCmd.Flags().BoolVar(&AllImagesFlag, "all", false, "list of all cluster virtual images")
}
