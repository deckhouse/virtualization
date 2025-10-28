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
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var (
	NamespaceFlag     string
	YesFlag           bool
	AllImagesFlag     bool
	AllNamespacesFlag bool
	RepoDir           = "/var/lib/registry/docker/registry/v2/repositories"
)

var LsCmd = &cobra.Command{
	Use:   "ls",
	Short: "A list of `VirtualImages` or `ClusterVirtualImages`",
	Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
}

var lsViCmd = &cobra.Command{
	Use:   "vi [imageName]",
	Short: "Get a `VirtualImage` or a list of `VirtualImages`",
	Args: cobra.MatchAll(
		cobra.MaximumNArgs(1),
		cobra.OnlyValidArgs,
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		imgsDir := fmt.Sprintf("%s/vi", RepoDir)
		err := ListImage(v1alpha2.VirtualImageKind, imgsDir, cmd, args)
		if err != nil {
			return err
		}

		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

var lsCviCmd = &cobra.Command{
	Use:   "cvi [imageName]",
	Short: "Get a `ClusterVirtualImage` or a list of `ClusterVirtualImages`",
	Args: cobra.MatchAll(
		cobra.MaximumNArgs(1),
		cobra.OnlyValidArgs,
	),
	RunE: func(cmd *cobra.Command, args []string) error {
		// return ListImage(v1alpha2.ClusterVirtualImageKind, cmd, args)
		imgsDir := fmt.Sprintf("%s/cvi", RepoDir)
		err := ListImage(v1alpha2.ClusterVirtualImageKind, imgsDir, cmd, args)
		if err != nil {
			return err
		}

		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func ListImage(imgType, imgsDir string, cmd *cobra.Command, args []string) error {
	if len(args) != 0 && (AllImagesFlag || AllNamespacesFlag) {
		return errors.New("flags `all|all-namespaces` cannot be used with `imageName`")
	}

	if len(args) != 0 {

		var (
			fileInfo os.FileInfo
			err      error
		)
		imgName := args[0]
		switch imgType {
		case v1alpha2.VirtualImageKind:
			path := fmt.Sprintf("%s/%s/%s", imgsDir, NamespaceFlag, imgName)
			fileInfo, err = os.Stat(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("the `%s` %q is not found in %q namespace", imgType, imgName, NamespaceFlag)
				}
				return fmt.Errorf("cannot get the `%s` %q in the %q namespace: %w", imgType, imgName, NamespaceFlag, err)
			}
		case v1alpha2.ClusterVirtualImageKind:
			path := fmt.Sprintf("%s/%s", imgsDir, imgName)
			fileInfo, err = os.Stat(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("the `%s` %q is not found", imgType, imgName)
				}
				return fmt.Errorf("cannot get the `%s` %q: %w", imgType, imgName, err)
			}
		default:
			return fmt.Errorf("unknown image type: %s", imgType)
		}

		w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
		fmt.Fprintln(w, "Name\t")
		fmt.Fprintf(w, "%s\t\n", fileInfo.Name())
		w.Flush()
		return nil
	}

	if AllImagesFlag {
		switch imgType {
		case v1alpha2.VirtualImageKind:
			imgs, err := listAllVirtualImages(imgsDir, NamespaceFlag)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
			fmt.Fprintln(w, "Name\t")
			for _, img := range imgs {
				fmt.Fprintf(w, "%s\t\n", img.Name)
			}
			w.Flush()
		case v1alpha2.ClusterVirtualImageKind:
			imgs, err := os.ReadDir(imgsDir)
			if err != nil {
				return fmt.Errorf("cannot get the list of all `ClusterVirtualImages`: %w", err)
			}
			w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
			fmt.Fprintln(w, "Name\t")
			for _, img := range imgs {
				fmt.Fprintf(w, "%s\t\n", img.Name())
			}
			w.Flush()
		default:
			return fmt.Errorf("unknown image type: %s", imgType)
		}
		return nil
	}

	if imgType == v1alpha2.VirtualImageKind && AllNamespacesFlag {
		namespaces, err := os.ReadDir(imgsDir)
		if err != nil {
			return fmt.Errorf("cannot get the list of all namespaces: %w", err)
		}
		w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
		fmt.Fprintln(w, "Namespace\tName\t")
		for _, ns := range namespaces {
			imgs, err := listAllVirtualImages(imgsDir, ns.Name())
			if err != nil {
				return err
			}
			for _, img := range imgs {
				fmt.Fprintf(w, "%s\t%s\t\n", img.Namespace, img.Name)
			}
		}
		w.Flush()
		return nil
	}

	return cmd.Help()
}

type VirtualImage struct {
	Name      string
	Namespace string
}

func listAllVirtualImages(imgsDir, namespace string) ([]VirtualImage, error) {
	nsPath := fmt.Sprintf("%s/%s", imgsDir, namespace)
	imgs, err := os.ReadDir(nsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("the namespace %q is not found", namespace)
		}
		return nil, fmt.Errorf("cannot get the list of all `VirtualImages` in %q namespace: %w", namespace, err)
	}
	imgNames := make([]VirtualImage, len(imgs))
	for i, img := range imgs {
		imgNames[i] = VirtualImage{Name: img.Name(), Namespace: namespace}
	}
	return imgNames, nil
}

func init() {
	LsCmd.AddCommand(lsCviCmd, lsViCmd)
	lsViCmd.Flags().StringVarP(&NamespaceFlag, "namespace", "n", "default", "a namespace of VirtualImages")
	lsViCmd.Flags().BoolVar(&AllImagesFlag, "all", false, "a list of all VirtualImages")
	lsViCmd.Flags().BoolVar(&AllNamespacesFlag, "all-namespaces", false, "a list of all VirtualImages in all namespaces")
	lsViCmd.MarkFlagsMutuallyExclusive("all", "all-namespaces")
	lsViCmd.MarkFlagsMutuallyExclusive("namespace", "all-namespaces")
	lsCviCmd.Flags().BoolVar(&AllImagesFlag, "all", false, "a list of all ClusterVirtualImages")
}
