package kubectl

import (
	"context"
	"fmt"
	"github.com/deckhouse/virtualization/tests/e2e/executor"
	"os"
	"time"
)

const (
	Cmd                        = "kubectl"
	ApplyTimeout               = 10 * time.Second
	CreateTimeout              = 10 * time.Second
	GetTimeout                 = 60 * time.Second
	WaitTimeout                = 30 * time.Second
	ResourceNode      Resource = "node"
	ResourceNamespace Resource = "namespace"
	ResourcePod       Resource = "pod"
)

type Resource string

type Kubectl interface {
	Apply(filepath string, opts KubectlOptions) *executor.CMDResult
	Create(filepath string, opts KubectlOptions) *executor.CMDResult
	CreateResource(resource Resource, name string, opts KubectlOptions) *executor.CMDResult
	Get(filepath string, opts KubectlOptions) *executor.CMDResult
	GetResource(resource Resource, name string, opts KubectlOptions) *executor.CMDResult
	Delete(filepath string, opts KubectlOptions) *executor.CMDResult
	DeleteResource(resource Resource, name string, opts KubectlOptions) *executor.CMDResult
	List(resource Resource, opts KubectlOptions) *executor.CMDResult
	Wait(filepath string, opts KubectlOptions) *executor.CMDResult
	WaitResource(resource Resource, name string, opts KubectlOptions) *executor.CMDResult
	RawCommand(subCmd string, timeout time.Duration) *executor.CMDResult
}

type KubectlOptions struct {
	Namespace   string
	Output      string
	Force       bool
	WaitFor     string
	WaitTimeout time.Duration
}

func NewKubectl() (*KubectlCMD, error) {
	if kubeConfig := os.Getenv("KUBECONFIG"); kubeConfig != "" {
		e := executor.NewExecutor([]string{"KUBECONFIG=" + kubeConfig})
		return &KubectlCMD{
			Executor: e,
			cmd:      Cmd,
		}, nil
	}
	token := os.Getenv("TOKEN")
	endpoint := os.Getenv("ENDPOINT")
	if token == "" || endpoint == "" {
		return nil, fmt.Errorf("not found creds for connect to cluster")
	}
	cmd := fmt.Sprintf("%s --token=%s --server=%s", Cmd, token, endpoint)
	if ca := os.Getenv("CA_CRT"); ca != "" {
		cmd = fmt.Sprintf("%s --certificate-authority=%s", cmd, ca)
	}
	if insecureTLS := os.Getenv("INSECURE_TLS"); insecureTLS != "" {
		cmd = fmt.Sprintf("%s --insecure-skip-tls-verify=%s", cmd, insecureTLS)
	}
	e := executor.NewExecutor([]string{})
	return &KubectlCMD{
		Executor: e,
		cmd:      cmd,
	}, nil
}

type KubectlCMD struct {
	executor.Executor
	cmd string
}

func (k KubectlCMD) addOptions(cmd string, opts KubectlOptions) string {
	if opts.Namespace != "" {
		cmd = fmt.Sprintf("%s -n %s", cmd, opts.Namespace)
	}
	if opts.Output != "" {
		cmd = fmt.Sprintf("%s -o %s", cmd, opts.Output)
	}
	return cmd
}

func (k KubectlCMD) Apply(filepath string, opts KubectlOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s apply -f %s --force=%t", k.cmd, filepath, opts.Force)
	cmd = k.addOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), ApplyTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) Create(filepath string, opts KubectlOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s create -f %s", k.cmd, filepath)
	cmd = k.addOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), CreateTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) CreateResource(resource Resource, name string, opts KubectlOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s create %s %s", k.cmd, resource, name)
	cmd = k.addOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), CreateTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) Get(filepath string, opts KubectlOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s get -f %s", k.cmd, filepath)
	cmd = k.addOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), GetTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) GetResource(resource Resource, name string, opts KubectlOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s get %s %s", k.cmd, resource, name)
	cmd = k.addOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), GetTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) Delete(filepath string, opts KubectlOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s delete -f %s", k.cmd, filepath)
	cmd = k.addOptions(cmd, opts)
	return k.Exec(cmd)
}

func (k KubectlCMD) DeleteResource(resource Resource, name string, opts KubectlOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s delete %s %s", k.cmd, resource, name)
	cmd = k.addOptions(cmd, opts)
	return k.Exec(cmd)
}

func (k KubectlCMD) List(resource Resource, opts KubectlOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s get %s", k.cmd, resource)
	cmd = k.addOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), GetTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) Wait(filepath string, opts KubectlOptions) *executor.CMDResult {
	forFlag := ""
	if opts.WaitFor != "" {
		forFlag = "--for=" + opts.WaitFor
	}
	timeoutFlag := ""
	timeout := WaitTimeout
	if opts.WaitTimeout != 0 {
		timeoutFlag = "--timeout=" + opts.WaitTimeout.String()
		timeout = opts.WaitTimeout
	}
	cmd := fmt.Sprintf("%s wait -f %s %s %s", k.cmd, filepath, forFlag, timeoutFlag)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) WaitResource(resource Resource, name string, opts KubectlOptions) *executor.CMDResult {
	forFlag := ""
	if opts.WaitFor != "" {
		forFlag = "--for=" + opts.WaitFor
	}
	timeoutFlag := ""
	timeout := WaitTimeout
	if opts.WaitTimeout != 0 {
		timeoutFlag = "--timeout=" + opts.WaitTimeout.String()
		timeout = opts.WaitTimeout
	}
	cmd := fmt.Sprintf("%s wait  %s %s %s %s", k.cmd, resource, name, forFlag, timeoutFlag)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) RawCommand(subCmd string, timeout time.Duration) *executor.CMDResult {
	cmd := fmt.Sprintf("%s %s", k.cmd, subCmd)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}
