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

	"github.com/deckhouse/virtualization/tests/e2e/config/clustertransport"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
)

const (
	Cmd           = "d8 v"
	ShortTimeout  = 10 * time.Second
	MediumTimeout = 30 * time.Second
	LongTimeout   = 60 * time.Second
)

type d8VirtualizationCMD struct {
	executor.Executor
	cmd string
}

type SshOptions struct {
	Namespace   string
	Username    string
	IdenityFile string
	Port        int
	Timeout     time.Duration
}

type D8VirtualizationConf struct {
	KubeConfig           string
	Token                string
	Endpoint             string
	CertificateAuthority string
	InsecureTls          bool
}

type D8Virtualization interface {
	SshCommand(vmName, command string, opts SshOptions) *executor.CMDResult
	StopVM(vmName string, opts SshOptions) *executor.CMDResult
	StartVM(vmName string, opts SshOptions) *executor.CMDResult
	RestartVM(vmName string, opts SshOptions) *executor.CMDResult
}

func NewD8Virtualization(conf D8VirtualizationConf) (*d8VirtualizationCMD, error) {
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
	return &d8VirtualizationCMD{
		Executor: e,
		cmd:      strings.Join(append([]string{Cmd}, connArgs...), " "),
	}, nil
}

func (v d8VirtualizationCMD) SshCommand(vmName, command string, opts SshOptions) *executor.CMDResult {
	timeout := ShortTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout
	}

	// Begin with command
	sshCmd := []string{
		fmt.Sprintf("%s ssh", v.cmd),
	}

	// Continue with more arguments.
	// Add VM name.
	sshCmd = append(sshCmd, vmName)

	// Add VM namespace.
	if opts.Namespace != "" {
		sshCmd = append(sshCmd, fmt.Sprintf("-n %s", opts.Namespace))
	}

	// Add command to run via SSH.
	sshCmd = append(sshCmd, fmt.Sprintf("-c '%s'", command))

	// Use local ssh.
	sshCmd = append(sshCmd, "--local-ssh=true")

	// Add more SSH options.
	sshOpts := []string{
		"-o StrictHostKeyChecking=no",
		"-o UserKnownHostsFile=/dev/null",
		"-o LogLevel=ERROR",
		// ConnectTimeout in seconds.
		fmt.Sprintf("-o ConnectTimeout=%.0f", timeout.Seconds()),
		// Set retries for more robust connection.
		"-o ConnectionAttempts=5",
	}
	for _, sshOpt := range sshOpts {
		sshCmd = append(sshCmd, fmt.Sprintf("--local-ssh-opts='%s'", sshOpt))
	}

	// Enable debug messages into stderr.
	sshCmd = append(sshCmd, "-vvv")

	if opts.Username != "" {
		sshCmd = append(sshCmd, fmt.Sprintf("--username=%s", opts.Username))
	}

	if opts.IdenityFile != "" {
		sshCmd = append(sshCmd, fmt.Sprintf("--identity-file=%s", opts.IdenityFile))
	}

	if opts.Port != 0 {
		sshCmd = append(sshCmd, fmt.Sprintf("--port=%d", opts.Port))
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return v.ExecContext(ctx, strings.Join(sshCmd, " "))
}

func (v d8VirtualizationCMD) StartVM(vmName string, opts SshOptions) *executor.CMDResult {
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

func (v d8VirtualizationCMD) StopVM(vmName string, opts SshOptions) *executor.CMDResult {
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

func (v d8VirtualizationCMD) RestartVM(vmName string, opts SshOptions) *executor.CMDResult {
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

func (v d8VirtualizationCMD) addNamespace(cmd, ns string) string {
	if ns != "" {
		return fmt.Sprintf("%s -n %s", cmd, ns)
	}
	return cmd
}
