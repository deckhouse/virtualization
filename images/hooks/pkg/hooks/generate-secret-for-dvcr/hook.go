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
	"encoding/base64"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"hooks/pkg/settings"

	"golang.org/x/crypto/bcrypt"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
)

const (
	dvcrSecrets         = "dvcr-secrets"
	passwordRWValuePath = "virtualization.internal.dvcr.passwordRW"
	saltValuePath       = "virtualization.internal.dvcr.salt"
	htpasswdValuePath   = "virtualization.internal.dvcr.htpasswd"
	user                = "admin"
)

type dvcrSecretData struct {
	PasswordRW string `json:"passwordRW"`
	Salt       string `json:"salt"`
	Htpasswd   string `json:"htpasswd"`
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

func handlerDVCRSecrets(_ context.Context, input *pkg.HookInput) error {
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
		PasswordRW: input.Values.Get(passwordRWValuePath).String(),
		Salt:       input.Values.Get(saltValuePath).String(),
		Htpasswd:   input.Values.Get(htpasswdValuePath).String(),
	}
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
	rnd := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))

	b := make([]byte, length)
	for i := range b {
		b[i] = letterBytes[rnd.IntN(len(letterBytes))]
	}
	return string(b)
}
