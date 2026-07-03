// Local module for standalone unit-testing of the DVCR authorization code.
// policy_test.go tests the dependency-free policy without build tags; the
// distribution/go-jose/golang-jwt requires below exist only so access_test.go
// (build tag dvcr_registry) can exercise the JWT verifier against the real
// distribution token backend. The .go sources are copied into the distribution
// registry module at build time (see images/dvcr/werf.inc.yaml); this go.mod and
// the *_test.go files are not used in that build.
module github.com/deckhouse/virtualization/dvcr-k8s-auth

go 1.25.0

require (
	github.com/distribution/distribution/v3 v3.1.1
	github.com/go-jose/go-jose/v4 v4.1.4
	github.com/golang-jwt/jwt/v5 v5.3.0
)

require (
	github.com/sirupsen/logrus v1.9.4 // indirect
	golang.org/x/sys v0.42.0 // indirect
)
