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

// Build tag dvcr_registry: this file depends on the distribution registry module
// and is only compiled inside the werf DVCR build (which passes -tags dvcr_registry).
// The dependency-free policy in policy.go stays unit-testable without it.

//go:build dvcr_registry

package dvcrk8s

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/distribution/distribution/v3/registry/auth"
)

func init() {
	if err := auth.Register("dvcr-k8s", auth.InitFunc(newAccessController)); err != nil {
		log.Printf("failed to register dvcr-k8s auth: %v", err)
	}
}

type accessController struct {
	realm string

	adminUsername string
	adminPassword []byte

	pullerUsername string
	pullerPassword []byte

	privilegedNamespace string

	reviewer *tokenReviewer
}

// newAccessController builds the controller from the `auth: { dvcr-k8s: {...} }`
// config block. Options:
//
//	realm                   string (WWW-Authenticate realm)
//	adminusername           string
//	adminpasswordfile       string (path to the admin password, e.g. from dvcr-secrets)
//	pullerusername          string
//	pullerpasswordfile      string (path to the node-puller password)
//	tokenreviewcachettl     string (Go duration, default 45s)
//	tokenreviewcachenegttl  string (Go duration for failed reviews, default 5s)
func newAccessController(options map[string]interface{}) (auth.AccessController, error) {
	realm, err := optString(options, "realm", "dvcr")
	if err != nil {
		return nil, err
	}

	adminUser, err := optString(options, "adminusername", "admin")
	if err != nil {
		return nil, err
	}
	adminPass, err := readSecretFile(options, "adminpasswordfile")
	if err != nil {
		return nil, err
	}

	pullerUser, err := optString(options, "pullerusername", "node-puller")
	if err != nil {
		return nil, err
	}
	pullerPass, err := readSecretFile(options, "pullerpasswordfile")
	if err != nil {
		return nil, err
	}

	privilegedNamespace, err := optString(options, "privilegednamespace", "")
	if err != nil {
		return nil, err
	}

	ttl := optDuration(options, "tokenreviewcachettl", 45*time.Second)
	negTTL := optDuration(options, "tokenreviewcachenegttl", 5*time.Second)

	reviewer, err := newTokenReviewer(ttl, negTTL)
	if err != nil {
		return nil, fmt.Errorf("init token reviewer: %w", err)
	}

	return &accessController{
		realm:               realm,
		adminUsername:       adminUser,
		adminPassword:       adminPass,
		pullerUsername:      pullerUser,
		pullerPassword:      pullerPass,
		privilegedNamespace: privilegedNamespace,
		reviewer:            reviewer,
	}, nil
}

// Authorized implements auth.AccessController for distribution v3.
func (ac *accessController) Authorized(req *http.Request, accessRecords ...auth.Access) (*auth.Grant, error) {
	username, password, ok := req.BasicAuth()
	if !ok || password == "" {
		return nil, &challenge{realm: ac.realm, err: auth.ErrInvalidCredential}
	}

	subject, name, err := ac.classify(req, username, password)
	if err != nil {
		// Bad credential -> 401 challenge so the client may retry with valid creds.
		return nil, &challenge{realm: ac.realm, err: err}
	}

	if !Authorize(subject, toPolicyAccess(accessRecords)) {
		// Authenticated but not permitted -> 403 (plain error, not a challenge).
		return nil, fmt.Errorf("dvcr-k8s: access denied for %q: %w", name, auth.ErrAuthenticationFailure)
	}

	return &auth.Grant{User: auth.UserInfo{Name: name}}, nil
}

// classify maps a Basic credential to an authorization Subject. Static admin and
// node-puller passwords are matched in constant time; anything else is treated as
// a ServiceAccount token and verified via TokenReview (fail-closed on any error).
func (ac *accessController) classify(req *http.Request, username, password string) (Subject, string, error) {
	if len(ac.adminPassword) > 0 && username == ac.adminUsername &&
		subtle.ConstantTimeCompare([]byte(password), ac.adminPassword) == 1 {
		return Subject{Role: RoleAdmin}, username, nil
	}

	if len(ac.pullerPassword) > 0 && username == ac.pullerUsername &&
		subtle.ConstantTimeCompare([]byte(password), ac.pullerPassword) == 1 {
		return Subject{Role: RolePuller}, username, nil
	}

	ns, err := ac.reviewer.namespaceForToken(req.Context(), password)
	if err != nil {
		return Subject{}, "", fmt.Errorf("token review: %w", err)
	}
	subject := SubjectForServiceAccount(ns, ac.privilegedNamespace)
	if subject.Role == RoleNone {
		return Subject{}, "", errors.New("token is not a namespaced ServiceAccount")
	}
	return subject, "serviceaccount:" + ns, nil
}

func toPolicyAccess(records []auth.Access) []Access {
	out := make([]Access, len(records))
	for i, r := range records {
		out[i] = Access{Type: r.Type, Name: r.Name, Action: r.Action}
	}
	return out
}

// challenge implements auth.Challenge for a 401 Basic auth response.
type challenge struct {
	realm string
	err   error
}

func (ch *challenge) SetHeaders(_ *http.Request, w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=%q", ch.realm))
}

func (ch *challenge) Error() string {
	return fmt.Sprintf("dvcr-k8s basic auth challenge for realm %q: %v", ch.realm, ch.err)
}

func optString(options map[string]interface{}, key, def string) (string, error) {
	v, present := options[key]
	if !present {
		return def, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("dvcr-k8s: option %q must be a string", key)
	}
	return s, nil
}

func optDuration(options map[string]interface{}, key string, def time.Duration) time.Duration {
	v, present := options[key]
	if !present {
		return def
	}
	s, ok := v.(string)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return d
}

func readSecretFile(options map[string]interface{}, key string) ([]byte, error) {
	path, err := optString(options, key, "")
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("dvcr-k8s: read %q from %s: %w", key, path, err)
	}
	return []byte(strings.TrimSpace(string(data))), nil
}
