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

// Integration tests for verifyJWT: mint tokens as the controller (golang-jwt) and
// as an x5c attacker (go-jose), verify through the real distribution backend. The
// only end-to-end check of the empty-Roots x5c defense.

package dvcrk8s

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	golangjwt "github.com/golang-jwt/jwt/v5"

	jose "github.com/go-jose/go-jose/v4"
)

const (
	testIssuer   = "virtualization-controller"
	testAudience = "dvcr"
	testKeyID    = "dvcr"
)

func newController(t *testing.T, pub crypto.PublicKey) *accessController {
	t.Helper()
	return &accessController{
		realm:       "dvcr",
		jwtIssuer:   testIssuer,
		jwtAudience: testAudience,
		trustedKeys: map[string]crypto.PublicKey{testKeyID: pub},
	}
}

func testClaims(now time.Time) map[string]interface{} {
	return map[string]interface{}{
		"iss": testIssuer,
		"aud": testAudience,
		"iat": now.Unix(),
		"nbf": now.Add(-30 * time.Second).Unix(),
		"exp": now.Add(time.Hour).Unix(),
		"access": []map[string]interface{}{
			{"type": "repository", "name": "cvi/img", "actions": []string{"pull", "push"}},
		},
	}
}

// mintKidToken signs a token the way the controller does: golang-jwt/v5, ES256,
// kid header, no embedded key material.
func mintKidToken(t *testing.T, key *ecdsa.PrivateKey, claims map[string]interface{}) string {
	t.Helper()
	tok := golangjwt.NewWithClaims(golangjwt.SigningMethodES256, golangjwt.MapClaims(claims))
	tok.Header["kid"] = testKeyID
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign kid token: %v", err)
	}
	return raw
}

// mintX5cToken forges a token carrying its own self-signed certificate in the
// x5c header — the attack the empty-Roots pool must defeat.
func mintX5cToken(t *testing.T, claims map[string]interface{}) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "attacker"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	opts := (&jose.SignerOptions{}).WithHeader("x5c", []string{base64.StdEncoding.EncodeToString(certDER)})
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, opts)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	jws, err := signer.Sign(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jws.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestVerifyJWT_ValidKidToken(t *testing.T) {
	key := newKey(t)
	ac := newController(t, &key.PublicKey)

	grants, err := ac.verifyJWT(mintKidToken(t, key, testClaims(time.Now())))
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if len(grants) != 1 || grants[0].Name != "cvi/img" || grants[0].Type != "repository" {
		t.Fatalf("unexpected grants: %+v", grants)
	}
}

// The core security guarantee: a token whose signing key is smuggled in via x5c
// must not be trusted, even though its self-signed cert is internally valid.
func TestVerifyJWT_X5cTokenRejected(t *testing.T) {
	key := newKey(t)
	ac := newController(t, &key.PublicKey)

	if _, err := ac.verifyJWT(mintX5cToken(t, testClaims(time.Now()))); err == nil {
		t.Fatal("x5c-signed token must be rejected")
	}
}

func TestVerifyJWT_Rejections(t *testing.T) {
	key := newKey(t)
	ac := newController(t, &key.PublicKey)
	now := time.Now()

	t.Run("expired", func(t *testing.T) {
		claims := testClaims(now)
		claims["exp"] = now.Add(-time.Hour).Unix()
		if _, err := ac.verifyJWT(mintKidToken(t, key, claims)); err == nil {
			t.Fatal("expired token accepted")
		}
	})

	t.Run("wrong issuer", func(t *testing.T) {
		claims := testClaims(now)
		claims["iss"] = "someone-else"
		if _, err := ac.verifyJWT(mintKidToken(t, key, claims)); err == nil {
			t.Fatal("token with wrong issuer accepted")
		}
	})

	t.Run("wrong audience", func(t *testing.T) {
		claims := testClaims(now)
		claims["aud"] = "not-dvcr"
		if _, err := ac.verifyJWT(mintKidToken(t, key, claims)); err == nil {
			t.Fatal("token with wrong audience accepted")
		}
	})

	t.Run("wrong signing key", func(t *testing.T) {
		attacker := newKey(t)
		if _, err := ac.verifyJWT(mintKidToken(t, attacker, testClaims(now))); err == nil {
			t.Fatal("token signed by untrusted key accepted")
		}
	})

	t.Run("tampered payload", func(t *testing.T) {
		raw := mintKidToken(t, key, testClaims(now))
		tampered := raw[:len(raw)-3] + "AAA"
		if _, err := ac.verifyJWT(tampered); err == nil {
			t.Fatal("tampered token accepted")
		}
	})
}

func TestClassify_StaticCredentials(t *testing.T) {
	key := newKey(t)
	ac := newController(t, &key.PublicKey)
	ac.adminUsername = "admin"
	ac.adminPassword = []byte("admin-pass")
	ac.pullerUsername = "node-puller"
	ac.pullerPassword = []byte("puller-pass")

	if s, _, err := ac.classify("admin", "admin-pass"); err != nil || s.Role != RoleAdmin {
		t.Fatalf("admin credential: role=%v err=%v", s.Role, err)
	}
	if s, _, err := ac.classify("node-puller", "puller-pass"); err != nil || s.Role != RolePuller {
		t.Fatalf("puller credential: role=%v err=%v", s.Role, err)
	}
	// A scoped token presented under the admin username must not become admin.
	scoped := mintKidToken(t, key, testClaims(time.Now()))
	if s, _, err := ac.classify("admin", scoped); err != nil || s.Role != RoleScoped {
		t.Fatalf("scoped token as admin username: role=%v err=%v", s.Role, err)
	}
}
