package git

import (
	"github.com/deckhouse/virtualization/tests/e2e/executor"
)

const (
	Cmd = "git"
)

type Git interface {
	GetHeadHash() *executor.CMDResult
}

type GitCMD struct {
	executor.Executor
	cmd string
}

func NewGit() (*GitCMD, error) {
	cmd := Cmd
	e := executor.NewExecutor([]string{})
	return &GitCMD{
		Executor: e,
		cmd:      cmd,
	}, nil
}

func (g GitCMD) GetHeadHash() *executor.CMDResult {
	cmd := "git rev-parse --short HEAD"
	return g.Exec(cmd)
}
