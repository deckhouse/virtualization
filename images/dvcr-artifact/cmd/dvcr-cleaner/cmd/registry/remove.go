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
	"fmt"
	"os"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func RemoveImages(images []Image) error {
	if len(images) == 0 {
		return nil
	}

	for _, image := range images {
		err := RemoveImage(image)
		if err != nil {
			return err
		}
	}

	return nil
}

func RemoveImage(image Image) error {
	if _, err := os.Stat(image.Path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("image directory %s for `%s` %q is not found", image.Path, image.Type, image.Name)
		}
	}

	err := os.RemoveAll(image.Path)
	if err != nil {
		switch image.Type {
		case v1alpha2.VirtualImageKind, v1alpha2.VirtualDiskKind:
			return fmt.Errorf("delete image directory %s for `%s` %q in %q namespace: %w", image.Path, image.Type, image.Name, image.Namespace, err)
		case v1alpha2.ClusterVirtualImageKind:
			return fmt.Errorf("delete image directory %s for `%s` %q: %w", image.Path, image.Type, image.Name, err)
		default:
			return fmt.Errorf("unknown image type: %s", image.Type)
		}
	}

	return nil
}
