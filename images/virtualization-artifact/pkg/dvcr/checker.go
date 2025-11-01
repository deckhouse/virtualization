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

package dvcr

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ImageChecker provides functionality to check if images exist in a registry.
type ImageChecker interface {
	CheckImageExists(ctx context.Context, imageURL string) (bool, error)
}

// DefaultImageChecker implements ImageChecker using go-containerregistry.
type DefaultImageChecker struct {
	username string
	password string
	insecure bool
}

// NewImageChecker creates a new ImageChecker with the provided authentication credentials.
func NewImageChecker(username, password string, insecure bool) ImageChecker {
	return &DefaultImageChecker{
		username: username,
		password: password,
		insecure: insecure,
	}
}

// CheckImageExists checks if an image exists in the registry by performing a lightweight HEAD request.
// It returns true if the image exists, false if it doesn't exist, and an error for other failures.
func (c *DefaultImageChecker) CheckImageExists(ctx context.Context, imageURL string) (bool, error) {
	if imageURL == "" {
		return false, fmt.Errorf("image URL is empty")
	}

	ref, err := name.ParseReference(imageURL, c.nameOptions()...)
	if err != nil {
		return false, fmt.Errorf("failed to parse image reference %q: %w", imageURL, err)
	}

	_, err = remote.Head(ref, c.remoteOptions(ctx)...)
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check image existence for %q: %w", imageURL, err)
	}

	return true, nil
}

// nameOptions returns the name options for parsing image references.
func (c *DefaultImageChecker) nameOptions() []name.Option {
	opts := []name.Option{}
	if c.insecure {
		opts = append(opts, name.Insecure)
	}
	return opts
}

// remoteOptions returns the remote options for registry operations.
func (c *DefaultImageChecker) remoteOptions(ctx context.Context) []remote.Option {
	opts := []remote.Option{
		remote.WithContext(ctx),
	}

	if c.username != "" || c.password != "" {
		opts = append(opts, remote.WithAuth(&authn.Basic{
			Username: c.username,
			Password: c.password,
		}))
	}

	if c.insecure {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = tlsConfig
		opts = append(opts, remote.WithTransport(transport))
	}

	return opts
}

// isNotFoundError checks if the error indicates that the image was not found.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "MANIFEST_UNKNOWN") ||
		strings.Contains(errStr, "NAME_UNKNOWN") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "404")
}
