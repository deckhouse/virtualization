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

// Package dvcrk8s implements the DVCR multi-tenant authorization policy.
//
// This file holds the pure, dependency-free authorization decision logic so it
// can be unit-tested without the distribution registry runtime. The distribution
// auth.AccessController glue (Basic parsing, JWT verification) lives in access.go
// and only translates registry types into the Subject/Access values decided here.
package dvcrk8s

import (
	"path"
	"strings"
)

// Role is the authorization role derived from the presented credential.
type Role int

const (
	// RoleNone denies everything (fail-closed default).
	RoleNone Role = iota
	// RoleAdmin grants full access. Presented by the virtualization-controller
	// with the static read-write password.
	RoleAdmin
	// RolePuller grants pull-only access to any repository. Presented by node
	// containerd with the static node-puller password.
	RolePuller
	// RoleScoped grants exactly the access carried in the credential. Presented by
	// importer/uploader Pods with a signed JWT the controller minted for the single
	// repository that Pod reads from / writes to.
	RoleScoped
)

// Subject is the authenticated caller together with its authorization scope.
type Subject struct {
	Role Role
	// Grants is the set of allowed repository actions carried by a RoleScoped
	// credential (the JWT's access claim). Ignored for other roles.
	Grants []Grant
}

// Access mirrors the subset of distribution's auth.Access needed for a single
// authorization decision (one requested resource + action).
type Access struct {
	// Type is the resource type: "repository" or "registry".
	Type string
	// Name is the repository path without tag (e.g. "vi/ns/name"), or "catalog"
	// for the registry-wide catalog resource.
	Name string
	// Action is "pull", "push", "delete" or "*".
	Action string
}

// Grant is one entry of a RoleScoped credential's access claim: a resource with
// the set of actions permitted on it. Mirrors distribution's token ResourceActions.
type Grant struct {
	Type    string
	Name    string
	Actions []string
}

// Authorize returns true only if the subject is allowed every requested access.
// A single denied access denies the whole request (fail-closed). An empty access
// list is allowed (distribution uses it for unauthenticated capability probes).
func Authorize(s Subject, accesses []Access) bool {
	for i := range accesses {
		if !authorizeOne(s, accesses[i]) {
			return false
		}
	}
	return true
}

func authorizeOne(s Subject, a Access) bool {
	switch s.Role {
	case RoleAdmin:
		return true
	case RolePuller:
		// Nodes only ever pull image layers; never push or delete, never enumerate.
		return a.Type == "repository" && a.Action == "pull"
	case RoleScoped:
		return grantsCover(s.Grants, a)
	default:
		return false
	}
}

// grantsCover reports whether any grant permits the requested access. Names are
// path-cleaned on both sides so trailing slashes or "." segments cannot smuggle a
// mismatch past an exact string compare.
func grantsCover(grants []Grant, a Access) bool {
	name := cleanName(a.Name)
	for i := range grants {
		g := grants[i]
		if g.Type != a.Type || cleanName(g.Name) != name {
			continue
		}
		for _, act := range g.Actions {
			if act == a.Action || act == "*" {
				return true
			}
		}
	}
	return false
}

// cleanName normalizes a resource name by anchoring path.Clean at root, so
// "cvi/foo/", "cvi/foo" and "cvi/./foo" compare equal and no "../" can escape.
func cleanName(name string) string {
	return strings.TrimPrefix(path.Clean("/"+name), "/")
}
