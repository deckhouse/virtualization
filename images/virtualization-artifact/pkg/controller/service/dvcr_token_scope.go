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
	"github.com/deckhouse/virtualization-controller/pkg/controller/importer"
	"github.com/deckhouse/virtualization-controller/pkg/controller/uploader"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
)

// importerTokenScope is the DVCR access an importer Pod needs: push+pull on its
// destination repository, plus pull on the source repository when the source is
// itself a DVCR image (dvcr-artifact copies both through the same credential).
func importerTokenScope(s *dvcr.Settings, settings *importer.Settings) []registrytoken.Access {
	access := []registrytoken.Access{repoAccess(s.RepoPath(settings.DestinationEndpoint), "pull", "push")}
	if settings.Source == importer.SourceDVCR && settings.Endpoint != "" {
		access = append(access, repoAccess(s.RepoPath(settings.Endpoint), "pull"))
	}
	return access
}

// uploaderTokenScope is the DVCR access an uploader Pod needs: push+pull on its
// destination repository (an uploader has no DVCR source).
func uploaderTokenScope(s *dvcr.Settings, settings *uploader.Settings) []registrytoken.Access {
	return []registrytoken.Access{repoAccess(s.RepoPath(settings.DestinationEndpoint), "pull", "push")}
}

func repoAccess(name string, actions ...string) registrytoken.Access {
	return registrytoken.Access{Type: "repository", Name: name, Actions: actions}
}
