/*
Copyright 2026 Flant JSC

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

package service

import (
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
)

// importerTokenScope is the DVCR access an importer Pod needs: push+pull on its
// destination repository, plus pull on the source repository when the source is
// itself a DVCR image (dvcr-artifact copies both through the same credential).
// Returns nil when per-namespace authorization is off (no token is minted).
func importerTokenScope(s *dvcr.Settings, settings *importer.Settings) []registrytoken.Access {
	if !s.TenantAuthzEnabled {
		return nil
	}
	access := []registrytoken.Access{repoAccess(s.RepoPath(settings.DestinationEndpoint), "pull", "push")}
	if settings.Source == importer.SourceDVCR && settings.Endpoint != "" {
		access = append(access, repoAccess(s.RepoPath(settings.Endpoint), "pull"))
	}
	return access
}

// uploaderTokenScope is the DVCR access an uploader Pod needs: push+pull on its
// destination repository (an uploader has no DVCR source).
func uploaderTokenScope(s *dvcr.Settings, settings *uploader.Settings) []registrytoken.Access {
	if !s.TenantAuthzEnabled {
		return nil
	}
	return []registrytoken.Access{repoAccess(s.RepoPath(settings.DestinationEndpoint), "pull", "push")}
}

// dataVolumeTokenScope is the DVCR access a CDI DataVolume needs: pull-only on the
// source repository it imports from (it writes to a PVC, not to DVCR).
func dataVolumeTokenScope(s *dvcr.Settings, source *cdiv1.DataVolumeSource) []registrytoken.Access {
	if !s.TenantAuthzEnabled || source == nil || source.Registry == nil || source.Registry.URL == nil {
		return nil
	}
	return []registrytoken.Access{repoAccess(s.RepoPath(*source.Registry.URL), "pull")}
}

func repoAccess(name string, actions ...string) registrytoken.Access {
	return registrytoken.Access{Type: "repository", Name: name, Actions: actions}
}
