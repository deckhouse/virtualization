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

package license

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_CELicenseRe(t *testing.T) {
	validCases := []struct {
		title   string
		content string
	}{
		{
			title: "Bash comment with previous spaces",
			content: `# Copyright 2024 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.`,
		},
		{
			title: "Bash with shebang",
			content: `#!/bin/bash

# Copyright 2021 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


set -Eeo pipefail`,
		},
		{
			title: "Golang multiline comment without previous spaces",
			content: `/*
Copyright 2027 Flant JSC

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
  "fmt"
  "os"
)

func main() {
	fmt.Printf("Hello, world!")
    os.Exit(0)
}
`,
		},
		{
			title: "Golang multiline comment with previous spaces",
			content: `
/*
Copyright 2032 Flant JSC

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
  "fmt"
  "os"
)

func main() {
	fmt.Printf("Hello, world!")
    os.Exit(0)
}
`,
		},

		{
			title: "Golang multiple one line comments without previous spaces",
			content: `// Copyright 2029 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
  "fmt"
  "os"
)

func main() {
	fmt.Printf("Hello, world!")
    os.Exit(0)
}
`,
		},

		{
			title: "Golang multiple one line comments with previous spaces",
			content: `
// Copyright 2021 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
  "fmt"
  "os"
)

func main() {
	fmt.Printf("Hello, world!")
    os.Exit(0)
}
`,
		},

		{
			title: "Lua multiple one line comments without previous spaces",
			content: `--[[
Copyright 2021 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
--]]

local a = require "table.nkeys"

print("Hello")
`,
		},
	}
	for _, c := range validCases {
		t.Run(c.title, func(t *testing.T) {
			require.True(t, CELicenseRe.MatchString(c.content), "should detect license")
		})
	}
}

func Test_detectLicense(t *testing.T) {
	cases := []struct {
		title   string
		content string
		want    licenseKind
	}{
		{"no header", "package main\n", kindNone},
		{"ce header", blockComment(ceText), kindCE},
		{"ce header without a year", strings.Replace(blockComment(ceText), fmt.Sprintf("Copyright %d Flant", time.Now().Year()), "Copyright Flant", 1), kindCE},
		{"ce header, hash comments", hashComment(ceText), kindCE},
		{"ee header", blockComment(eeText), kindEE},
		{"autogenerated", "// Code generated by mockgen. DO NOT EDIT.\n", kindGenerated},
		{"foreign copyright", "// Copyright The Kubernetes Authors\n", kindForeign},
		{"flant copyright, no known template", "// Copyright 2024 Flant JSC. All rights reserved.\n", kindInvalid},
		{"other copyright", "// Copyright 2020 Acme Inc\n", kindForeign},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			require.Equal(t, c.want, detectLicense([]byte(c.content)))
		})
	}
}

// A mention of Deckhouse or Flant in the body of a file must not pass for a
// license header. This is a shell script from .github/scripts that has no
// header and only talks about Deckhouse.
func Test_detectLicense_mentionIsNotAHeader(t *testing.T) {
	body := `#!/usr/bin/env bash

show_deckhouse_state() {
  echo \"[DEBUG] Show deckhouse pods\"
  kubectl -n d8-system get pods -l app=deckhouse -o wide || true
}
`
	require.Equal(t, kindNone, detectLicense(fileHeader([]byte(body))))
}

func Test_fileHeader(t *testing.T) {
	require.Len(t, fileHeader(make([]byte, headerLimit*2)), headerLimit)
	require.Len(t, fileHeader(make([]byte, 10)), 10)
}

// hashComment must not leave trailing whitespace: .editorconfig trims it.
func Test_hashComment(t *testing.T) {
	out := hashComment(ceText)
	for _, line := range strings.Split(out, "\n") {
		require.Equal(t, strings.TrimRight(line, " \t"), line, "trailing whitespace in %q", line)
	}
	require.Contains(t, out, "\n#\n")
	require.True(t, strings.HasPrefix(out, "# Copyright "))
}

func Test_licenseForFile(t *testing.T) {
	goEE := []byte("//go:build EE\n// +build EE\n\npackage main\n")
	plain := []byte("package main\n")

	require.Contains(t, licenseForFile("edition.go", plain, false), "Apache License", "CE by default")
	require.Contains(t, licenseForFile("edition_ee.go", goEE, false), "Enterprise Edition (EE) license", "EE build constraint wins")
	require.Contains(t, licenseForFile("edition.go", plain, true), "Enterprise Edition (EE) license", "--license ee")
	require.Empty(t, licenseForFile("hack/enable", plain, false), "unknown comment style")
	require.Empty(t, licenseForFile("config.yaml", plain, false), "YAML carries no header")
}

func Test_parseLicenseFlag(t *testing.T) {
	for _, value := range []string{"ce", "CE", ""} {
		forceEE, err := parseLicenseFlag(value)
		if value == "" {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.False(t, forceEE)
	}
	for _, value := range []string{"ee", "EE"} {
		forceEE, err := parseLicenseFlag(value)
		require.NoError(t, err)
		require.True(t, forceEE)
	}
	_, err := parseLicenseFlag("apache")
	require.Error(t, err)
}

func Test_writeWithLicense(t *testing.T) {
	cases := []struct {
		title   string
		name    string
		content string
		license string
		// wantFirst is the line that must stay at the very top of the file,
		// wantBody the content that must survive below the header.
		wantFirst string
		wantBody  string
	}{
		{"plain go file", "main.go", "package main\n", blockComment(ceText), "/*", "package main\n"},
		{"shebang stays first", "run.sh", "#!/bin/bash\necho hi\n", hashComment(ceText), "#!/bin/bash", "echo hi\n"},
		{"build constraint stays first", "edition_ee.go", "//go:build EE\n// +build EE\n\npackage main\n", blockComment(eeText), "//go:build EE", "package main\n"},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), c.name)
			require.NoError(t, os.WriteFile(path, []byte(c.content), 0o755))
			require.NoError(t, writeWithLicense(path, []byte(c.content), c.license, 0o755))

			out, err := os.ReadFile(path)
			require.NoError(t, err)
			got := string(out)

			require.Equal(t, c.wantFirst, strings.SplitN(got, "\n", 2)[0])
			require.NotEqual(t, kindNone, detectLicense(fileHeader(out)), "header must be detectable")
			require.True(t, strings.HasSuffix(got, c.wantBody), "original content is kept: %q", got)

			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0o755), info.Mode().Perm(), "permissions are preserved")

			// A second run is a no-op: the file now has a header.
			require.NotEqual(t, kindNone, detectLicense(fileHeader(out)))
		})
	}
}

func Test_FilesPathWithExtensionRe(t *testing.T) {
	filePathCases := []struct {
		title    string
		filePath string
	}{
		{title: "Path with .github js extension", filePath: "./.github/scripts/js/report.js"},
		{title: "Path with sh extension", filePath: "./run.sh"},
		{title: "Path with py extension", filePath: "./scripts/run.py"},
		{title: "Path with go extension", filePath: "./cmds/run.go"},
	}

	for _, c := range filePathCases {
		t.Run(c.title, func(t *testing.T) {
			require.True(t, fileToCheckRe.MatchString(c.filePath))

			// Copyright is maintained for files with an extension
			license := licenseForFile(c.filePath, nil, false)
			require.NotEmpty(t, license)
			require.True(t, CELicenseRe.MatchString(license))
		})
	}
}

// YAML is out of scope: no header is required in the chart or in CI config.
func Test_FilesPathYamlNotChecked(t *testing.T) {
	for _, p := range []string{
		"./.github/workflows/build.yaml",
		"/some/folder/.github/workflows/build.yml",
		"/repo/.gitlab/ci/jobs/lint-validate.yml",
		"/repo/templates/monitoring.yaml",
	} {
		t.Run(p, func(t *testing.T) {
			require.Falsef(t, fileToCheckRe.MatchString(p), "expected %q not to be checked", p)
			require.Empty(t, licenseForFile(p, nil, false))
		})
	}
}

func Test_fileToSkipRe(t *testing.T) {
	skipped := []string{
		"./.github/CODEOWNERS",
		"/repo/.gitlab/CODEOWNERS",
		"/repo/hack/modules_menu_skip",
		"/repo/LICENSE",
		"/repo/images/Dockerfile",
		"/repo/Taskfile.yaml",
		"/repo/.git/config",
		"/repo/docs/README.md",
		"/repo/vendor/foo/bar.go",
		"/repo/.github/scripts/js/node_modules/pkg/index.js",
		"/repo/images/virtualization-dra/pkg/libusb/testdata/sys/uevent",
		"/repo/images/virt-api/__virt/pkg/foo.go",
		// Third-party trees excluded from the license rule in .dmtlint.yaml.
		"/repo/images/dvcr-artifact/staging/src/github.com/docker/docker/registry/registry.go",
		"/repo/images/pvc-artifact/staging/src/github.com/docker/docker/registry/registry.go",
		"/repo/test/performance/ssh/id_ed",
	}
	for _, p := range skipped {
		t.Run("skip "+p, func(t *testing.T) {
			require.Truef(t, fileToSkipRe.MatchString(p), "expected %q to be skipped", p)
		})
	}

	notSkipped := []string{
		"/repo/pkg/main.go",
		"/repo/hack/run.sh",
		// A dunder file is not a vendored upstream tree.
		"/repo/.gitlab/ci/scripts/python/tests/__init__.py",
	}
	for _, p := range notSkipped {
		t.Run("keep "+p, func(t *testing.T) {
			require.Falsef(t, fileToSkipRe.MatchString(p), "expected %q not to be skipped", p)
		})
	}
}

func Test_FilesPathNoExtensionRe(t *testing.T) {
	filePathCases := []struct {
		title    string
		filePath string
	}{
		{title: "Path with no extension", filePath: "./cmds/enable"},
		{title: "Path with no extension root dir", filePath: "/enable"},
	}

	for _, c := range filePathCases {
		t.Run(c.title, func(t *testing.T) {
			require.True(t, fileToCheckRe.MatchString(c.filePath))

			// Copyright is not maintained for files without an extension
			license := licenseForFile(c.filePath, nil, false)
			require.Empty(t, license)
			require.False(t, CELicenseRe.MatchString(license))
		})
	}
}
