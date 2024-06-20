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

package main

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
			res := CELicenseRe.MatchString(c.content)
			require.Equal(t, true, res)

			if !res {
				t.Errorf("should detect license")
			}
		})
	}
}

func Test_FilesPathWithExtensionRe(t *testing.T) {
	filePathCases := []struct {
		title    string
		filePath string
	}{
		{
			title:    "Path .github with yaml extension",
			filePath: "./.github/workflows/build.yaml",
		},
		{
			title:    "Path with /some/folder/.github yaml extension",
			filePath: "/some/folder/.github/workflows/build.yaml",
		},
		{
			title:    "Path with ./.github yml extension",
			filePath: "./.github/workflows/build.yml",
		},
		{
			title:    "Path with sh extension",
			filePath: "./run.sh",
		},
		{
			title:    "Path with py extension",
			filePath: "./scripts/run.py",
		},
		{
			title:    "Path with go extension",
			filePath: "./cmds/run.go",
		},
	}

	for _, c := range filePathCases {
		t.Run(c.title, func(t *testing.T) {
			resFilePathMatch := fileToCheckRe.MatchString(c.filePath)
			require.True(t, resFilePathMatch)

			// Copyright is maintained for files with an extension
			license := getLicenseForFile(c.filePath)
			require.NotEmpty(t, license)
			require.True(t, CELicenseRe.MatchString(license))
		})
	}
}

func Test_FilesPathNoExtensionRe(t *testing.T) {
	filePathCases := []struct {
		title    string
		filePath string
	}{
		{
			title:    "Path with no extension",
			filePath: "./cmds/enable",
		},
		{
			title:    "Path with no extension root dir",
			filePath: "/enable",
		},
	}

	for _, c := range filePathCases {
		t.Run(c.title, func(t *testing.T) {
			resFilePathMatch := fileToCheckRe.MatchString(c.filePath)
			require.True(t, resFilePathMatch)

			// Copyright is not maintained for files without an extension
			license := getLicenseForFile(c.filePath)
			require.Empty(t, license)
			require.False(t, CELicenseRe.MatchString(license))
		})
	}
}
