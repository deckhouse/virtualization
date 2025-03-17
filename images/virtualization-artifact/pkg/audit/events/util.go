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

package events

import (
	"fmt"
	"net/url"
)

// removeAllQueryParams removes all query parameters from the given URI.
//
// @param uri The URI string from which query parameters need to be removed.
//
// @return A string representing the URI without query parameters, or an error if the URI parsing fails.
func removeAllQueryParams(uri string) (string, error) {
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("failed to parse URI: %w", err)
	}

	parsedURL.RawQuery = ""

	return parsedURL.String(), nil
}
