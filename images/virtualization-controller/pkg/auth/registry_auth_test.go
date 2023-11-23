package auth

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_get_creds(t *testing.T) {
	authUsername := "admin"
	authPassword := "pass"
	authAddress := "registry.dvcr.svc.cluster.local"

	cfg, err := ReadDockerConfigJSON(mustEncodeDockerConfigJSON(authUsername, authPassword, authAddress))
	require.NoError(t, err, "should read config from string")

	// Use registry URL as ref.
	ref := "registry.dvcr.svc.cluster.local/dvcr"

	username, password, err := CredsFromRegistryAuthFile(cfg, ref)
	require.NoError(t, err, "should find config for registry url")

	require.Equal(t, authUsername, username)
	require.Equal(t, authPassword, password)
}

// mustEncodeDockerConfigJSON returns
//
//	{"auths":{
//	   "registry.dvcr.svc.cluster.local":{
//	     "username":"admin",
//	     "password":"p4ssw0rd",
//	     "auth": BASE64() "admin:p4ssw0rd"
//	    }
//	}}
func mustEncodeDockerConfigJSON(username, password, registryAddress string) []byte {
	auth := map[string]interface{}{
		"auths": map[string]interface{}{
			registryAddress: map[string]interface{}{
				"username": username,
				"password": password,
				"auth":     base64.StdEncoding.EncodeToString([]byte(username + ":" + password)),
			},
		},
	}
	authBytes, _ := json.Marshal(auth)
	return authBytes
}
