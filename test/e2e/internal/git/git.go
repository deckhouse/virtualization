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

package git

import (
	"github.com/deckhouse/virtualization/test/e2e/internal/executor"
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
