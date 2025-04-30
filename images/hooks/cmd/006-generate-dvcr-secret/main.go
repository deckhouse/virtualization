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

package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"hooks/pkg/common"
	"math/big"
	"strings"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/app"
	"github.com/deckhouse/module-sdk/pkg/registry"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

const (
	dvcrSecretSnapshotName = "secrets"
	dvcrSecretName         = "dvcr-secrets"
	dvcrSecretJqFilter     = `{
	  "data": .data
	}`
	dvcrUser           = "admin"
	dvcrPasswordRWPath = "virtualization.internal.dvcr.passwordRW"
	dvcrHtpasswdPath   = "virtualization.internal.dvcr.htpasswd"
	dvcrSaltPath       = "virtualization.internal.dvcr.salt"
)

type dvcrSecretData struct {
	Data struct {
		Htpasswd   string `json:"htpasswd"`
		PasswordRW string `json:"passwordRW"`
		Salt       string `json:"salt"`
	} `json:"data"`
}

func (d dvcrSecretData) GetHtpasswd() (string, error) {
	// decode base64 string to byte array
	htpasswdBytes, err := base64.StdEncoding.DecodeString(d.Data.Htpasswd)
	if err != nil {
		return "", err
	}

	return string(htpasswdBytes), nil
}

func (d dvcrSecretData) GetPasswordRW() (string, error) {
	// decode base64 string to byte array
	PasswordRWBytes, err := base64.StdEncoding.DecodeString(d.Data.PasswordRW)
	if err != nil {
		return "", err
	}

	return string(PasswordRWBytes), nil
}

func (d dvcrSecretData) GetSalt() (string, error) {
	// decode base64 string to byte array
	SaltBytes, err := base64.StdEncoding.DecodeString(d.Data.Salt)
	if err != nil {
		return "", err
	}

	return string(SaltBytes), nil
}

var config = &pkg.HookConfig{
	OnBeforeHelm: &pkg.OrderedConfig{Order: 10},
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       dvcrSecretSnapshotName,
			APIVersion: "v1",
			Kind:       "Secret",
			JqFilter:   dvcrSecretJqFilter,
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{common.MODULE_NAMESPACE},
				},
			},
			NameSelector: &pkg.NameSelector{
				MatchNames: []string{dvcrSecretName},
			},
		},
	},

	Queue: fmt.Sprintf("modules/%s", common.MODULE_NAME),
}

var _ = registry.RegisterFunc(config, GenerateSecret)

func generateAlphaNum(n int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	charLen := big.NewInt(int64(len(chars)))
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, charLen)
		if err != nil {
			return "", err
		}
		b[i] = chars[idx.Int64()]
	}
	return string(b), nil
}

func ValidateHtpasswdWithSalt(storedLine, password, salt string) bool {
	saltedPassword := password + salt
	parts := strings.SplitN(storedLine, ":", 2)
	if len(parts) != 2 {
		return false
	}
	hash := parts[1]
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(saltedPassword)) == nil
}

func GenerateSecret(_ context.Context, input *pkg.HookInput) error {
	input.Logger.Info("🟥🟧🟥")
	secrets := input.Snapshots.Get(dvcrSecretSnapshotName)

	var secretData dvcrSecretData

	if len(secrets) == 0 {
		// generate PasswordRW
		PasswordRW, err := generateAlphaNum(32)
		if err != nil {
			return errors.Wrap(err, "generate alpha num")
		}

		input.Values.Set(dvcrHtpasswdPath, base64.StdEncoding.EncodeToString([]byte(PasswordRW)))

		// generate Salt
		Salt, err := generateAlphaNum(40)
		if err != nil {
			return errors.Wrap(err, "generate alpha num")
		}
		input.Values.Set(dvcrSaltPath, base64.StdEncoding.EncodeToString([]byte(Salt)))

		// generate htpasswd
		saltedPassword := PasswordRW + Salt
		hashBytes, err := bcrypt.GenerateFromPassword([]byte(saltedPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		input.Logger.Info(string(hashBytes))
		// input.Values.Set(dvcrSaltPath, base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", dvcrUser, string(hashBytes)))))

		return nil
	}

	err := secrets[0].UnmarhalTo(&secretData)
	if err != nil {
		input.Logger.Error("Unable to unmarshal secret" + err.Error())
	}

	password, _ := secretData.GetPasswordRW()
	salt, _ := secretData.GetSalt()
	htpasswd, _ := secretData.GetHtpasswd()

	if !ValidateHtpasswdWithSalt(htpasswd, password, salt) {

		saltedPassword := password + salt
		hashBytes, err := bcrypt.GenerateFromPassword([]byte(saltedPassword), bcrypt.DefaultCost)
		if err != nil {
			input.Logger.Log("⛔ hehehehe")
			return err
		}
		input.Logger.Info(string(hashBytes))
		//input.Values.Set(dvcrSaltPath, base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", dvcrUser, string(hashBytes)))))
	}

	// input.Values.Set(dvcrHtpasswdPath, secretData.Data.Htpasswd)
	// input.Values.Set(dvcrPasswordRWPath, secretData.Data.PasswordRW)
	// input.Values.Set(dvcrSaltPath, secretData.Data.Salt)

	return nil
}

func main() {
	app.Run()
}
