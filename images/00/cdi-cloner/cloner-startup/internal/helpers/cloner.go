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

package helpers

import (
	"os"
	"os/exec"
	"strconv"
)

func RunCloner(contentType string, uploadBytes uint64, mountPoint string) error {
	cmd := exec.Command("/usr/bin/cdi-cloner",
		"-v=3",
		"-alsologtostderr",
		"-content-type="+contentType,
		"-upload-bytes="+strconv.FormatUint(uploadBytes, 10),
		"-mount="+mountPoint,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
