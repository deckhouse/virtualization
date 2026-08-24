/*
Copyright 2026 Flant JSC

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

package main

import (
	"errors"
	"fmt"
	"os"
)

// runCheck validates the ```changes blocks of the merge request the pipeline
// runs for, so that a block reaches the changelog generator only after somebody
// could still fix it.
//
// A description with no block at all is fine: not every merge request needs a
// changelog entry.
func runCheck(sections Sections) error {
	if source := os.Getenv("CI_PIPELINE_SOURCE"); source != "merge_request_event" {
		logf("not a merge request pipeline (CI_PIPELINE_SOURCE=%s), skipping", source)
		return nil
	}

	apiURL, err := requireEnv("CI_API_V4_URL")
	if err != nil {
		return err
	}
	projectID, err := requireEnv("CI_PROJECT_ID")
	if err != nil {
		return err
	}
	mrIID, err := requireEnv("CI_MERGE_REQUEST_IID")
	if err != nil {
		return err
	}
	token, err := requireEnv("GITLAB_API_TOKEN")
	if err != nil {
		return err
	}

	description, err := NewGitLab(apiURL, token, projectID).MergeRequestDescription(mrIID)
	if err != nil {
		return err
	}

	blocks := ParseDescription(description)
	if len(blocks) == 0 {
		logf("no ```changes blocks in the description of !%s — nothing to validate", mrIID)
		return nil
	}
	logf("validating %d ```changes block(s) in the description of !%s", len(blocks), mrIID)

	var problems []string
	for _, block := range blocks {
		problems = append(problems, blockProblems(block, sections)...)
	}
	if len(problems) == 0 {
		logf("all %d ```changes block(s) are valid", len(blocks))
		return nil
	}
	for _, problem := range problems {
		logf("ERROR: %s", problem)
	}
	return fmt.Errorf("%d problem(s) in the ```changes blocks of !%s", len(problems), mrIID)
}

func blockProblems(block ParsedBlock, sections Sections) []string {
	if block.Err != nil {
		return []string{fmt.Sprintf("block #%d: %v", block.Index, block.Err)}
	}
	var problems []string
	for index, entry := range block.Entries {
		prefix := fmt.Sprintf("block #%d", block.Index)
		if len(block.Entries) > 1 {
			prefix = fmt.Sprintf("%s entry #%d", prefix, index+1)
		}
		for _, problem := range entry.Validate(sections) {
			problems = append(problems, fmt.Sprintf("%s: %s", prefix, problem))
		}
	}
	return problems
}

func requireEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", errors.New("required environment variable " + name + " is not set")
	}
	return value, nil
}
