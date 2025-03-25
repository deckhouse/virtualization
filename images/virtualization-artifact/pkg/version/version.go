/*
Copyright 2025 Flant JSC

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

package version

import "golang.org/x/mod/semver"

const mainVersion Version = "main"

type Version string

func (v Version) IsValid() bool {
	return v == mainVersion || semver.IsValid(string(v))
}

func (v Version) IsMain() bool {
	return v == mainVersion
}

func (v Version) String() string {
	return string(v)
}

func (v Version) Compare(v2 Version) int {
	vIsMain := v.IsMain()
	v2IsMain := v2.IsMain()

	switch {
	case vIsMain && v2IsMain:
		return 0
	case vIsMain:
		return 1
	case v2IsMain:
		return -1
	}

	return semver.Compare(v.String(), v2.String())
}
