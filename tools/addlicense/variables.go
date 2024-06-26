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
	"fmt"
	"regexp"
	"strings"
	"time"
)

var licenseText = fmt.Sprintf(`Copyright %d Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
`, time.Now().Year())

var (
	goLicense         = "/*\n" + licenseText + "*/\n"
	bashPythonLicense = "# " + strings.ReplaceAll(strings.TrimSpace(licenseText), "\n", "\n# ") + "\n"
)

// fileToCheckRe matches files that will be checked for adding license, meet the following conditions:
//   - Ends with .go, .sh, .py
//   - Is inside a .github directory: scripts, workflows, or workflow_templates subdirectories,
//     and ends with .js, .yml, .yaml, or .sh
var fileToCheckRe = regexp.MustCompile(`\.go$|/[^/.]+$|\.sh$|\.py$|\.github/(scripts|workflows|workflow_templates)/.+\.(js|yml|yaml|sh)$`)

// fileToSkipRe matches filenames that will be skipped for adding license, meet the following conditions:
//   - Directories .github/CODEOWNERS, /docs/
//   - Filename contains Dockerfile, Makefile, Taskfile, LICENSE
//   - Ends with geohash.lua, bashrc, inputrc, modules_menu_skip
var fileToSkipRe = regexp.MustCompile(`geohash.lua$|.git/|\.
github/CODEOWNERS|Dockerfile$|Makefile$|Taskfile|/docs/|bashrc$|inputrc$|modules_menu_skip$
|LICENSE$`)

var CELicenseRe = regexp.MustCompile(`(?s)[/#{!-]*(\s)*Copyright 20[2-9][1-9] Flant JSC[-!}\s\n#/]*
[/#{!-]*(\s)*Licensed under the Apache License, Version 2.0 \(the "License"\);[-!}\n]*
[/#{!-]*(\s)*you may not use this file except in compliance with the License.[-!}\n]*
[/#{!-]*(\s)*You may obtain a copy of the License at[-!}\n\s#/]*
[/#{!-]*(\s)*http://www.apache.org/licenses/LICENSE-2.0[-!}\n\s#/]*
[/#{!-]*(\s)*Unless required by applicable law or agreed to in writing, software[-!}\n]*
[/#{!-]*(\s)*distributed under the License is distributed on an "AS IS" BASIS,[-!}\n]*
[/#{!-]*(\s)*WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.[-!}\n]*
[/#{!-]*(\s)*See the License for the specific language governing permissions and[-!}\n]*
[/#{!-]*(\s)*limitations under the License.[-!}\n]{0,1}`)

var (
	copyrightOrAutogenRe = regexp.MustCompile(`Copyright The|autogenerated|DO NOT EDIT`)
	copyrightRe          = regexp.MustCompile(`Copyright`)
	flantRe              = regexp.MustCompile(`Flant|Deckhouse`)
)
