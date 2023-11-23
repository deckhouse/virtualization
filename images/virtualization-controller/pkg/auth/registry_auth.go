package auth

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/distribution/reference"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
)

// Registry auth methods from dvcr-importer.

// ReadDockerConfigJSON creates ConfigFile from bytes, e.g. from the Secret.data.
func ReadDockerConfigJSON(dockerconfigjson []byte) (*configfile.ConfigFile, error) {
	config, err := config.LoadFromReader(bytes.NewReader(dockerconfigjson))
	if err != nil {
		return nil, fmt.Errorf("loading auth config: %w", err)
	}

	return config, nil
}

// CredsFromRegistryAuthFile returns username and password for the registry related to ref.
// It returns auth for the first registry if ref is empty.
func CredsFromRegistryAuthFile(configFile *configfile.ConfigFile, ref string) (string, string, error) {
	var authConfig types.AuthConfig

	if ref == "" {
		// Get credentials for the first entry.
		auths, err := configFile.GetAllCredentials()
		if err != nil {
			return "", "", err
		}
		for _, v := range auths {
			authConfig = v
			break
		}
	} else {
		namedRef, err := reference.ParseNormalizedNamed(ref)
		if err != nil {
			return "", "", fmt.Errorf("parsing reference: %w", err)
		}

		host := reference.Domain(namedRef)
		if host == "docker.io" {
			host = "https://index.docker.io/v1/"
		}

		authConfig, err = configFile.GetAuthConfig(host)
		if err != nil {
			return "", "", fmt.Errorf("get auth config for %s: %w", host, err)
		}
	}

	var username, password string
	var err error
	switch {
	case authConfig.IdentityToken != "":
		return "", "", fmt.Errorf("identity token not supported")
	case authConfig.Auth != "":
		username, password, err = decodeAuth(authConfig.Auth)
		if err != nil {
			return "", "", fmt.Errorf("decode auth config: %w", err)
		}
	default:
		username = authConfig.Username
		password = authConfig.Password
	}

	return username, password, nil
}

// decodeAuth extracts username and password from the base64 encoded string.
func decodeAuth(authStr string) (string, string, error) {
	decLen := base64.StdEncoding.DecodedLen(len(authStr))
	decoded := make([]byte, decLen)
	authByte := []byte(authStr)
	n, err := base64.StdEncoding.Decode(decoded, authByte)
	if err != nil {
		return "", "", err
	}
	if n > decLen {
		return "", "", fmt.Errorf("something went wrong decoding auth config")
	}

	userName, password, ok := strings.Cut(string(decoded), ":")
	if !ok || userName == "" {
		return "", "", fmt.Errorf("invalid auth configuration format, should be username:password")
	}

	return userName, strings.Trim(password, "\x00"), nil
}
