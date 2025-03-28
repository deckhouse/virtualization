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

package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const DefaultTimeout = 300 * time.Second

type Executor interface {
	Exec(cmd string) *CMDResult
	ExecContext(ctx context.Context, cmd string) *CMDResult
	ExecWithSudo(cmd string) *CMDResult
	ExecWithSudoContext(ctx context.Context, cmd string) *CMDResult
	ExecuteContext(ctx context.Context, cmd string, stdout, stderr io.Writer) error
	MakeCmd(ctx context.Context, command string) *exec.Cmd
}

func (e CMDExecutor) Exec(command string) *CMDResult {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	return e.ExecContext(ctx, command)
}

func (e CMDExecutor) ExecContext(ctx context.Context, command string) *CMDResult {
	stdout := new(Buffer)
	stderr := new(Buffer)
	err := e.ExecuteContext(ctx, command, stdout, stderr)
	cmdResult := &CMDResult{
		stdOut:  stdout,
		stdErr:  stderr,
		command: command,
		success: true,
	}
	if err != nil {
		cmdResult.success = false
		cmdResult.err = err
	}
	return cmdResult
}

func (e CMDExecutor) ExecWithSudo(cmd string) *CMDResult {
	return e.Exec(fmt.Sprintf("sudo %s", cmd))
}

func (e CMDExecutor) ExecWithSudoContext(ctx context.Context, command string) *CMDResult {
	return e.ExecContext(ctx, fmt.Sprintf("sudo %s", command))
}

func (e CMDExecutor) ExecuteContext(ctx context.Context, command string, stdout, stderr io.Writer) error {
	cmd := e.MakeCmd(ctx, command)
	cmd.Stdin = os.Stdin
	cmd.Stderr = stderr
	cmd.Stdout = stdout
	return cmd.Run()
}

func (e CMDExecutor) MakeCmd(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Env = mergeEnvs(cmd.Environ(), e.env)
	return cmd
}

func mergeEnvs(curr, override []string) []string {
	envMap := make(map[string]string)

	for _, currEnv := range curr {
		envName, envValue, _ := strings.Cut(currEnv, "=")
		envMap[envName] = envValue
	}
	for _, newEnv := range override {
		envName, envValue, _ := strings.Cut(newEnv, "=")
		envMap[envName] = envValue
	}

	res := make([]string, 0, len(envMap))
	for name, val := range envMap {
		res = append(res, fmt.Sprintf("%s=%s", name, val))
	}
	return res
}

type CMDExecutor struct {
	env []string
}

func NewExecutor(env []string) *CMDExecutor {
	return &CMDExecutor{
		env: env,
	}
}

type CMDResult struct {
	command string
	stdOut  *Buffer
	stdErr  *Buffer
	success bool
	err     error
}

func (r CMDResult) GetCmd() string {
	return r.command
}

func (r CMDResult) StdOut() string {
	return r.stdOut.String()
}

func (r CMDResult) StdOutBytes() []byte {
	return r.stdOut.Bytes()
}

func (r CMDResult) StdErr() string {
	return r.stdErr.String()
}

func (r CMDResult) WasSuccess() bool {
	return r.success
}

func (r CMDResult) Error() error {
	return r.err
}
