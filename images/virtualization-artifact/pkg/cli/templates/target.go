/*
Copyright 2018 The KubeVirt Authors.
Copyright 2024 Flant JSC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/templates/target.go
*/

package templates

import (
	"errors"
	"fmt"
	"strings"
)

// ParseTarget argument supporting the form of name.namespace
func ParseTarget(arg string) (namespace, name string, err error) {
	if len(arg) == 0 {
		return "", "", errors.New("name is empty")
	}
	if arg[0] == '.' {
		return "", "", errors.New("expected name before '.'")
	}
	if arg[len(arg)-1] == '.' {
		return "", "", errors.New("expected namespace after '.'")
	}

	parts := strings.FieldsFunc(arg, func(r rune) bool {
		return r == '.'
	})

	name = parts[0]

	if len(parts) > 1 {
		namespace = parts[1]
	}

	return namespace, name, nil
}

// ParseSSHTarget argument supporting the form of username@name.namespace
func ParseSSHTarget(arg string) (namespace, name, username string, err error) {
	usernameAndTarget := strings.Split(arg, "@")
	if len(usernameAndTarget) > 1 {
		username = usernameAndTarget[0]
		if len(username) < 1 {
			return "", "", "", errors.New("expected username before '@'")
		}
		arg = usernameAndTarget[1]
	}

	if len(arg) < 1 {
		return "", "", "", errors.New("expected target after '@'")
	}

	namespace, name, err = ParseTarget(arg)
	return namespace, name, username, err
}

type LocalSCPArgument struct {
	Path string
}

type RemoteSCPArgument struct {
	Namespace string
	Name      string
	Username  string
	Path      string
}

func ParseSCPArguments(arg1 string, arg2 string) (local LocalSCPArgument, remote RemoteSCPArgument, toRemote bool, err error) {
	remoteArg := arg1
	localArg := arg2
	toRemote = false
	if strings.Contains(arg1, ":") && strings.Contains(arg2, ":") {
		err = fmt.Errorf("copying from a remote location to another remote location is not supported: %q to %q", arg1, arg2)
		return
	} else if !strings.Contains(arg1, ":") && !strings.Contains(arg2, ":") {
		err = fmt.Errorf("none of the two provided locations seems to be a remote location: %q to %q", arg1, arg2)
		return
	} else if strings.Contains(localArg, ":") {
		remoteArg = arg2
		localArg = arg1
		toRemote = true
	}

	split := strings.SplitN(remoteArg, ":", 2)
	remote.Namespace, remote.Name, remote.Username, err = ParseSSHTarget(split[0])
	if err != nil {
		return
	}
	remote.Path = split[1]
	local.Path = localArg
	return
}
