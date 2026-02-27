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
	"time"

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
	return f.sshCommandWithClient(f.d8virtualization, vmName, vmNamespace, command, options...)
}

// SSHCommandWithKubeConfig executes ssh command with an explicitly provided kubeconfig.
func (f *Framework) SSHCommandWithKubeConfig(vmName, vmNamespace, command, kubeConfig string, options ...SSHCommandOption) (string, error) {
	if kubeConfig == "" {
		return f.SSHCommand(vmName, vmNamespace, command, options...)
	}

	altD8, err := d8.NewD8Virtualization(d8.D8VirtualizationConf{
		KubeConfig: kubeConfig,
	})
	if err != nil {
		return "", fmt.Errorf("failed to initialize d8 client with fallback kubeconfig: %w", err)
	}

	return f.sshCommandWithClient(altD8, vmName, vmNamespace, command, options...)
}

func (f *Framework) sshCommandWithClient(d8client d8.D8Virtualization, vmName, vmNamespace, command string, options ...SSHCommandOption) (string, error) {
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

	res := d8client.SSHCommand(vmName, command, d8.SSHOptions{
		Namespace:    vmNamespace,
		Username:     o.user,
		IdentityFile: file.Name(),
		Timeout:      o.timeout,
	})

	if !res.WasSuccess() {
		return "", fmt.Errorf("failed to execute command %s: %w: %s", command, res.Error(), res.StdErr())
	}

	return res.StdOut(), nil
}

// D8SSHCommandWithKubeConfig executes ssh command against target host through d8 using provided kubeconfig.
func (f *Framework) D8SSHCommandWithKubeConfig(
	vmName, vmNamespace, command, kubeConfig, user, identityFile string,
	timeout time.Duration,
) (string, error) {
	altD8, err := d8.NewD8Virtualization(d8.D8VirtualizationConf{
		KubeConfig: kubeConfig,
	})
	if err != nil {
		return "", fmt.Errorf("failed to initialize d8 client with fallback kubeconfig: %w", err)
	}

	if timeout == 0 {
		timeout = ShortTimeout
	}

	res := altD8.SSHCommand(vmName, command, d8.SSHOptions{
		Namespace:    vmNamespace,
		Username:     user,
		IdentityFile: identityFile,
		Timeout:      timeout,
	})
	if !res.WasSuccess() {
		return "", fmt.Errorf("failed to execute command %s: %w: %s", command, res.Error(), res.StdErr())
	}

	return res.StdOut(), nil
}
