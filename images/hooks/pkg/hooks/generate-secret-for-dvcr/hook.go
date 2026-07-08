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

package generate_secret_for_dvcr

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/deckhouse/virtualization/hooks/pkg/settings"
)

const (
	dvcrSecrets              = "dvcr-secrets"
	passwordRWValuePath      = "virtualization.internal.dvcr.passwordRW"
	passwordROValuePath      = "virtualization.internal.dvcr.passwordRO"
	saltValuePath            = "virtualization.internal.dvcr.salt"
	htpasswdValuePath        = "virtualization.internal.dvcr.htpasswd"
	tokenPrivateKeyValuePath = "virtualization.internal.dvcr.tokenPrivateKey"
	tokenPublicKeyValuePath  = "virtualization.internal.dvcr.tokenPublicKey"
	user                     = "admin"
)

type dvcrSecretData struct {
	PasswordRW      string `json:"passwordRW"`
	PasswordRO      string `json:"passwordRO"`
	Salt            string `json:"salt"`
	Htpasswd        string `json:"htpasswd"`
	TokenPrivateKey string `json:"tokenPrivateKey"`
	TokenPublicKey  string `json:"tokenPublicKey"`
}

var _ = registry.RegisterFunc(configDVCRSecrets, handlerDVCRSecrets)

var configDVCRSecrets = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 5},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       dvcrSecrets,
			APIVersion: "v1",
			Kind:       "Secret",
			JqFilter:   ".data",

			NameSelector: &pkg.NameSelector{
				MatchNames: []string{dvcrSecrets},
			},

			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{settings.ModuleNamespace},
				},
			},
			ExecuteHookOnSynchronization: ptr.To(false),
		},
	},

	Queue: fmt.Sprintf("modules/%s", settings.ModuleName),
}

func handlerDVCRSecrets(ctx context.Context, input *pkg.HookInput) error {
	canRun, err := settings.CanRunWithModuleConfig(ctx, input)
	if err != nil {
		return err
	}
	if !canRun {
		return nil
	}

	dataFromSecret, err := getDVCRSecretsFromSecrets(input)
	if err != nil {
		return err
	}
	dataFromValues := getDVCRSecretsFromValues(input)

	passwordRW := dataFromSecret.PasswordRW
	passwordRWBytes, err := base64.StdEncoding.DecodeString(passwordRW)
	passwordRWDecoded := string(passwordRWBytes)
	if err != nil || passwordRWDecoded == "" {
		input.Logger.Info("Regenerate PasswordRW")
		passwordRWDecoded = alphaNum(32)
		passwordRW = base64.StdEncoding.EncodeToString([]byte(passwordRWDecoded))
	}
	if passwordRW != dataFromValues.PasswordRW {
		input.Logger.Info("Set PasswordRW to values")
		input.Values.Set(passwordRWValuePath, passwordRW)
	}

	// passwordRO is the pull-only node-puller credential used by the dvcr-k8s
	// authorization backend. It carries no push rights, so it does not need an
	// htpasswd entry (nodes fall back to admin only when the backend is htpasswd).
	passwordRO := dataFromSecret.PasswordRO
	passwordROBytes, err := base64.StdEncoding.DecodeString(passwordRO)
	passwordRODecoded := string(passwordROBytes)
	if err != nil || passwordRODecoded == "" {
		input.Logger.Info("Regenerate PasswordRO")
		passwordRODecoded = alphaNum(32)
		passwordRO = base64.StdEncoding.EncodeToString([]byte(passwordRODecoded))
	}
	if passwordRO != dataFromValues.PasswordRO {
		input.Logger.Info("Set PasswordRO to values")
		input.Values.Set(passwordROValuePath, passwordRO)
	}

	salt := dataFromSecret.Salt
	saltBytes, err := base64.StdEncoding.DecodeString(salt)
	saltDecoded := string(saltBytes)
	if err != nil || saltDecoded == "" {
		input.Logger.Info("Regenerate Salt")
		saltDecoded = alphaNum(32)
		salt = base64.StdEncoding.EncodeToString([]byte(saltDecoded))
	}
	if salt != dataFromValues.Salt {
		input.Logger.Info("Set Salt to values")
		input.Values.Set(saltValuePath, salt)
	}

	htpasswd := dataFromSecret.Htpasswd
	htpasswdBytes, err := base64.StdEncoding.DecodeString(htpasswd)
	htpasswdDecoded := string(htpasswdBytes)
	if err != nil || htpasswdDecoded == "" || !validateHtpasswd(passwordRWDecoded, htpasswdDecoded) {
		input.Logger.Info("Regenerate Htpasswd")
		htpasswdDecoded, err = generateHtpasswd(passwordRWDecoded)
		if err != nil {
			return fmt.Errorf("generate htpasswd: %w", err)
		}
		htpasswd = base64.StdEncoding.EncodeToString([]byte(htpasswdDecoded))
	}
	if htpasswd != dataFromValues.Htpasswd {
		input.Logger.Info("Set Htpasswd to values")
		input.Values.Set(htpasswdValuePath, htpasswd)
	}

	// tokenPrivateKey/tokenPublicKey is the ECDSA keypair used to mint and verify
	// scoped DVCR tokens when per-namespace authorization is on. Regenerate the
	// pair whenever the private key is missing or unparseable.
	tokenPrivateKey := dataFromSecret.TokenPrivateKey
	tokenPublicKey := dataFromSecret.TokenPublicKey
	privBytes, err := base64.StdEncoding.DecodeString(tokenPrivateKey)
	pubBytes, pubErr := base64.StdEncoding.DecodeString(tokenPublicKey)
	if err != nil || pubErr != nil || !validateECKeypair(privBytes, pubBytes) {
		input.Logger.Info("Regenerate DVCR token keypair")
		privPEM, pubPEM, genErr := generateECKeypair()
		if genErr != nil {
			return fmt.Errorf("generate DVCR token keypair: %w", genErr)
		}
		tokenPrivateKey = base64.StdEncoding.EncodeToString(privPEM)
		tokenPublicKey = base64.StdEncoding.EncodeToString(pubPEM)
	}
	if tokenPrivateKey != dataFromValues.TokenPrivateKey {
		input.Logger.Info("Set DVCR token private key to values")
		input.Values.Set(tokenPrivateKeyValuePath, tokenPrivateKey)
	}
	if tokenPublicKey != dataFromValues.TokenPublicKey {
		input.Logger.Info("Set DVCR token public key to values")
		input.Values.Set(tokenPublicKeyValuePath, tokenPublicKey)
	}

	return nil
}

func getDVCRSecretsFromSecrets(input *pkg.HookInput) (dvcrSecretData, error) {
	snapshots := input.Snapshots.Get(dvcrSecrets)

	dataFromSecret := dvcrSecretData{}

	if len(snapshots) > 0 {
		err := snapshots[0].UnmarshalTo(&dataFromSecret)
		if err != nil {
			return dataFromSecret, fmt.Errorf("unmarshalTo: %w", err)
		}
	}

	return dataFromSecret, nil
}

func getDVCRSecretsFromValues(input *pkg.HookInput) dvcrSecretData {
	return dvcrSecretData{
		PasswordRW:      input.Values.Get(passwordRWValuePath).String(),
		PasswordRO:      input.Values.Get(passwordROValuePath).String(),
		Salt:            input.Values.Get(saltValuePath).String(),
		Htpasswd:        input.Values.Get(htpasswdValuePath).String(),
		TokenPrivateKey: input.Values.Get(tokenPrivateKeyValuePath).String(),
		TokenPublicKey:  input.Values.Get(tokenPublicKeyValuePath).String(),
	}
}

// generateECKeypair returns a fresh ECDSA P-256 keypair as PKCS8/PKIX PEM. The
// private key mints scoped DVCR tokens (controller); the public key verifies them
// (registry).
func generateECKeypair() (privPEM, pubPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		return nil, nil, err
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	privPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	pubPEM = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return privPEM, pubPEM, nil
}

// validateECKeypair reports whether privPEM parses as ECDSA and pubPEM is its
// matching public key. A bad public key (shipped to the registry) would reject
// every scoped token, so it must force regenerating the pair too.
func validateECKeypair(privPEM, pubPEM []byte) bool {
	priv := parseECPrivateKey(privPEM)
	if priv == nil {
		return false
	}
	block, _ := pem.Decode(pubPEM)
	if block == nil {
		return false
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false
	}
	ecPub, ok := pub.(*ecdsa.PublicKey)
	return ok && ecPub.Equal(&priv.PublicKey)
}

func parseECPrivateKey(pemBytes []byte) *ecdsa.PrivateKey {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if ec, ok := k.(*ecdsa.PrivateKey); ok {
			return ec
		}
		return nil
	}
	ec, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil
	}
	return ec
}

func generateHtpasswd(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s", user, string(hashed)), nil
}

func validateHtpasswd(password, hashEntry string) bool {
	parts := strings.SplitN(hashEntry, ":", 2)
	if len(parts) != 2 {
		return false
	}

	validatedUser := parts[0]
	if validatedUser != user {
		return false
	}

	hash := parts[1]
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

//nolint:unparam // we need to pass the length
func alphaNum(length int) string {
	max := big.NewInt(int64(len(letterBytes)))
	b := make([]byte, length)
	for i := range b {
		n, err := crand.Int(crand.Reader, max)
		if err != nil {
			panic(fmt.Sprintf("generate random password: %v", err))
		}
		b[i] = letterBytes[n.Int64()]
	}
	return string(b)
}
