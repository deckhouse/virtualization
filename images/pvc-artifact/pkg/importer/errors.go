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

package importer

import (
	"fmt"

	"kubevirt.io/containerized-data-importer/pkg/common"
)

// ValidationSizeError is an error indication size validation failure.
type ValidationSizeError struct {
	err error
}

func (e ValidationSizeError) Error() string { return e.err.Error() }

// ErrRequiresScratchSpace indicates that we require scratch space.
var ErrRequiresScratchSpace = fmt.Errorf(common.ScratchSpaceRequired)

// ErrInvalidPath indicates that the path is invalid.
var ErrInvalidPath = fmt.Errorf("invalid transfer path")

// ImagePullFailedError indicates that the importer failed to pull an image; This error type wraps the actual error.
type ImagePullFailedError struct {
	err error
}

// NewImagePullFailedError creates new ImagePullFailedError error object, with embedded error.
//
// Use the err parameter fot the actual wrapped error
func NewImagePullFailedError(err error) *ImagePullFailedError {
	return &ImagePullFailedError{
		err: err,
	}
}

func (err *ImagePullFailedError) Error() string {
	return fmt.Sprintf("%s: %s", common.ImagePullFailureText, err.err.Error())
}

func (err *ImagePullFailedError) Unwrap() error {
	return err.err
}
