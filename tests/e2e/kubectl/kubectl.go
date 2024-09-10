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

package kubectl

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/deckhouse/virtualization/tests/e2e/executor"
)

const (
	Cmd           = "kubectl"
	ShortTimeout  = 10 * time.Second
	MediumTimeout = 30 * time.Second
	LongTimeout   = 60 * time.Second
)

type Resource string

type Kubectl interface {
	Apply(filepath string, opts ApplyOptions) *executor.CMDResult
	Create(filepath string, opts CreateOptions) *executor.CMDResult
	CreateResource(resource Resource, name string, opts CreateOptions) *executor.CMDResult
	Get(filepath string, opts GetOptions) *executor.CMDResult
	GetResource(resource Resource, name string, opts GetOptions) *executor.CMDResult
	Delete(filepath string, opts DeleteOptions) *executor.CMDResult
	DeleteResource(resource Resource, name string, opts DeleteOptions) *executor.CMDResult
	Kustomize(directory string, opts KustomizeOptions) *executor.CMDResult
	List(resource Resource, opts GetOptions) *executor.CMDResult
	Wait(filepath string, opts WaitOptions) *executor.CMDResult
	WaitResources(resource Resource, opts WaitOptions, name ...string) *executor.CMDResult
	Patch(filepath string, opts PatchOptions) *executor.CMDResult
	PatchResource(resource Resource, name string, opts PatchOptions) *executor.CMDResult
	RawCommand(subCmd string, timeout time.Duration) *executor.CMDResult
}

type ApplyOptions struct {
	Namespace string
	Output    string
	Force     bool
}

type CreateOptions struct {
	Namespace string
	Output    string
}

type DeleteOptions struct {
	Label     string
	Namespace string
}

type GetOptions struct {
	Label          string
	Namespace      string
	Output         string
	IgnoreNotFound bool
}

type KustomizeOptions struct {
	Namespace string
	Output    string
	Force     bool
}

type WaitOptions struct {
	Namespace string
	For       string
	Timeout   time.Duration
}

type PatchOptions struct {
	Namespace  string
	Type       string
	PatchFile  string
	MergePatch string
	JsonPatch  *JsonPatch
}

type JsonPatch struct {
	Op    string
	Path  string
	Value string
}

func (p JsonPatch) String() string {
	var value string
	if _, err := strconv.Atoi(p.Value); err == nil ||
		strings.HasPrefix(p.Value, "[") ||
		strings.HasPrefix(p.Value, "{") {
		value = p.Value
	} else {
		value = fmt.Sprintf("\"%s\"", p.Value)
	}
	return fmt.Sprintf("[{\"op\": \"%s\", \"path\": \"%s\", \"value\":%s}]", p.Op, p.Path, value)
}

type KubectlConf struct {
	KubeConfig           string
	Token                string
	Endpoint             string
	CertificateAuthority string
	InsecureTls          bool
}

func NewKubectl(conf KubectlConf) (*KubectlCMD, error) {
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
		return &KubectlCMD{
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

func (k KubectlCMD) Apply(filepath string, opts ApplyOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s apply -f %s", k.cmd, filepath)
	cmd = k.applyOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), ShortTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) Create(filepath string, opts CreateOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s create -f %s", k.cmd, filepath)
	cmd = k.createOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), ShortTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) CreateResource(resource Resource, name string, opts CreateOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s create %s %s", k.cmd, resource, name)
	cmd = k.createOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), ShortTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) Get(filepath string, opts GetOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s get -f %s", k.cmd, filepath)
	cmd = k.getOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) GetResource(resource Resource, name string, opts GetOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s get %s %s", k.cmd, resource, name)
	cmd = k.getOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) Delete(filepath string, opts DeleteOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s delete -f %s", k.cmd, filepath)
	cmd = k.deleteOptions(cmd, opts)
	return k.Exec(cmd)
}

func (k KubectlCMD) DeleteResource(resource Resource, name string, opts DeleteOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s delete %s %s", k.cmd, resource, name)
	cmd = k.deleteOptions(cmd, opts)
	return k.Exec(cmd)
}

func (k KubectlCMD) Kustomize(directory string, opts KustomizeOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s apply --kustomize %s", k.cmd, directory)
	cmd = k.kustomizeOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), LongTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) List(resource Resource, opts GetOptions) *executor.CMDResult {
	cmd := fmt.Sprintf("%s get %s", k.cmd, resource)
	cmd = k.getOptions(cmd, opts)
	ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) Wait(filepath string, opts WaitOptions) *executor.CMDResult {
	cmd := k.waitOptions(fmt.Sprintf("%s wait -f %s", k.cmd, filepath), opts)
	timeout := MediumTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) WaitResource(resource Resource, name string, opts WaitOptions) *executor.CMDResult {
	cmd := k.waitOptions(fmt.Sprintf("%s wait %s %s", k.cmd, resource, name), opts)
	timeout := MediumTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) WaitResources(resource Resource, opts WaitOptions, names ...string) *executor.CMDResult {
	cmd := k.waitOptions(fmt.Sprintf("%s wait %s %v", k.cmd, resource, strings.Join(names, " ")), opts)
	timeout := MediumTimeout
	if opts.Timeout != 0 {
		timeout = opts.Timeout * time.Second
	}
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

func (k KubectlCMD) Patch(filepath string, opts PatchOptions) *executor.CMDResult {
	cmd := k.patchOptions(fmt.Sprintf("%s patch -f %s", k.cmd, filepath), opts)
	ctx, cancel := context.WithTimeout(context.Background(), ShortTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) PatchResource(resource Resource, name string, opts PatchOptions) *executor.CMDResult {
	cmd := k.patchOptions(fmt.Sprintf("%s patch %s %s", k.cmd, resource, name), opts)
	ctx, cancel := context.WithTimeout(context.Background(), ShortTimeout)
	defer cancel()
	return k.ExecContext(ctx, cmd)
}

func (k KubectlCMD) addNamespace(cmd, ns string) string {
	if ns != "" {
		return fmt.Sprintf("%s -n %s", cmd, ns)
	}
	return cmd
}

func (k KubectlCMD) addLabel(cmd, label string) string {
	if label != "" {
		return fmt.Sprintf("%s -l %s", cmd, label)
	}
	return cmd
}

func (k KubectlCMD) addOutput(cmd, output string) string {
	if output != "" {
		return fmt.Sprintf("%s -o %s", cmd, output)
	}
	return cmd
}

func (k KubectlCMD) addIgnoreNotFound(cmd string, ignoreNotFound bool) string {
	if ignoreNotFound {
		return fmt.Sprintf("%s --ignore-not-found", cmd)
	}
	return cmd
}

func (k KubectlCMD) applyOptions(cmd string, opts ApplyOptions) string {
	cmd = k.addNamespace(cmd, opts.Namespace)
	cmd = k.addOutput(cmd, opts.Output)
	return fmt.Sprintf("%s --force=%t", cmd, opts.Force)
}

func (k KubectlCMD) kustomizeOptions(cmd string, opts KustomizeOptions) string {
	cmd = k.addNamespace(cmd, opts.Namespace)
	cmd = k.addOutput(cmd, opts.Output)
	return fmt.Sprintf("%s --force=%t", cmd, opts.Force)
}

func (k KubectlCMD) createOptions(cmd string, opts CreateOptions) string {
	cmd = k.addNamespace(cmd, opts.Namespace)
	cmd = k.addOutput(cmd, opts.Output)
	return cmd
}

func (k KubectlCMD) getOptions(cmd string, opts GetOptions) string {
	cmd = k.addNamespace(cmd, opts.Namespace)
	cmd = k.addOutput(cmd, opts.Output)
	cmd = k.addIgnoreNotFound(cmd, opts.IgnoreNotFound)
	cmd = k.addLabel(cmd, opts.Label)
	return cmd
}

func (k KubectlCMD) deleteOptions(cmd string, opts DeleteOptions) string {
	cmd = k.addNamespace(cmd, opts.Namespace)
	cmd = k.addLabel(cmd, opts.Label)
	return cmd
}

func (k KubectlCMD) waitOptions(cmd string, opts WaitOptions) string {
	cmd = k.addNamespace(cmd, opts.Namespace)
	if opts.For != "" {
		cmd = fmt.Sprintf("%s --for=%s", cmd, opts.For)
	}
	if opts.Timeout != 0 {
		cmd = fmt.Sprintf("%s --timeout=%s", cmd, opts.Timeout*time.Second)
	}
	return cmd
}

func (k KubectlCMD) patchOptions(cmd string, opts PatchOptions) string {
	cmd = k.addNamespace(cmd, opts.Namespace)
	if opts.Type != "" {
		cmd = fmt.Sprintf("%s --type=%s", cmd, opts.Type)
	}
	if opts.PatchFile != "" {
		cmd = fmt.Sprintf("%s --patch-file=%s", cmd, opts.PatchFile)
	}
	if opts.JsonPatch != nil {
		cmd = fmt.Sprintf("%s --type=json --patch='%s'", cmd, opts.JsonPatch.String())
	}
	if opts.MergePatch != "" {
		cmd = fmt.Sprintf("%s --type=merge --patch='%s'", cmd, opts.MergePatch)
	}
	return cmd
}
