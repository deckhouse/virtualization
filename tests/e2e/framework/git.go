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

package framework

import (
	"errors"
	"fmt"
	"os"

	storagev1 "k8s.io/api/storage/v1"
)

const (
	ceph                = "rbd.csi.ceph.com"
	sdsReplicatedVolume = "replicated.csi.storage.deckhouse.io"
)

func (f *Framework) GetNamePrefix(storageClass *storagev1.StorageClass) (string, error) {
	if prNumber, ok := os.LookupEnv("MODULES_MODULE_TAG"); ok && prNumber != "" {
		return prNumber, nil
	}

	res := f.git.GetHeadHash()
	if !res.WasSuccess() {
		return "", errors.New(res.StdErr())
	}

	commitHash := res.StdOut()
	commitHash = commitHash[:len(commitHash)-1]

	namePrefix := fmt.Sprintf("v12n-%s", commitHash)

	var scPrefix string
	if storageClass != nil {
		switch storageClass.Provisioner {
		case ceph:
			scPrefix = "ceph"
		case sdsReplicatedVolume:
			scPrefix = "sds-rep-vol"
		default:
			scPrefix = "ukn-strg"
		}

		namePrefix = fmt.Sprintf("%s-%s", namePrefix, scPrefix)
	}

	return namePrefix, nil
}
