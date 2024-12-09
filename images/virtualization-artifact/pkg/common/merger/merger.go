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

package merger

// MergeLabels merges maps of labels into one map.
// Labels in the first argument are
// overridden with labels from the next argument and so on.
func MergeLabels(in ...map[string]string) map[string]string {
	res := make(map[string]string)

	for _, labels := range in {
		for k, v := range labels {
			res[k] = v
		}
	}

	return res
}

// ApplyMapChanges merges to the target all keys and values from the current version,
// removes from the target the keys that were present in the previous version but are absent in the current one.
// It returns true if the keys or values of the target have changed.
func ApplyMapChanges(target, prev, cur map[string]string) (map[string]string, bool) {
	if target == nil {
		target = map[string]string{}
	}

	var isChanged bool

	for key, value := range cur {
		if val, ok := target[key]; !ok || val != value {
			target[key] = value
			isChanged = true
		}
	}

	for key := range prev {
		_, currHasKey := cur[key]
		_, targetHasKey := target[key]
		if !currHasKey && targetHasKey {
			delete(target, key)
			isChanged = true
		}
	}

	return target, isChanged
}
