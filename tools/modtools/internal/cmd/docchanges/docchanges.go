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

// Package docchanges implements the `doc-changes` command: it checks that a
// changed documentation or resource file also updates its related language file.
package docchanges

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization/tools/modtools/internal/diff"
	"github.com/deckhouse/virtualization/tools/modtools/internal/report"
)

func NewCommand(load diff.Loader) *cobra.Command {
	return &cobra.Command{
		Use:   "doc-changes",
		Short: "Check that documentation and resource changes update related language files",
		RunE: func(_ *cobra.Command, _ []string) error {
			info, err := load()
			if err != nil {
				return err
			}
			return report.Result(validate(info))
		},
	}
}

var (
	resourceFileRe = regexp.MustCompile(`openapi/config-values.y[a]?ml$|crds/.+.y[a]?ml$`)
	docFileRe      = regexp.MustCompile(`\.md$`)
	excludeFileRe  = regexp.MustCompile("crds/embedded/.+.y[a]?ml$")
)

func validate(info *diff.DiffInfo) (exitCode int) {
	fmt.Printf("Run 'doc changes' validation ...\n")

	if len(info.Files) == 0 {
		fmt.Printf("Nothing to validate, diff is empty\n")
		return 0
	}

	msgs := report.NewMessages()
	for _, fileInfo := range info.Files {
		if !fileInfo.HasContent() {
			continue
		}

		fileName := fileInfo.NewFileName

		switch {
		case strings.Contains(fileName, "testdata"):
			msgs.Add(report.NewSkip(fileName, ""))
		case docFileRe.MatchString(fileName):
			msgs.Add(checkDocFile(fileName, info))
		case resourceFileRe.MatchString(fileName) && !excludeFileRe.MatchString(fileName):
			msgs.Add(checkResourceFile(fileName, info))
		default:
			msgs.Add(report.NewSkip(fileName, ""))
		}
	}
	msgs.PrintReport()

	if msgs.CountErrors() > 0 {
		return 1
	}
	return 0
}

var (
	possibleDocRootsRe   = regexp.MustCompile(`docs/|docs/documentation`)
	docsDirAllowedFileRe = regexp.MustCompile(`docs/(CONFIGURATION|CR|FAQ|README|ADMIN_GUIDE|USER_GUIDE|CHARACTERISTICS_DESCRIPTION|INSTALL|RELEASE_NOTES)(\.ru)?.md`)
	docsDirFileRe        = regexp.MustCompile(`docs/[^/]+.md`)
)

func checkDocFile(fName string, diffInfo *diff.DiffInfo) report.Message {
	if !possibleDocRootsRe.MatchString(fName) {
		return report.NewSkip(fName, "")
	}

	if docsDirFileRe.MatchString(fName) && !docsDirAllowedFileRe.MatchString(fName) {
		return report.NewError(
			fName,
			"name is not allowed",
			`Rename this file or move it, for example, into 'internal' folder.
Only following file names are allowed in the module '/docs/' directory:
    CONFIGURATION.md
    CR.md
    FAQ.md
    README.md
    ADMIN_GUIDE.md
    USER_GUIDE.md
    CHARACTERISTICS_DESCRIPTION.md
    INSTALL.md
    RELEASE_NOTES.md
(also their Russian versions ended with '.ru.md')`,
		)
	}

	// Check if documentation for other language file is also modified.
	var otherFileName string
	if strings.HasSuffix(fName, `.ru.md`) {
		otherFileName = strings.TrimSuffix(fName, ".ru.md") + ".md"
	} else {
		otherFileName = strings.TrimSuffix(fName, ".md") + ".ru.md"
	}
	return checkRelatedFileExists(fName, otherFileName, diffInfo)
}

var (
	docRuResourceRe    = regexp.MustCompile(`doc-ru-.+.y[a]?ml$`)
	notDocRuResourceRe = regexp.MustCompile(`([^/]+\.y[a]?ml)$`)
)

// Check if resource for other language is also modified.
func checkResourceFile(fName string, diffInfo *diff.DiffInfo) report.Message {
	var otherFileName string
	if docRuResourceRe.MatchString(fName) {
		otherFileName = strings.Replace(fName, "doc-ru-", "", 1)
	} else {
		otherFileName = notDocRuResourceRe.ReplaceAllString(fName, `doc-ru-$1`)
	}
	return checkRelatedFileExists(fName, otherFileName, diffInfo)
}

func checkRelatedFileExists(origName, otherName string, diffInfo *diff.DiffInfo) report.Message {
	file, err := os.Open(otherName)
	if err != nil {
		return report.NewError(origName, "related is absent", fmt.Sprintf(`Documentation or resource file is changed
while related language file '%s' is absent.`, otherName))
	}
	defer func() { _ = file.Close() }()

	for _, fileInfo := range diffInfo.Files {
		if fileInfo.NewFileName == otherName {
			return report.NewOK(origName)
		}
	}
	return report.NewError(origName, "related not changed", fmt.Sprintf(`Documentation or resource file is changed
while related language file '%s' is not changed`, otherName))
}
