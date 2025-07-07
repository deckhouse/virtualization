/*
Copyright 2024 Flant JSC

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
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
)

func RegistryAuthFile(path string) (*configfile.ConfigFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("error opening auth config file: %w", err)
	}
	defer f.Close()

	config, err := config.LoadFromReader(f)
	if err != nil {
		return nil, fmt.Errorf("error loading auth config: %w", err)
	}

	return config, nil
}

func CredsFromRegistryAuthFile(configFile *configfile.ConfigFile, ref string) (string, string, error) {
	namedRef, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return "", "", fmt.Errorf("error parsing reference: %w", err)
	}

	host := reference.Domain(namedRef)
	if host == "docker.io" {
		host = "https://index.docker.io/v1/"
	}

	authConfig, err := configFile.GetAuthConfig(host)
	if err != nil {
		return "", "", fmt.Errorf("error getting auth config: %w", err)
	}

	var username, password string
	switch {
	case authConfig.IdentityToken != "":
		return "", "", fmt.Errorf("identity token not supported")
	case authConfig.Auth != "":
		username, password, err = decodeAuth(authConfig.Auth)
		if err != nil {
			return "", "", fmt.Errorf("error decoding auth config: %w", err)
		}
	default:
		username = authConfig.Username
		password = authConfig.Password
	}

	return username, password, nil
}

// decodeAuth decodes a base64 encoded string and returns username and password
func decodeAuth(authStr string) (string, string, error) {
	decLen := base64.StdEncoding.DecodedLen(len(authStr))
	decoded := make([]byte, decLen)
	authByte := []byte(authStr)
	n, err := base64.StdEncoding.Decode(decoded, authByte)
	if err != nil {
		return "", "", err
	}
	if n > decLen {
		return "", "", fmt.Errorf("Something went wrong decoding auth config")
	}

	userName, password, ok := strings.Cut(string(decoded), ":")
	if !ok || userName == "" {
		return "", "", fmt.Errorf("Invalid auth configuration file")
	}

	return userName, strings.Trim(password, "\x00"), nil
}
