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
// auth.AccessController glue (request parsing, TokenReview) lives in access.go
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
	// RoleAdmin grants full access. Used by the virtualization-controller.
	RoleAdmin
	// RolePuller grants pull-only access to any repository. Used by node containerd.
	RolePuller
	// RoleTenant grants namespace-scoped access. Used by importer/uploader Pods.
	RoleTenant
)

// Subject is the authenticated caller together with its authorization scope.
type Subject struct {
	Role Role
	// Namespace is the tenant namespace; only meaningful for RoleTenant.
	Namespace string
}

// Access mirrors the subset of distribution's auth.Access needed for a decision.
type Access struct {
	// Type is the resource type: "repository" or "registry".
	Type string
	// Name is the repository path without tag (e.g. "vi/ns/name"), or "catalog"
	// for the registry-wide catalog resource.
	Name string
	// Action is "pull", "push", "delete" or "*".
	Action string
}

// Repository path prefixes, kept in sync with pkg/dvcr/dvcr.go image templates:
//
//	cvi/<name>          (ClusterVirtualImage, cluster-scoped, shared read-only)
//	vi/<ns>/<name>      (VirtualImage, namespaced)
//	vd/<ns>/<name>      (VirtualDisk, namespaced)
const (
	prefixCVI = "cvi"
	prefixVI  = "vi"
	prefixVD  = "vd"
)

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
	case RoleTenant:
		return authorizeTenant(s.Namespace, a)
	default:
		return false
	}
}

func authorizeTenant(ns string, a Access) bool {
	// Deny the registry-wide catalog and any non-repository resource: it would
	// enumerate every tenant's images.
	if a.Type != "repository" {
		return false
	}
	// Empty namespace can never match a repository segment; deny.
	if ns == "" {
		return false
	}

	seg := splitClean(a.Name)
	switch {
	case len(seg) == 3 && (seg[0] == prefixVI || seg[0] == prefixVD) && seg[1] == ns:
		// Own-namespace VirtualImage / VirtualDisk: read and write.
		return a.Action == "pull" || a.Action == "push"
	case len(seg) >= 2 && seg[0] == prefixCVI:
		// Cluster images are shared; tenants may read them as disk/image sources,
		// but only the controller (RoleAdmin) creates them.
		// ponytail: pull-only for tenants; if a future flow pushes cvi from a
		// tenant Pod, grant push here and cover it with an e2e test.
		return a.Action == "pull"
	default:
		return false
	}
}

// splitClean normalizes a repository name into path segments, neutralizing any
// "." / ".." traversal by anchoring the clean at root. Returns nil for an empty
// or root-only path. Using path.Clean (not HasPrefix) is what prevents a name
// like "vi/nsA-evil" or "vi/nsA/../nsB/x" from being mistaken for namespace nsA.
func splitClean(name string) []string {
	cleaned := strings.TrimPrefix(path.Clean("/"+name), "/")
	if cleaned == "" || cleaned == "." {
		return nil
	}
	return strings.Split(cleaned, "/")
}
