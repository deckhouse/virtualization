/*
Copyright 2018 The CDI Authors.
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

package image

import (
	"fmt"
	"net/url"
	"os"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"

	"kubevirt.io/containerized-data-importer/pkg/common"
)

func convertTo(format, src, dest string, preallocate bool) error {
	switch format {
	case "qcow2", "raw":
		// Do nothing.
	default:
		return errors.Errorf("unknown format: %s", format)
	}
	args := []string{"convert", "-t", "writeback", "-p", "-O", format, src, dest}
	var err error

	if preallocate {
		err = addPreallocation(args, convertPreallocationMethods, func(args []string) ([]byte, error) {
			return qemuExecFunction(nil, reportProgress, "qemu-img", args...)
		})
	} else {
		klog.V(1).Infof("Running qemu-img with args: %v", args)
		_, err = qemuExecFunction(nil, reportProgress, "qemu-img", args...)
	}
	if err != nil {
		_ = os.Remove(dest)
		errorMsg := fmt.Sprintf("could not convert image to %s", format)
		if nbdkitLog, err := os.ReadFile(common.NbdkitLogPath); err == nil {
			errorMsg += " " + string(nbdkitLog)
		}
		return errors.Wrap(err, errorMsg)
	}

	return nil
}

func (o *qemuOperations) ConvertToFormatStream(url *url.URL, format, dest string, preallocate bool) error {
	if len(url.Scheme) > 0 && url.Scheme != "nbd+unix" {
		return fmt.Errorf("not valid schema %s", url.Scheme)
	}
	return convertTo(format, url.String(), dest, preallocate)
}
