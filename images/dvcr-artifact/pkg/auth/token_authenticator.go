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

package auth

import (
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
)

// tokenFileAuthenticator authenticates to DVCR with a projected ServiceAccount
// token read fresh from disk on every request. Reading per-call is required
// because kubelet rotates the projected token file; a token cached once would
// break long pushes when it rotates mid-transfer.
type tokenFileAuthenticator struct {
	path string
}

// TokenFileAuthenticator returns an authn.Authenticator that presents the
// contents of the token file as the Basic auth password. The username is a
// fixed non-empty placeholder; the DVCR authorization backend only inspects the
// token in the password.
func TokenFileAuthenticator(path string) authn.Authenticator {
	return &tokenFileAuthenticator{path: path}
}

func (t *tokenFileAuthenticator) Authorization() (*authn.AuthConfig, error) {
	token, err := os.ReadFile(t.path)
	if err != nil {
		return nil, err
	}
	return &authn.AuthConfig{
		Username: "sa",
		Password: strings.TrimSpace(string(token)),
	}, nil
}
