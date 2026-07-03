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

func TestAuthorize(t *testing.T) {
	tenantA := Subject{Role: RoleTenant, Namespace: "nsA"}

	tests := []struct {
		name     string
		subject  Subject
		accesses []Access
		want     bool
	}{
		// --- tenant: own namespace ---
		{"tenant pull own vi", tenantA, []Access{repo("vi/nsA/img", "pull")}, true},
		{"tenant push own vi", tenantA, []Access{repo("vi/nsA/img", "push")}, true},
		{"tenant pull own vd", tenantA, []Access{repo("vd/nsA/disk", "pull")}, true},
		{"tenant push own vd", tenantA, []Access{repo("vd/nsA/disk", "push")}, true},
		{"tenant delete own vi denied", tenantA, []Access{repo("vi/nsA/img", "delete")}, false},

		// --- tenant: cross-namespace (the core vulnerability) ---
		{"tenant pull other-ns vi denied", tenantA, []Access{repo("vi/nsB/img", "pull")}, false},
		{"tenant push other-ns vi denied", tenantA, []Access{repo("vi/nsB/img", "push")}, false},
		{"tenant push other-ns vd denied", tenantA, []Access{repo("vd/nsB/disk", "push")}, false},

		// --- tenant: cvi (cluster, shared read-only) ---
		{"tenant pull cvi ok", tenantA, []Access{repo("cvi/ubuntu", "pull")}, true},
		{"tenant push cvi denied", tenantA, []Access{repo("cvi/ubuntu", "push")}, false},

		// --- tenant: catalog / registry enumeration ---
		{"tenant catalog denied", tenantA, []Access{{Type: "registry", Name: "catalog", Action: "*"}}, false},

		// --- tenant: path normalization / prefix confusion ---
		{"tenant prefix confusion nsA-evil denied", tenantA, []Access{repo("vi/nsA-evil/img", "push")}, false},
		{"tenant traversal to other ns denied", tenantA, []Access{repo("vi/nsA/../nsB/img", "push")}, false},
		{"tenant traversal escape denied", tenantA, []Access{repo("vi/nsA/../../etc/x", "push")}, false},
		{"tenant unknown prefix denied", tenantA, []Access{repo("foo/nsA/img", "push")}, false},
		{"tenant bare ns no name denied", tenantA, []Access{repo("vi/nsA", "push")}, false},

		// --- tenant: cross-repo blob mount (distribution issues a pull on the from-repo) ---
		{"tenant mount from own ns ok", tenantA, []Access{
			repo("vi/nsA/dst", "push"),
			repo("vi/nsA/src", "pull"),
		}, true},
		{"tenant mount from other ns denied", tenantA, []Access{
			repo("vi/nsA/dst", "push"),
			repo("vi/nsB/src", "pull"),
		}, false},

		// --- tenant: empty namespace is never valid ---
		{"tenant empty ns denied", Subject{Role: RoleTenant, Namespace: ""}, []Access{repo("vi//img", "pull")}, false},

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
		{"none denied", Subject{Role: RoleNone}, []Access{repo("vi/nsA/img", "pull")}, false},

		// --- mixed list: one denied access denies all ---
		{"tenant mixed one bad denies all", tenantA, []Access{
			repo("vi/nsA/ok", "pull"),
			repo("vi/nsB/bad", "pull"),
		}, false},

		// --- empty access list is allowed (capability probe) ---
		{"empty access allowed", tenantA, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Authorize(tt.subject, tt.accesses); got != tt.want {
				t.Errorf("Authorize(%+v, %+v) = %v, want %v", tt.subject, tt.accesses, got, tt.want)
			}
		})
	}
}

func TestNamespaceFromUsername(t *testing.T) {
	tests := []struct {
		username string
		want     string
	}{
		{"system:serviceaccount:nsA:importer", "nsA"},
		{"system:serviceaccount:d8-virtualization:dvcr", "d8-virtualization"},
		{"system:serviceaccount::name", ""},   // empty namespace
		{"system:serviceaccount:nsA", ""},      // no name separator
		{"system:node:worker-1", ""},           // not a service account
		{"kubernetes-admin", ""},               // human user
		{"", ""},                               // empty
	}
	for _, tt := range tests {
		if got := namespaceFromUsername(tt.username); got != tt.want {
			t.Errorf("namespaceFromUsername(%q) = %q, want %q", tt.username, got, tt.want)
		}
	}
}
