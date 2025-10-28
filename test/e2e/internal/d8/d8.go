/*
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
*/

package d8

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/deckhouse/virtualization/test/e2e/internal/config/clustertransport"
	"github.com/deckhouse/virtualization/test/e2e/internal/executor"
)

const (
	Cmd           = "d8 v"
	ShortTimeout  = 10 * time.Second
	MediumTimeout = 30 * time.Second
	LongTimeout   = 60 * time.Second
)

type D8VirtualizationCMD struct {
	executor.Executor
	cmd string
}

type SSHOptions struct {
	Namespace    string
	Username     string
	IdentityFile string
	Port         int
	Timeout      time.Duration
}

type D8VirtualizationConf struct {
	KubeConfig           string
	Token                string
	Endpoint             string
	CertificateAuthority string
	InsecureTLS          bool
}

type D8Virtualization interface {
	SSHCommand(vmName, command string, opts SSHOptions) *executor.CMDResult
	StopVM(vmName string, opts SSHOptions) *executor.CMDResult
	StartVM(vmName string, opts SSHOptions) *executor.CMDResult
	RestartVM(vmName string, opts SSHOptions) *executor.CMDResult
}

func NewD8Virtualization(conf D8VirtualizationConf) (*D8VirtualizationCMD, error) {
	if _, found := os.LookupEnv("HOME"); !found {
		return nil, fmt.Errorf("HOME environment variable shoule be set")
	}
	if _, found := os.LookupEnv("PATH"); !found {
		return nil, fmt.Errorf("PATH environment variable shoule be set")
	}

	connEnvs, connArgs, err := clustertransport.KubeConnectionCmdSettings(clustertransport.ClusterTransport(conf))
	if err != nil {
		return nil, fmt.Errorf("load connection config: %w", err)
	}

	e := executor.NewExecutor(connEnvs)
	return &D8VirtualizationCMD{
		Executor: e,
		cmd:      strings.Join(append([]string{Cmd}, connArgs...), " "),
	}, nil
}

func (v D8VirtualizationCMD) SSHCommand(vmName, command string, opts SSHOptions) *executor.CMDResult {
	timeout := ShortTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout
	}

	localSSHOpts := "--local-ssh-opts='-o StrictHostKeyChecking=no' --local-ssh-opts='-o UserKnownHostsFile=/dev/null' --local-ssh-opts='-o LogLevel=ERROR'"
	localSSHOpts = fmt.Sprintf("%s --local-ssh-opts='-o ConnectTimeout=%s'", localSSHOpts, timeout.String())

	cmd := fmt.Sprintf("%s ssh %s -c '%s' --local-ssh=true %s", v.cmd, vmName, command, localSSHOpts)
	cmd = v.addNamespace(cmd, opts.Namespace)

	if opts.Username != "" {
		cmd = fmt.Sprintf("%s --username=%s", cmd, opts.Username)
	}

	if opts.IdentityFile != "" {
		cmd = fmt.Sprintf("%s --identity-file=%s", cmd, opts.IdentityFile)
	}

	if opts.Port != 0 {
		cmd = fmt.Sprintf("%s --port=%d", cmd, opts.Port)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return v.ExecContext(ctx, cmd)
}

func (v D8VirtualizationCMD) StartVM(vmName string, opts SSHOptions) *executor.CMDResult {
	timeout := ShortTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout
	}

	cmd := fmt.Sprintf("%s start %s", v.cmd, vmName)
	cmd = v.addNamespace(cmd, opts.Namespace)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return v.ExecContext(ctx, cmd)
}

func (v D8VirtualizationCMD) StopVM(vmName string, opts SSHOptions) *executor.CMDResult {
	timeout := ShortTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout
	}

	cmd := fmt.Sprintf("%s stop %s", v.cmd, vmName)
	cmd = v.addNamespace(cmd, opts.Namespace)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return v.ExecContext(ctx, cmd)
}

func (v D8VirtualizationCMD) RestartVM(vmName string, opts SSHOptions) *executor.CMDResult {
	timeout := ShortTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout
	}

	cmd := fmt.Sprintf("%s restart %s", v.cmd, vmName)
	cmd = v.addNamespace(cmd, opts.Namespace)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return v.ExecContext(ctx, cmd)
}

func (v D8VirtualizationCMD) addNamespace(cmd, ns string) string {
	if ns != "" {
		return fmt.Sprintf("%s -n %s", cmd, ns)
	}
	return cmd
}
