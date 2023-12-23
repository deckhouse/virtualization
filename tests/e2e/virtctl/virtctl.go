package virtctl

import (
	"context"
	"fmt"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
	"os"
	"time"
)

const (
	Cmd           = "virtctl"
	ShortTimeout  = 10 * time.Second
	MediumTimeout = 30 * time.Second
	LongTimeout   = 60 * time.Second
)

type Virtctl interface {
	StopVm(name, namespace string) *executor.CMDResult
	StartVm(name, namespace string) *executor.CMDResult
	SshCommand(vmName, command string, opts SshOptions) *executor.CMDResult
}

type VirtctlCMD struct {
	executor.Executor
	cmd string
}

type VirtctlConf struct {
	KubeConfig           string
	Token                string
	Endpoint             string
	CertificateAuthority string
	InsecureTls          bool
}

func NewVirtctl(conf VirtctlConf) (*VirtctlCMD, error) {
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
		return &VirtctlCMD{
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
	return &VirtctlCMD{
		Executor: e,
		cmd:      cmd,
	}, nil
}

func (v VirtctlCMD) StopVm(name, namespace string) *executor.CMDResult {
	cmd := v.addNamespace(fmt.Sprintf("%s stop %s", v.cmd, name), namespace)
	ctx, cancel := context.WithTimeout(context.Background(), ShortTimeout)
	defer cancel()
	return v.ExecContext(ctx, cmd)
}

func (v VirtctlCMD) StartVm(name, namespace string) *executor.CMDResult {
	cmd := v.addNamespace(fmt.Sprintf("%s start %s", v.cmd, name), namespace)
	ctx, cancel := context.WithTimeout(context.Background(), ShortTimeout)
	defer cancel()
	return v.ExecContext(ctx, cmd)
}

type SshOptions struct {
	Namespace   string
	Username    string
	IdenityFile string
	Port        int
	Timeout     time.Duration
}

func (v VirtctlCMD) SshCommand(vmName, command string, opts SshOptions) *executor.CMDResult {
	timeout := LongTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout
	}
	localSshOpts := "--local-ssh-opts='-o StrictHostKeyChecking=no' --local-ssh-opts='-o UserKnownHostsFile=/dev/null' --local-ssh-opts='-o LogLevel=ERROR'"
	localSshOpts = fmt.Sprintf("%s --local-ssh-opts='-o ConnectTimeout=%s'", localSshOpts, timeout.String())
	cmd := fmt.Sprintf("%s ssh %s --command '%s' --local-ssh %s",
		v.cmd, vmName, command, localSshOpts)
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

func (v VirtctlCMD) addNamespace(cmd, ns string) string {
	if ns != "" {
		return fmt.Sprintf("%s -n %s", cmd, ns)
	}
	return cmd
}
