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

package webhook

import (
	"context"
	"slices"

	admissionv1 "k8s.io/api/admission/v1"
	"kubevirt.io/api/clone"
	virtcore "kubevirt.io/api/core"
	"kubevirt.io/api/export"
	"kubevirt.io/api/instancetype"
	"kubevirt.io/api/migrations"
	"kubevirt.io/api/pool"
	"kubevirt.io/api/snapshot"
	cdicore "kubevirt.io/containerized-data-importer-api/pkg/apis/core"
	"kubevirt.io/containerized-data-importer-api/pkg/apis/forklift"
	"kubevirt.io/containerized-data-importer-api/pkg/apis/upload"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const ProtectResourcesPath = "/protect-resources"

var defaultProtectGroups = []string{
	virtcore.GroupName,
	clone.GroupName,
	export.GroupName,
	instancetype.GroupName,
	migrations.GroupName,
	pool.GroupName,
	snapshot.GroupName,
	cdicore.GroupName,
	upload.GroupName,
	forklift.GroupName,
}

func newProtectHook(allowSA []string, groups []string) *protectHook {
	return &protectHook{
		allowSA: allowSA,
		groups:  groups,
		operations: []admissionv1.Operation{
			admissionv1.Create,
			admissionv1.Update,
			admissionv1.Delete,
		},
	}
}

type protectHook struct {
	allowSA    []string
	groups     []string
	operations []admissionv1.Operation
}

func (p protectHook) Handle(_ context.Context, req admission.Request) admission.Response {
	if slices.Contains(p.groups, req.Resource.Group) &&
		!slices.Contains(p.allowSA, req.UserInfo.Username) &&
		slices.Contains(p.operations, req.Operation) {
		return admission.Denied("Operation forbidden for this service account.")
	}

	return admission.Allowed("")
}
