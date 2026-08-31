/*
Copyright 2018 The CDI Authors.
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
	"errors"
	"fmt"

	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/common"
	"github.com/deckhouse/virtualization/images/pvc-artifact/pkg/util"
)

// ValidationSizeError is an error indication size validation failure.
type ValidationSizeError struct {
	err error
}

func (e ValidationSizeError) Error() string {
	// The type is exported with an unexported field, so a zero value is easy to construct by
	// accident; a panic here would take down the very path that reports failures.
	if e.err == nil {
		return "image size validation failed"
	}
	return e.err.Error()
}

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

// WriteFailedError indicates that the importer read the image fine but could not store it:
// the write itself, the range zeroing or the page cache flush failed. Both sides of the copy
// end up in the same error chain, so they can only be told apart where they happen; guessing
// afterwards by the message text mislabels a storage failure as a failed download.
type WriteFailedError struct {
	err error
}

// NewWriteFailedError creates new WriteFailedError error object, with embedded error.
func NewWriteFailedError(err error) *WriteFailedError {
	return &WriteFailedError{
		err: err,
	}
}

func (err *WriteFailedError) Error() string {
	return fmt.Sprintf("%s: %s", common.WriteFailureText, err.err.Error())
}

func (err *WriteFailedError) Unwrap() error {
	return err.err
}

// IsPermanentFailure reports whether restarting the importer cannot change the outcome.
// The image not fitting the volume and a refused write stay refused on the next attempt,
// while a failed pull is usually the registry or the network blinking: the kubelet restarts
// the pod either way, and only the permanent ones should surface as a failed disk.
func IsPermanentFailure(err error) bool {
	var validationErr ValidationSizeError
	var writeErr *WriteFailedError
	return errors.As(err, &validationErr) || errors.As(err, &writeErr)
}

// failureMessage builds the termination message for a failed import. Every exit path of the
// importer must go through it: a failure that leaves no message behind reaches the user as an
// import that is forever "provisioning", with the reason available only in the log of the
// attempt that the restarts push out within seconds.
func failureMessage(err error) *common.TerminationMessage {
	msg := &common.TerminationMessage{
		ErrMessage: ptr.To(err.Error()),
	}
	if IsPermanentFailure(err) {
		msg.Permanent = ptr.To(true)
	}
	return msg
}

// WriteFailureMessage persists the reason of a failed import to the termination message, in the
// single format the controller reads. Both importers report through it, so a failure looks the
// same no matter which one produced it.
func WriteFailureMessage(failure error) error {
	msg, err := failureMessage(failure).String()
	if err != nil {
		return err
	}
	return util.WriteTerminationMessage(msg)
}
