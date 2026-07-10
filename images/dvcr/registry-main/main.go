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

// Command dvcr-registry is the distribution registry entrypoint for DVCR. It is
// upstream cmd/registry plus a blank import of the dvcr-k8s auth plugin, so the
// same binary serves both htpasswd (feature flag off) and dvcr-k8s multi-tenant
// authorization (flag on). Built into a binary named "registry" so the existing
// consumers (dvcr-cleaner's `registry garbage-collect`, the final image) are
// unchanged. This file is copied into the distribution module at build time.
package main

import (
	_ "net/http/pprof"

	"github.com/distribution/distribution/v3/registry"
	_ "github.com/distribution/distribution/v3/registry/auth/dvcrk8s"
	_ "github.com/distribution/distribution/v3/registry/auth/htpasswd"
	_ "github.com/distribution/distribution/v3/registry/auth/silly"
	_ "github.com/distribution/distribution/v3/registry/auth/token"
	_ "github.com/distribution/distribution/v3/registry/proxy"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/azure"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/gcs"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/middleware/cloudfront"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/middleware/redirect"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/middleware/rewrite"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/s3-aws"
)

func main() {
	// nolint:errcheck // RootCmd halts the program on failure of its subcommands.
	registry.RootCmd.Execute()
}
