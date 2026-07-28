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

package uploader

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WaitForUserUploadTimeout limits how long the uploader waits for the user to start the upload
// before the import process is considered failed.
const WaitForUserUploadTimeout = 10 * time.Minute

// WaitForUserUploadRequeueAfter is the interval at which the controller re-reconciles
// while the resource is sitting in the WaitForUserUpload phase. With a 10-minute TTL,
// 30 seconds keeps the timeout accurate to within half a minute.
const WaitForUserUploadRequeueAfter = 30 * time.Second

// WaitForUserUploadTimeoutMessage returns the user-facing explanation for a TTL
// timeout. Pass the kind of the failed resource (e.g. "VirtualDisk") so the
// message tells the user exactly what to recreate.
func WaitForUserUploadTimeoutMessage(resourceKind string) string {
	return fmt.Sprintf(
		"The image was not uploaded within %d minutes. "+
			"Recreate the %s to upload the image again.",
		int(WaitForUserUploadTimeout/time.Minute), resourceKind,
	)
}

// IsWaitForUserUploadTimeoutExpired reports whether more than WaitForUserUploadTimeout has elapsed
// since waitStartedAt (typically the Ready condition's LastTransitionTime while it holds the
// WaitForUserUpload reason).
func IsWaitForUserUploadTimeoutExpired(waitStartedAt metav1.Time) bool {
	return !waitStartedAt.IsZero() && time.Since(waitStartedAt.Time) > WaitForUserUploadTimeout
}
