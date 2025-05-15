/*
Copyright 2018 The KubeVirt Authors
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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/scp/native.go
*/

package scp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/povsister/scp"

	"github.com/deckhouse/virtualization/src/cli/internal/cmd/ssh"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

func (o *SCP) nativeSCP(local templates.LocalSCPArgument, remote templates.RemoteSCPArgument, toRemote bool) error {
	sshClient := ssh.NativeSSHConnection{
		ClientConfig: o.clientConfig,
		Options:      o.options,
	}
	client, err := sshClient.PrepareSSHClient(remote.Namespace, remote.Name)
	if err != nil {
		return err
	}

	scpClient, err := scp.NewClientFromExistingSSH(client, &scp.ClientOption{})
	if err != nil {
		return err
	}

	if toRemote {
		return o.copyToRemote(scpClient, local.Path, remote.Path)
	}
	return o.copyFromRemote(scpClient, local.Path, remote.Path)
}

func (o *SCP) copyToRemote(client *scp.Client, localPath, remotePath string) error {
	isFile, isDir, exists, err := stat(localPath)
	if err != nil {
		return fmt.Errorf("failed reading path %q: %v", localPath, err)
	}

	if !exists {
		return fmt.Errorf("local path %q does not exist, can't copy it", localPath)
	}

	if o.recursive {
		if isFile {
			return fmt.Errorf("local path %q is not a directory but '--recursive' was provided", localPath)
		}

		return client.CopyDirToRemote(localPath, remotePath, &scp.DirTransferOption{PreserveProp: o.preserve})
	}

	if isDir {
		return fmt.Errorf("local path %q is a directory but '--recursive' was not provided", localPath)
	}

	return client.CopyFileToRemote(localPath, remotePath, &scp.FileTransferOption{PreserveProp: o.preserve})
}

func (o *SCP) copyFromRemote(client *scp.Client, localPath, remotePath string) error {
	_, isDir, exists, err := stat(localPath)
	if err != nil {
		return fmt.Errorf("failed reading path %q: %v", localPath, err)
	}

	if o.recursive {
		if exists {
			if !isDir {
				return fmt.Errorf("local path %q is a file but '--recursive' was provided", localPath)
			}
			localPath = appendRemoteBase(localPath, remotePath)
		}

		if err := os.MkdirAll(localPath, os.ModePerm); err != nil {
			return fmt.Errorf("failed ensuring the existence of the local target directory %q: %v", localPath, err)
		}

		return client.CopyDirFromRemote(remotePath, localPath, &scp.DirTransferOption{PreserveProp: o.preserve})
	}

	if exists && isDir {
		localPath = appendRemoteBase(localPath, remotePath)
	}

	return client.CopyFileFromRemote(remotePath, localPath, &scp.FileTransferOption{PreserveProp: o.preserve})
}

func stat(path string) (isFile, isDir, exists bool, err error) {
	s, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, false, nil
	} else if err != nil {
		return false, false, false, err
	}
	return !s.IsDir(), s.IsDir(), true, nil
}

func appendRemoteBase(localPath, remotePath string) string {
	remoteBase := filepath.Base(remotePath)
	switch remoteBase {
	case "..", ".", "/", "./", "":
		// no identifiable base name, let's go with the supplied local path
		return localPath
	default:
		// we identified a base location, let's append it to the local path
		return filepath.Join(localPath, remoteBase)
	}
}
