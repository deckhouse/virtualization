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
	"testing"

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
		{"ce header", goLicense, kindCE},
		{"autogenerated", "// Code generated by mockgen. DO NOT EDIT.\n", kindAutogen},
		{"foreign copyright The", "// Copyright The Kubernetes Authors\n", kindAutogen},
		{"flant non-ce", "// Flant JSC internal note\n", kindFlant},
		{"other copyright", "// Copyright 2020 Acme Inc\n", kindOther},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			require.Equal(t, c.want, detectLicense([]byte(c.content)))
		})
	}
}

func Test_FilesPathWithExtensionRe(t *testing.T) {
	filePathCases := []struct {
		title    string
		filePath string
	}{
		{title: "Path .github with yaml extension", filePath: "./.github/workflows/build.yaml"},
		{title: "Path with /some/folder/.github yaml extension", filePath: "/some/folder/.github/workflows/build.yaml"},
		{title: "Path with ./.github yml extension", filePath: "./.github/workflows/build.yml"},
		{title: "Path with sh extension", filePath: "./run.sh"},
		{title: "Path with py extension", filePath: "./scripts/run.py"},
		{title: "Path with go extension", filePath: "./cmds/run.go"},
	}

	for _, c := range filePathCases {
		t.Run(c.title, func(t *testing.T) {
			require.True(t, fileToCheckRe.MatchString(c.filePath))

			// Copyright is maintained for files with an extension
			license := getLicenseForFile(c.filePath)
			require.NotEmpty(t, license)
			require.True(t, CELicenseRe.MatchString(license))
		})
	}
}

func Test_fileToSkipRe(t *testing.T) {
	skipped := []string{
		"./.github/CODEOWNERS",
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
	}
	for _, p := range skipped {
		t.Run("skip "+p, func(t *testing.T) {
			require.Truef(t, fileToSkipRe.MatchString(p), "expected %q to be skipped", p)
		})
	}

	notSkipped := []string{"/repo/pkg/main.go", "/repo/hack/run.sh"}
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
			license := getLicenseForFile(c.filePath)
			require.Empty(t, license)
			require.False(t, CELicenseRe.MatchString(license))
		})
	}
}
