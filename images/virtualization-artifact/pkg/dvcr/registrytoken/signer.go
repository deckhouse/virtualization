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

// Package registrytoken mints scoped JWT credentials for DVCR. An importer or
// uploader Pod presents such a token instead of the shared read-write password;
// the token authorizes exactly the repositories that Pod reads from and writes to.
// The DVCR registry verifies it with distribution's own token backend (the format
// matches its ClaimSet / ResourceActions), so no shared read-write credential is
// copied into tenant namespaces.
package registrytoken

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// Issuer is the token issuer; the registry accepts only this value.
	Issuer = "virtualization-controller"
	// Audience is the token audience; the registry accepts only this value.
	Audience = "dvcr"
	// KeyID is the JWS `kid` header the registry maps to the trusted public key.
	KeyID = "dvcr"
	// DefaultTTL is the scoped token lifetime. Generous on purpose: an import may
	// run for hours, and the token is scoped to a single repository, so a long
	// validity window is a small exposure.
	// ponytail: fixed TTL; bind to the Pod's activeDeadlineSeconds if imports ever
	// need to outlive this or exposure must be tightened further.
	DefaultTTL = 24 * time.Hour
)

// Access is one entry of a token's access claim: a resource and the actions
// permitted on it. JSON tags match distribution's token.ResourceActions.
type Access struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

// Signer mints ES256-signed scoped tokens with an ECDSA P-256 private key.
type Signer struct {
	key *ecdsa.PrivateKey
}

// NewSignerFromPEM builds a Signer from a PKCS8 (or SEC1) PEM-encoded ECDSA key.
func NewSignerFromPEM(keyPEM []byte) (*Signer, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("registrytoken: no PEM block in private key")
	}
	key, err := parseECKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("registrytoken: parse private key: %w", err)
	}
	return &Signer{key: key}, nil
}

func parseECKey(der []byte) (*ecdsa.PrivateKey, error) {
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		ec, ok := k.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not ECDSA")
		}
		return ec, nil
	}
	return x509.ParseECPrivateKey(der)
}

// Sign issues a token granting access, valid for ttl. A non-positive ttl uses
// DefaultTTL. now is passed explicitly to keep the function testable.
func (s *Signer) Sign(access []Access, ttl time.Duration, now time.Time) (string, error) {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	claims := jwt.MapClaims{
		"iss":    Issuer,
		"aud":    Audience,
		"iat":    now.Unix(),
		"nbf":    now.Add(-30 * time.Second).Unix(),
		"exp":    now.Add(ttl).Unix(),
		"access": access,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = KeyID
	return tok.SignedString(s.key)
}
