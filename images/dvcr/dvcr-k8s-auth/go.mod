// Local module for unit-testing the DVCR authorization code. The requires below
// exist only for access_test.go (build tag dvcr_registry) to verify tokens
// against the real distribution backend. The .go sources are copied into the
// distribution module at build time (see images/dvcr/werf.inc.yaml); this go.mod
// and the *_test.go files are not used in that build.
module github.com/deckhouse/virtualization/dvcr-k8s-auth

go 1.25.0

require (
	github.com/distribution/distribution/v3 v3.1.1
	github.com/go-jose/go-jose/v4 v4.1.4
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/onsi/ginkgo/v2 v2.23.3
	github.com/onsi/gomega v1.37.0
)

require (
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20241210010833-40e02aabc2ad // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.39.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
