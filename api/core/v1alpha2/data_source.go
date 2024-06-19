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

package v1alpha2

type DataSourceHTTP struct {
	URL      string    `json:"url"`
	CABundle []byte    `json:"caBundle"`
	Checksum *Checksum `json:"checksum,omitempty"`
}

type DataSourceContainerRegistry struct {
	Image           string          `json:"image"`
	ImagePullSecret ImagePullSecret `json:"imagePullSecret"`
	CABundle        []byte          `json:"caBundle"`
}

type ImagePullSecret struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type Checksum struct {
	MD5    string `json:"md5,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type DataSourceType string

const (
	DataSourceTypeHTTP           DataSourceType = "HTTP"
	DataSourceTypeContainerImage DataSourceType = "ContainerImage"
	DataSourceTypeObjectRef      DataSourceType = "ObjectRef"
	DataSourceTypeUpload         DataSourceType = "Upload"
)
