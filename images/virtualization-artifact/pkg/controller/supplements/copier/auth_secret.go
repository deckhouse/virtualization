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

package copier

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/auth"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
)

// ScopedTokenUsername is the Basic-auth username presented alongside a scoped
// DVCR token. The registry authorizes on the token in the password; the username
// is a fixed, human-readable marker.
const ScopedTokenUsername = "dvcr-jwt"

// AuthSecret copies auth credentials from the source Secret into
// Destination Secret and ensure its data is CDI compatible:
// type: Opaque
// data:
//
//	accessKeyId: ""  # <optional: your key or username, base64 encoded>
//	secretKey:   "" # <optional: your secret or password, base64 encoded>
//
// Additionally OwnerRef, Annotations, and Labels may be passed.
type AuthSecret struct {
	Secret
}

// CreateScopedTokenCDI mints a scoped DVCR token for access and stores it in a
// CDI-compatible Opaque Secret (accessKeyId/secretKey). The importer Pod
// authenticates to DVCR with a credential scoped to exactly the repositories in
// access, instead of the shared read-write credential.
func (a AuthSecret) CreateScopedTokenCDI(ctx context.Context, client client.Client, signer *registrytoken.Signer, access []registrytoken.Access) error {
	raw, err := signer.Sign(access, time.Now())
	if err != nil {
		return fmt.Errorf("mint scoped DVCR token: %w", err)
	}
	destData := map[string][]byte{
		"accessKeyId": []byte(ScopedTokenUsername),
		"secretKey":   []byte(raw),
	}
	_, err = a.Create(ctx, client, destData, corev1.SecretTypeOpaque)
	return err
}

// CreateScopedTokenDockerConfig mints a scoped DVCR token for access and stores
// it as a dockerconfigjson Secret keyed by registryURL, so the dvcr-artifact
// importer/uploader Pod reads it through the standard destination auth config.
func (a AuthSecret) CreateScopedTokenDockerConfig(ctx context.Context, client client.Client, signer *registrytoken.Signer, access []registrytoken.Access, registryURL string) error {
	raw, err := signer.Sign(access, time.Now())
	if err != nil {
		return fmt.Errorf("mint scoped DVCR token: %w", err)
	}
	cfg, err := dockerConfigJSON(registryURL, ScopedTokenUsername, raw)
	if err != nil {
		return err
	}
	destData := map[string][]byte{corev1.DockerConfigJsonKey: cfg}
	_, err = a.Create(ctx, client, destData, corev1.SecretTypeDockerConfigJson)
	return err
}

// dockerConfigJSON builds a minimal ~/.docker/config.json for a single registry.
func dockerConfigJSON(registryURL, username, password string) ([]byte, error) {
	entry := map[string]string{
		"username": username,
		"password": password,
		"auth":     base64.StdEncoding.EncodeToString([]byte(username + ":" + password)),
	}
	return json.Marshal(map[string]any{
		"auths": map[string]any{registryURL: entry},
	})
}

// CopyCDICompatible transforms auth credentials in dockerconfigjson format into CDI compatible:
// a Secret with two fields: accessKeyId and secretKey.
// ref is registry url or image name. It is used to select a desired auth pair from the config.
func (a AuthSecret) CopyCDICompatible(ctx context.Context, client client.Client, ref string) error {
	srcObj, err := object.FetchObject(ctx, a.Source, client, &corev1.Secret{})
	if err != nil {
		return err
	}

	destData := srcObj.Data
	destType := srcObj.Type
	if srcObj.Type == corev1.SecretTypeDockerConfigJson {
		cfg, err := auth.ReadDockerConfigJSON(srcObj.Data[corev1.DockerConfigJsonKey])
		if err != nil {
			return err
		}
		username, password, err := auth.CredsFromRegistryAuthFile(cfg, ref)
		if err != nil {
			return err
		}
		destData = map[string][]byte{
			"accessKeyId": []byte(username),
			"secretKey":   []byte(password),
		}
		destType = corev1.SecretTypeOpaque
	}

	_, err = a.Create(ctx, client, destData, destType)
	return err
}
