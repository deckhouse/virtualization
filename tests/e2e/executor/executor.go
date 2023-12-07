package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

const DefaultTimeout = 300 * time.Second

type Executor interface {
	Exec(cmd string) *CMDResult
	ExecContext(ctx context.Context, cmd string) *CMDResult
	ExecWithSudo(cmd string) *CMDResult
	ExecWithSudoContext(ctx context.Context, cmd string) *CMDResult
	ExecuteContext(ctx context.Context, cmd string, stdout io.Writer, stderr io.Writer) error
}

func (e CMDExecutor) Exec(command string) *CMDResult {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(DefaultTimeout))
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

func (e CMDExecutor) ExecuteContext(ctx context.Context, command string, stdout io.Writer, stderr io.Writer) error {
	cmd := e.makeCMD(ctx, command, stdout, stderr)
	return cmd.Run()
}
func (e CMDExecutor) makeCMD(ctx context.Context, command string, stdout io.Writer, stderr io.Writer) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = e.env
	return cmd
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

func (r CMDResult) StdErr() string {
	return r.stdOut.String()
}

func (r CMDResult) WasSuccess() bool {
	return r.success
}

func (r CMDResult) Error() error {
	return r.err
}
