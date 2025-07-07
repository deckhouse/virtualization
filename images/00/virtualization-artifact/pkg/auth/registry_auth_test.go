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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCredsFromRegistryAuthFile(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		address  string
		ref      string
	}{
		{
			"standalone dvcr",
			"admin",
			"pass",
			"registry.dvcr.svc.cluster.local",
			"registry.dvcr.svc.cluster.local/dvcr",
		},
		{
			"in-module dvcr",
			"admin",
			"pass",
			"dvcr.d8-virtualization.svc",
			"dvcr.d8-virtualization.svc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ReadDockerConfigJSON(mustEncodeDockerConfigJSON(tt.username, tt.password, tt.address))
			require.NoError(t, err, "should read config from string")

			authUsername, authPassword, err := CredsFromRegistryAuthFile(cfg, tt.ref)
			require.NoError(t, err, "should find config for registry url")

			require.Equal(t, tt.username, authUsername)
			require.Equal(t, tt.password, authPassword)
		})
	}
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
