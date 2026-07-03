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

//go:build dvcr_registry

package dvcrk8s

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	saDir       = "/var/run/secrets/kubernetes.io/serviceaccount"
	saTokenPath = saDir + "/token"
	saCAPath    = saDir + "/ca.crt"

	tokenReviewPath = "/apis/authentication.k8s.io/v1/tokenreviews"
)

var errTokenRejected = errors.New("token rejected by TokenReview")

// tokenReviewer verifies ServiceAccount tokens via the Kubernetes TokenReview API
// using only the standard library (no client-go). Results are cached to keep the
// hot push path off the apiserver.
type tokenReviewer struct {
	client *http.Client
	apiURL string
	ttl    time.Duration
	negTTL time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	ns     string
	ok     bool
	expiry time.Time
}

func newTokenReviewer(ttl, negTTL time.Duration) (*tokenReviewer, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("KUBERNETES_SERVICE_HOST/PORT not set (not running in-cluster?)")
	}

	caPEM, err := os.ReadFile(saCAPath)
	if err != nil {
		return nil, fmt.Errorf("read apiserver CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("parse apiserver CA")
	}

	return &tokenReviewer{
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
			},
		},
		apiURL: fmt.Sprintf("https://%s:%s%s", host, port, tokenReviewPath),
		ttl:    ttl,
		negTTL: negTTL,
		cache:  make(map[string]cacheEntry),
	}, nil
}

// namespaceForToken returns the ServiceAccount namespace for a token, or an error
// if the token is rejected. Transient errors (network/apiserver) are not cached.
func (tr *tokenReviewer) namespaceForToken(ctx context.Context, token string) (string, error) {
	key := hashToken(token)

	if e, ok := tr.lookup(key); ok {
		if !e.ok {
			return "", errTokenRejected
		}
		return e.ns, nil
	}

	ns, ok, err := tr.review(ctx, token)
	if err != nil {
		return "", err
	}
	if !ok {
		tr.store(key, cacheEntry{ok: false, expiry: time.Now().Add(tr.negTTL)})
		return "", errTokenRejected
	}
	tr.store(key, cacheEntry{ok: true, ns: ns, expiry: time.Now().Add(tr.ttl)})
	return ns, nil
}

func (tr *tokenReviewer) lookup(key string) (cacheEntry, bool) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	e, ok := tr.cache[key]
	if !ok || time.Now().After(e.expiry) {
		if ok {
			delete(tr.cache, key)
		}
		return cacheEntry{}, false
	}
	return e, true
}

func (tr *tokenReviewer) store(key string, e cacheEntry) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.cache[key] = e
}

// review performs a single TokenReview call. It authenticates to the apiserver
// with the registry's own ServiceAccount token, read fresh each call because the
// projected token rotates.
func (tr *tokenReviewer) review(ctx context.Context, token string) (ns string, authenticated bool, err error) {
	reqBody, err := json.Marshal(map[string]any{
		"apiVersion": "authentication.k8s.io/v1",
		"kind":       "TokenReview",
		"spec":       map[string]any{"token": token},
	})
	if err != nil {
		return "", false, err
	}

	ownToken, err := os.ReadFile(saTokenPath)
	if err != nil {
		return "", false, fmt.Errorf("read own SA token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tr.apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(ownToken)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := tr.client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", false, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("TokenReview HTTP %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		Status struct {
			Authenticated bool `json:"authenticated"`
			User          struct {
				Username string `json:"username"`
			} `json:"user"`
			Error string `json:"error"`
		} `json:"status"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", false, fmt.Errorf("decode TokenReview: %w", err)
	}
	if !out.Status.Authenticated {
		return "", false, nil
	}
	return namespaceFromUsername(out.Status.User.Username), true, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
