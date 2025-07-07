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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/auth"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
)

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
