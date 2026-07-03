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

// Plain stdlib table tests (not ginkgo): this package is dependency-free on
// purpose so it can be compiled into the upstream distribution binary without
// dragging test frameworks into that build.
package dvcrk8s

import "testing"

func repo(name, action string) Access {
	return Access{Type: "repository", Name: name, Action: action}
}

func grantRepo(name string, actions ...string) Grant {
	return Grant{Type: "repository", Name: name, Actions: actions}
}

func TestAuthorize(t *testing.T) {
	// A Pod scoped to its own cvi repository (the CVI-from-PVC case).
	scopedCVI := Subject{Role: RoleScoped, Grants: []Grant{grantRepo("cvi/my-image", "pull", "push")}}
	// A Pod scoped to a namespaced vd plus a DVCR source it pulls from.
	scopedVD := Subject{Role: RoleScoped, Grants: []Grant{
		grantRepo("vd/nsA/disk", "pull", "push"),
		grantRepo("cvi/base", "pull"),
	}}

	tests := []struct {
		name     string
		subject  Subject
		accesses []Access
		want     bool
	}{
		// --- scoped: exactly the granted repo/action ---
		{"scoped push own cvi", scopedCVI, []Access{repo("cvi/my-image", "push")}, true},
		{"scoped pull own cvi", scopedCVI, []Access{repo("cvi/my-image", "pull")}, true},
		{"scoped delete own cvi denied", scopedCVI, []Access{repo("cvi/my-image", "delete")}, false},

		// --- scoped: any other repository is denied (the core guarantee) ---
		{"scoped push other cvi denied", scopedCVI, []Access{repo("cvi/other", "push")}, false},
		{"scoped push tenant repo denied", scopedCVI, []Access{repo("vd/nsB/disk", "push")}, false},
		{"scoped prefix confusion denied", scopedCVI, []Access{repo("cvi/my-image-evil", "push")}, false},
		{"scoped traversal denied", scopedCVI, []Access{repo("cvi/my-image/../other", "push")}, false},
		{"scoped catalog denied", scopedCVI, []Access{{Type: "registry", Name: "catalog", Action: "*"}}, false},

		// --- scoped: name normalization compares equal ---
		{"scoped trailing slash ok", scopedCVI, []Access{repo("cvi/my-image/", "push")}, true},

		// --- scoped: destination push + source pull (cross-repo, both granted) ---
		{"scoped push dst pull src ok", scopedVD, []Access{
			repo("vd/nsA/disk", "push"),
			repo("cvi/base", "pull"),
		}, true},
		{"scoped push src denied", scopedVD, []Access{repo("cvi/base", "push")}, false},
		{"scoped mixed one bad denies all", scopedVD, []Access{
			repo("vd/nsA/disk", "push"),
			repo("cvi/other", "pull"),
		}, false},

		// --- puller (nodes) ---
		{"puller pull any ok", Subject{Role: RolePuller}, []Access{repo("vi/nsB/img", "pull")}, true},
		{"puller pull cvi ok", Subject{Role: RolePuller}, []Access{repo("cvi/ubuntu", "pull")}, true},
		{"puller push denied", Subject{Role: RolePuller}, []Access{repo("vi/nsB/img", "push")}, false},
		{"puller delete denied", Subject{Role: RolePuller}, []Access{repo("vi/nsB/img", "delete")}, false},
		{"puller catalog denied", Subject{Role: RolePuller}, []Access{{Type: "registry", Name: "catalog", Action: "*"}}, false},

		// --- admin (controller) ---
		{"admin all repo ok", Subject{Role: RoleAdmin}, []Access{repo("vi/nsB/img", "push"), repo("cvi/x", "delete")}, true},
		{"admin catalog ok", Subject{Role: RoleAdmin}, []Access{{Type: "registry", Name: "catalog", Action: "*"}}, true},

		// --- fail-closed default ---
		{"none denied", Subject{Role: RoleNone}, []Access{repo("cvi/x", "pull")}, false},
		{"scoped no grants denied", Subject{Role: RoleScoped}, []Access{repo("cvi/x", "pull")}, false},

		// --- empty access list is allowed (capability probe) ---
		{"empty access allowed", scopedCVI, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Authorize(tt.subject, tt.accesses); got != tt.want {
				t.Errorf("Authorize(%+v, %+v) = %v, want %v", tt.subject, tt.accesses, got, tt.want)
			}
		})
	}
}
