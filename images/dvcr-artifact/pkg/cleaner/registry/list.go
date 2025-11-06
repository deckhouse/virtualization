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

package registry

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	RepoDir = "/var/lib/registry/docker/registry/v2/repositories"
)

type Image struct {
	Type      string
	Namespace string
	Name      string
	Path      string
}

// clusterVirtualImagesDir returns a directory where stored all images for ClusterVirtualImage resources.
//
// Example:
//
//	 /.../repositories
//	 `-- cvi
//		    |-- ubuntu-24-04
//		    `-- alpine-latest
func clusterVirtualImagesDir() string {
	return filepath.Join(RepoDir, "cvi")
}

// virtualImagesDir returns a directory where stored all images for VirtualImage resources.
// These images are actually one level deep, there is a directory with the corresponding namespace name.
//
// Example:
//
//	  /.../repositories
//	  `-- vi
//		     |-- default
//		     |   `-- ubuntu-24-04
//		     |-- testvms
//		         |-- alpine-latest
//		         |-- win-server
//		         `-- ubuntu-latest
func virtualImagesDir() string {
	return filepath.Join(RepoDir, "vi")
}

// virtualImagesDir returns a directory where stored all temporary images for VirtualDisk resources.
// Directory structure is the same as for VirtualImage (see virtualImagesDir).
func virtualDisksDir() string {
	return filepath.Join(RepoDir, "vd")
}

// ListImagesAll return image info for all image types:
// virtualImages, clusterVirtualImages and virtualDisks.
func ListImagesAll() ([]Image, error) {
	clusterVirtualImages, err := ListClusterVirtualImages()
	if err != nil {
		return nil, err
	}

	virtualImages, err := ListVirtualImagesAll()
	if err != nil {
		return nil, err
	}

	virtualDiskImages, err := ListVirtualDisksAll()
	if err != nil {
		return nil, err
	}

	fmt.Printf("Found %d cvi, %d vi, %d vd manifests in registry\n",
		len(clusterVirtualImages),
		len(virtualImages),
		len(virtualDiskImages),
	)

	clusterVirtualImages = append(clusterVirtualImages, virtualImages...)
	return append(clusterVirtualImages, virtualDiskImages...), nil
}

func ListClusterVirtualImages() ([]Image, error) {
	imagesDir := clusterVirtualImagesDir()

	imagesEntries, err := os.ReadDir(imagesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot get the list of all `ClusterVirtualImages`: %w", err)
	}

	images := make([]Image, 0)
	for _, imageEntry := range imagesEntries {
		images = append(images, Image{
			Type:      v1alpha2.ClusterVirtualImageKind,
			Namespace: "",
			Name:      imageEntry.Name(),
			Path:      filepath.Join(imagesDir, imageEntry.Name()),
		})
	}
	return images, nil
}

func ListVirtualImagesAll() ([]Image, error) {
	imagesDir := virtualImagesDir()

	namespacesEntries, err := os.ReadDir(imagesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot list directories for namespaces in %s: %w", imagesDir, err)
	}

	images := make([]Image, 0)
	for _, nsEntry := range namespacesEntries {
		nsImages, err := ListVirtualImagesForNamespace(nsEntry.Name())
		if err != nil {
			return nil, err
		}
		images = append(images, nsImages...)
	}
	return images, nil
}

func ListVirtualImagesForNamespace(namespace string) ([]Image, error) {
	if namespace == "" {
		namespace = "default"
	}

	imagesDir := filepath.Join(virtualImagesDir(), namespace)
	imagesEntries, err := os.ReadDir(imagesDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot list directories with images for `VirtualImage` resources in %s: %w", imagesDir, err)
	}

	images := make([]Image, 0)
	for _, imageEntry := range imagesEntries {
		images = append(images, Image{
			Type:      v1alpha2.VirtualImageKind,
			Namespace: namespace,
			Name:      imageEntry.Name(),
			Path:      filepath.Join(imagesDir, imageEntry.Name()),
		})
	}
	return images, nil
}

func GetAnyImage(imageType, namespace, name string) (*Image, error) {
	var (
		imageDir string
		fileInfo os.FileInfo
		err      error
	)

	switch imageType {
	case v1alpha2.VirtualImageKind:
		if namespace == "" {
			namespace = "default"
		}
		imageDir = virtualImagesDir()
		path := filepath.Join(imageDir, namespace, name)
		fileInfo, err = os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("the `%s` %q is not found in %q namespace", imageType, name, namespace)
			}
			return nil, fmt.Errorf("cannot get the `%s` %q in the %q namespace: %w", imageType, name, namespace, err)
		}
	case v1alpha2.ClusterVirtualImageKind:
		imageDir = clusterVirtualImagesDir()
		path := filepath.Join(imageDir, name)

		fileInfo, err = os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("the `%s` %q is not found", imageType, name)
			}
			return nil, fmt.Errorf("cannot get the `%s` %q: %w", imageType, name, err)
		}
	default:
		return nil, fmt.Errorf("unknown image type: %s", imageType)
	}

	return &Image{
		Type:      imageType,
		Namespace: namespace,
		Name:      name,
		Path:      filepath.Join(imageDir, fileInfo.Name()),
	}, nil
}

func ListVirtualDisksAll() ([]Image, error) {
	disksDir := virtualDisksDir()

	namespacesEntries, err := os.ReadDir(disksDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot list directories for namespaces in %s: %w", disksDir, err)
	}

	disks := make([]Image, 0)
	for _, nsEntry := range namespacesEntries {
		disksInNamespace, err := ListVirtualDisksForNamespace(nsEntry.Name())
		if err != nil {
			return nil, err
		}
		disks = append(disks, disksInNamespace...)
	}
	return disks, nil
}

func ListVirtualDisksForNamespace(namespace string) ([]Image, error) {
	if namespace == "" {
		namespace = "default"
	}

	disksDir := filepath.Join(virtualDisksDir(), namespace)
	disksEntries, err := os.ReadDir(disksDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot list directories with images for `VirtualDisk` resources in %s: %w", disksDir, err)
	}

	disksImages := make([]Image, 0)
	for _, diskEntry := range disksEntries {
		disksImages = append(disksImages, Image{
			Type:      v1alpha2.VirtualDiskKind,
			Namespace: namespace,
			Name:      diskEntry.Name(),
			Path:      filepath.Join(disksDir, diskEntry.Name()),
		})
	}
	return disksImages, nil
}
