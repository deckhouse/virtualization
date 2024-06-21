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
	"time"

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
}

func NewD8Virtualization(conf D8VirtualizationConf) (*d8VirtualizationCMD, error) {
	envs := make([]string, 2)
	if home, found := os.LookupEnv("HOME"); found {
		envs[0] = "HOME=" + home
	} else {
		return nil, fmt.Errorf("env HOME not found")
	}
	if path, found := os.LookupEnv("PATH"); found {
		envs[1] = "PATH=" + path
	} else {
		return nil, fmt.Errorf("env PATH not found")
	}
	if conf.KubeConfig != "" {
		envs = append(envs, "KUBECONFIG="+conf.KubeConfig)
		e := executor.NewExecutor(envs)
		return &d8VirtualizationCMD{
			Executor: e,
			cmd:      Cmd,
		}, nil
	}
	if conf.Token == "" || conf.Endpoint == "" {
		return nil, fmt.Errorf("not found creds for connect to cluster")
	}
	cmd := fmt.Sprintf("%s --token=%s --server=%s", Cmd, conf.Token, conf.Endpoint)
	if conf.CertificateAuthority != "" {
		cmd = fmt.Sprintf("%s --certificate-authority=%s", cmd, conf.CertificateAuthority)
	}
	if conf.InsecureTls {
		cmd = fmt.Sprintf("%s --insecure-skip-tls-verify=%t", cmd, true)
	}
	e := executor.NewExecutor(envs)
	return &d8VirtualizationCMD{
		Executor: e,
		cmd:      cmd,
	}, nil
}

func (v d8VirtualizationCMD) SshCommand(vmName, command string, opts SshOptions) *executor.CMDResult {
	timeout := LongTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout
	}

	localSshOpts := "--local-ssh-opts='-o StrictHostKeyChecking=no' --local-ssh-opts='-o UserKnownHostsFile=/dev/null' --local-ssh-opts='-o LogLevel=ERROR'"
	localSshOpts = fmt.Sprintf("%s --local-ssh-opts='-o ConnectTimeout=%s'", localSshOpts, timeout.String())

	cmd := fmt.Sprintf("%s ssh %s -c '%s' --local-ssh=true %s", v.cmd, vmName, command, localSshOpts)
	cmd = v.addNamespace(cmd, opts.Namespace)

	if opts.Username != "" {
		cmd = fmt.Sprintf("%s --username=%s", cmd, opts.Username)
	}

	if opts.IdenityFile != "" {
		cmd = fmt.Sprintf("%s --identity-file=%s", cmd, opts.IdenityFile)
	}

	if opts.Port != 0 {
		cmd = fmt.Sprintf("%s --port=%d", cmd, opts.Port)
	}

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
