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

// changelog reads the ```changes blocks of merge request descriptions.
//
//	changelog -type check     validate the blocks of the merge request the
//	                          pipeline runs for
//	changelog -type collect   re-generate CHANGELOG/* for a milestone and open
//	                          the changelog merge request
//
// Both run types read a description through the same parser, so what the check
// accepts on a merge request is what the collection writes into the changelog.
// They used to be two Python scripts with a regular expression each, and the
// two expressions disagreed: the check ignored the example the merge request
// template ships, the collection took it for a real entry and lost the entry
// written under it.
//
// The environment is the one GitLab CI provides:
//
//	both:     GITLAB_API_TOKEN, CI_API_V4_URL, CI_PROJECT_ID
//	check:    CI_MERGE_REQUEST_IID, CI_PIPELINE_SOURCE
//	collect:  CI_PROJECT_PATH, CI_SERVER_HOST, CI_PROJECT_DIR, and optionally
//	          MILESTONE_TITLE, OPEN_CHANGELOG_MR, CHANGELOG_BASE_BRANCH
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var runType string
	flag.StringVar(&runType, "type", "", "Run type: check or collect.")
	var sectionsFile string
	flag.StringVar(&sectionsFile, "sections", defaultSectionsFile, "List of allowed sections.")
	flag.Parse()

	switch runType {
	case "check", "collect":
	default:
		fmt.Printf("Unknown run type '%s'\n", runType)
		flag.Usage()
		os.Exit(2)
	}

	sections, err := LoadSections(sectionsFile)
	if err == nil {
		if runType == "check" {
			err = runCheck(sections)
		} else {
			err = runCollect(sections)
		}
	}
	if err != nil {
		logf("changelog: %v", err)
		os.Exit(1)
	}
}

// logf writes to standard error, where the job log keeps it apart from anything
// a caller might read from standard output.
func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
