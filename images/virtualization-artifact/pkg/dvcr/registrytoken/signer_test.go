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

package registrytoken

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newSignerPEM(t *testing.T) ([]byte, *ecdsa.PublicKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), &key.PublicKey
}

func TestSignRoundTrip(t *testing.T) {
	keyPEM, pub := newSignerPEM(t)
	signer, err := NewSignerFromPEM(keyPEM)
	if err != nil {
		t.Fatal(err)
	}

	access := []Access{{Type: "repository", Name: "cvi/my-image", Actions: []string{"pull", "push"}}}
	now := time.Unix(1_700_000_000, 0)
	raw, err := signer.Sign(access, now)
	if err != nil {
		t.Fatal(err)
	}

	// Verify signature and claims the way the registry will.
	parsed, err := jwt.Parse(raw, func(*jwt.Token) (interface{}, error) { return pub, nil },
		jwt.WithValidMethods([]string{"ES256"}),
		jwt.WithTimeFunc(func() time.Time { return now.Add(time.Minute) }))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if kid := parsed.Header["kid"]; kid != KeyID {
		t.Errorf("kid = %v, want %q", kid, KeyID)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if claims["iss"] != Issuer {
		t.Errorf("iss = %v, want %q", claims["iss"], Issuer)
	}
	if claims["aud"] != Audience {
		t.Errorf("aud = %v, want %q", claims["aud"], Audience)
	}
	acc, ok := claims["access"].([]interface{})
	if !ok || len(acc) != 1 {
		t.Fatalf("access claim = %v", claims["access"])
	}
	entry := acc[0].(map[string]interface{})
	if entry["name"] != "cvi/my-image" || entry["type"] != "repository" {
		t.Errorf("access entry = %v", entry)
	}
}

func TestSignExpired(t *testing.T) {
	keyPEM, pub := newSignerPEM(t)
	signer, _ := NewSignerFromPEM(keyPEM)
	now := time.Unix(1_700_000_000, 0)
	raw, err := signer.Sign(nil, now)
	if err != nil {
		t.Fatal(err)
	}
	// Past DefaultTTL the token must no longer verify.
	_, err = jwt.Parse(raw, func(*jwt.Token) (interface{}, error) { return pub, nil },
		jwt.WithTimeFunc(func() time.Time { return now.Add(DefaultTTL + time.Hour) }))
	if err == nil {
		t.Fatal("expected expired token to fail verification")
	}
}
