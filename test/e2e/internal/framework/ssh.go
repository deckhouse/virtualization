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
	"fmt"
	"os"
	"regexp"
	"time"

	. "github.com/onsi/ginkgo/v2"

	"github.com/deckhouse/virtualization/test/e2e/internal/d8"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
)

type sshCommandOptions struct {
	user, privateKey string
	timeout          time.Duration
}

func WithSSHUser(user string) func(o *sshCommandOptions) {
	return func(o *sshCommandOptions) {
		o.user = user
	}
}

func WithSSHPrivateKey(privateKey string) func(o *sshCommandOptions) {
	return func(o *sshCommandOptions) {
		o.privateKey = privateKey
	}
}

func WithSSHTimeout(timeout time.Duration) func(o *sshCommandOptions) {
	return func(o *sshCommandOptions) {
		o.timeout = timeout
	}
}

type SSHCommandOption func(o *sshCommandOptions)

func makeSSHCommandOptions(options ...SSHCommandOption) *sshCommandOptions {
	o := &sshCommandOptions{
		user:       object.DefaultUser,
		privateKey: object.DefaultSSHPrivateKey,
		timeout:    ShortTimeout,
	}
	for _, option := range options {
		option(o)
	}
	return o
}

// SSHCommand returns the STDOUT of the command result and nil for the error if the command execution is successful.
// It returns an empty string and an error if the command execution fails.
func (f *Framework) SSHCommand(vmName, vmNamespace, command string, options ...SSHCommandOption) (string, error) {
	o := makeSSHCommandOptions(options...)

	file, err := os.CreateTemp(os.TempDir(), "ssh-key-")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}()

	if _, err = file.WriteString(o.privateKey); err != nil {
		return "", err
	}
	if err = os.Chmod(file.Name(), 0o600); err != nil {
		return "", err
	}

	res := f.d8virtualization.SSHCommand(vmName, command, d8.SSHOptions{
		Namespace:    vmNamespace,
		Username:     o.user,
		IdentityFile: file.Name(),
		Timeout:      o.timeout,
	})

	if !res.WasSuccess() {
		err := fmt.Errorf("failed to execute command %s: %s: %s", command, res.Error().Error(), res.StdErr())
		skipIfKnownKubeletTLSVerifyFailure(err)
		return "", err
	}

	return res.StdOut(), nil
}

// knownKubeletTLSVerifyRe matches the transient apiserver->kubelet TLS verification
// failure on the dev cluster ("error dialing backend: tls: failed to verify
// certificate: x509: certificate signed by unknown authority"): the kubelet serving
// certificate rotates and for a moment every exec/attach going through the apiserver
// is refused, killing whatever guest command a spec was running.
var knownKubeletTLSVerifyRe = regexp.MustCompile(`error dialing backend: tls: failed to verify certificate`)

// TODO: remove when the kubelet certificate rotation flake is fixed on the dev cluster.
func skipIfKnownKubeletTLSVerifyFailure(err error) {
	GinkgoHelper()

	if err == nil {
		return
	}
	if knownKubeletTLSVerifyRe.MatchString(err.Error()) {
		Skip("skip due to known transient kubelet TLS verification failure: " + err.Error())
	}
}
