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

// In-package test for the dvcrk8s authorization plugin of the distribution
// fork: it lives here and the dvcr-fuzz image places it into the cloned fork
// tree. Keep it in package dvcrk8s and import nothing the fork does not vendor.
package dvcrk8s

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	// The code decodes the claim set with go-jose's json fork, which matches
	// field names case sensitively; the standard library does not.
	josejson "github.com/go-jose/go-jose/v4/json"
	logrus "github.com/sirupsen/logrus"

	"github.com/distribution/distribution/v3/registry/auth/token"
)

const (
	fuzzMaxPart = 4 << 10

	fuzzMaxEntries = 64

	// What the registry config configures; the fuzzer gets to guess at these.
	fuzzAdminUser   = "dvcr-rw"
	fuzzAdminPass   = "admin-password-value"
	fuzzPullerUser  = "dvcr-node"
	fuzzPullerPass  = "puller-password-value"
	fuzzJWTIssuer   = "virtualization-controller"
	fuzzJWTAudience = "dvcr"
	fuzzKeyID       = "dvcr"
)

// fuzzSigningKey signs scoped tokens; its public half goes into trustedKeys, so
// the fuzzer's input gets past the key lookup and reaches the claim set.
var fuzzSigningKey = func() *ecdsa.PrivateKey {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	return key
}()

var fuzzSigner = func() jose.Signer {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.ES256, Key: fuzzSigningKey},
		(&jose.SignerOptions{}).WithHeader("kid", fuzzKeyID),
	)
	if err != nil {
		panic(err)
	}

	return signer
}()

// FuzzAuthorize drives the authorization decision of the DVCR registry plugin:
// Authorize and, under it, authorizeOne, grantsCover and cleanName. The role
// follows from the credential presented, the grants are the access claim of a
// scoped token minted per importer Pod, and the accesses are what the client
// asked for.
//
// Name normalization clamps at the root, as the plugin documents: "../../etc"
// denotes "etc", and "", "." and "/" denote the same empty name. The
// expectation clamps the same way.
func FuzzAuthorize(f *testing.F) {
	f.Add(uint8(RoleAdmin), "", "repository|vi/ns/name|push")
	f.Add(uint8(RolePuller), "", "repository|vi/ns/name|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/name|pull,push", "repository|vi/ns/name|pull")
	f.Add(uint8(RoleNone), "repository|vi/ns/name|pull", "repository|vi/ns/name|pull")
	// The node account must stay read-only.
	f.Add(uint8(RolePuller), "", "repository|vi/ns/name|push")
	f.Add(uint8(RolePuller), "", "repository|vi/ns/name|delete")
	f.Add(uint8(RolePuller), "", "repository|vi/ns/name|*")
	f.Add(uint8(RolePuller), "", "registry|catalog|*")
	f.Add(uint8(RolePuller), "", "repository|vi/ns/name|pull;repository|vi/ns/other|push")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|vi/ns/b|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|vi/ns/a|push")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "registry|catalog|*")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|vi/ns/a|pull;repository|vi/ns/b|pull")
	f.Add(uint8(RoleScoped), "", "repository|vi/ns/a|pull")
	// Name spellings that must not smuggle one repository past another.
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|vi/ns/a/|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|vi/ns/./a|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|/vi/ns/a|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|vi/ns/a/..|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|vi/ns/b/../a|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|vi/ns/a/../b|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a/..|pull", "repository|vi/ns|pull")
	f.Add(uint8(RoleScoped), "repository|../../etc|pull", "repository|etc|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|vi//ns//a|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|VI/NS/A|pull")
	f.Add(uint8(RoleScoped), "repository||pull", "repository||pull")
	f.Add(uint8(RoleScoped), "repository|.|pull", "repository||pull")
	f.Add(uint8(RoleScoped), "repository|/|pull", "repository||pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|*", "repository|vi/ns/a|delete")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|pull", "repository|vi/ns/a|*")
	f.Add(uint8(RoleScoped), "repository|*|pull", "repository|vi/ns/a|pull")
	f.Add(uint8(RoleScoped), "repository|vi/ns/a|,", "repository|vi/ns/a|")
	// Type confusion between the repository and the registry-wide resource.
	f.Add(uint8(RoleScoped), "registry|catalog|*", "repository|catalog|pull")
	f.Add(uint8(RoleScoped), "repository|catalog|pull", "registry|catalog|pull")
	f.Add(uint8(RoleScoped), "|vi/ns/a|pull", "repository|vi/ns/a|pull")
	// An empty request, documented as allowed, and roles outside the declared set.
	f.Add(uint8(RoleAdmin), "", "")
	f.Add(uint8(RoleScoped), "", "")
	f.Add(uint8(RoleNone), "", "")
	f.Add(uint8(200), "", "repository|vi/ns/a|pull")
	f.Add(uint8(255), "repository|vi/ns/a|pull", "repository|vi/ns/a|pull")
	f.Add(uint8(RoleScoped), "repository|\x00\x01|pull", "repository|\x00\x01|pull")
	f.Add(uint8(RoleScoped), "repository|\xff\xfe|pull", "repository|\xff\xfe|pull")
	f.Add(uint8(RoleScoped), "repository|"+strings.Repeat("a/", 512)+"x|pull", "repository|"+strings.Repeat("a/", 512)+"x|pull")
	f.Add(uint8(RoleScoped), strings.Repeat("repository|vi/ns/a|pull;", 32), "repository|vi/ns/a|pull")

	f.Fuzz(func(t *testing.T, role uint8, grantSpec, accessSpec string) {
		if len(grantSpec) > fuzzMaxPart || len(accessSpec) > fuzzMaxPart {
			t.Skip()
		}

		grants := parseGrants(grantSpec)
		accesses := parseAccesses(accessSpec)
		if len(grants) > fuzzMaxEntries || len(accesses) > fuzzMaxEntries {
			t.Skip()
		}

		subject := Subject{Role: Role(role), Grants: grants}

		got := Authorize(subject, accesses)

		if again := Authorize(subject, accesses); again != got {
			t.Fatal("Authorize returned two different verdicts for the same subject and request")
		}

		requireExpectedVerdict(t, subject, accesses, got)
		requireEveryAccessAllowedOnItsOwn(t, subject, accesses, got)
	})
}

// requireExpectedVerdict recomputes the verdict the plugin had to reach.
func requireExpectedVerdict(t *testing.T, subject Subject, accesses []Access, got bool) {
	t.Helper()

	switch subject.Role {
	case RoleAdmin:
		if !got {
			t.Fatal("the admin credential was denied")
		}
	case RolePuller:
		want := true
		for _, a := range accesses {
			if a.Type != "repository" || a.Action != "pull" {
				want = false
				break
			}
		}
		if got != want {
			t.Fatalf("the node credential got %v for %v, expected %v: it must never do anything but pull a repository", got, accesses, want)
		}
	case RoleScoped:
		want := true
		for _, a := range accesses {
			if !segmentsCover(subject.Grants, a) {
				want = false
				break
			}
		}
		if got != want {
			t.Fatalf("a scoped credential holding %v got %v for %v, expected %v", subject.Grants, got, accesses, want)
		}
	default:
		// Fail-closed, with the documented exception of an empty request.
		if got && len(accesses) > 0 {
			t.Fatalf("role %d was granted %v", subject.Role, accesses)
		}
	}
}

// requireEveryAccessAllowedOnItsOwn: a granted request must have each of its
// accesses granted alone.
func requireEveryAccessAllowedOnItsOwn(t *testing.T, subject Subject, accesses []Access, got bool) {
	t.Helper()

	if !got {
		return
	}

	for i := range accesses {
		if !Authorize(subject, accesses[i:i+1]) {
			t.Fatalf("the whole request %v was granted but its access %v is denied alone", accesses, accesses[i])
		}
	}
}

// segmentsCover is the expected scoped-subject decision, walking path segments
// rather than reusing the cleanName formulation the code uses.
func segmentsCover(grants []Grant, a Access) bool {
	want := nameSegments(a.Name)

	for _, g := range grants {
		if g.Type != a.Type {
			continue
		}

		if !slices.Equal(nameSegments(g.Name), want) {
			continue
		}

		for _, action := range g.Actions {
			if action == a.Action || action == "*" {
				return true
			}
		}
	}

	return false
}

// nameSegments reduces a repository name to the segments that identify it:
// empty and "." segments drop out, ".." removes the one before it and drops out
// at the root, the way path.Clean clamps when anchored at "/".
func nameSegments(name string) []string {
	segments := []string{}

	for _, segment := range strings.Split(name, "/") {
		switch segment {
		case "", ".":
			continue
		case "..":
			if len(segments) > 0 {
				segments = segments[:len(segments)-1]
			}
		default:
			segments = append(segments, segment)
		}
	}

	return segments
}

// parseGrants reads grants from the fuzzed string: entries separated by ";",
// fields by "|", actions by ",".
func parseGrants(spec string) []Grant {
	if spec == "" {
		return nil
	}

	var grants []Grant

	for _, entry := range strings.Split(spec, ";") {
		if entry == "" {
			continue
		}

		fields := strings.Split(entry, "|")
		grant := Grant{Type: field(fields, 0), Name: field(fields, 1)}
		if actions := field(fields, 2); actions != "" {
			grant.Actions = strings.Split(actions, ",")
		}

		grants = append(grants, grant)
	}

	return grants
}

// parseAccesses reads accesses in the same shape, one action per entry.
func parseAccesses(spec string) []Access {
	if spec == "" {
		return nil
	}

	var accesses []Access

	for _, entry := range strings.Split(spec, ";") {
		if entry == "" {
			continue
		}

		fields := strings.Split(entry, "|")
		accesses = append(accesses, Access{
			Type:   field(fields, 0),
			Name:   field(fields, 1),
			Action: field(fields, 2),
		})
	}

	return accesses
}

func field(fields []string, i int) string {
	if i < len(fields) {
		return fields[i]
	}
	return ""
}

// FuzzClassify drives the credential path: classify, and under it verifyJWT.
// Both halves come out of the HTTP Basic header of a registry client: the
// static admin and node passwords are matched in constant time, anything else
// is treated as a signed scoped token.
//
// The property is fail-closed: a credential that is not exactly one of the
// configured passwords must not be classified as that role, and an error must
// come back with a zero Subject. A token that verifies must carry the
// configured issuer and audience, a live validity window, and grants that are
// the access claim it was given.
func FuzzClassify(f *testing.F) {
	// Rejection is the normal outcome here, and verifyJWT logs every one.
	logrus.SetOutput(io.Discard)
	f.Cleanup(func() { logrus.SetOutput(os.Stderr) })

	// The exact configured credentials, which must be accepted.
	f.Add(fuzzAdminUser, fuzzAdminPass, []byte(nil))
	f.Add(fuzzPullerUser, fuzzPullerPass, []byte(nil))
	// Near misses on the configured credentials.
	f.Add(fuzzAdminUser, fuzzAdminPass[:len(fuzzAdminPass)-1], []byte(nil))
	f.Add(fuzzAdminUser, fuzzAdminPass+"x", []byte(nil))
	f.Add(fuzzAdminUser, strings.ToUpper(fuzzAdminPass), []byte(nil))
	f.Add(fuzzAdminUser, " "+fuzzAdminPass, []byte(nil))
	f.Add(fuzzAdminUser, fuzzAdminPass+"\x00", []byte(nil))
	f.Add(fuzzAdminUser, "", []byte(nil))
	f.Add(fuzzPullerUser, fuzzPullerPass[:1], []byte(nil))
	f.Add(fuzzPullerUser, fuzzAdminPass, []byte(nil))
	f.Add(fuzzAdminUser, fuzzPullerPass, []byte(nil))
	f.Add(fuzzAdminUser+"x", fuzzAdminPass, []byte(nil))
	f.Add(strings.ToUpper(fuzzAdminUser), fuzzAdminPass, []byte(nil))
	f.Add("", fuzzAdminPass, []byte(nil))
	f.Add(" "+fuzzAdminUser, fuzzAdminPass, []byte(nil))
	// Passwords that are shaped like a token.
	f.Add("importer", fuzzJWT(`{"alg":"ES256","kid":"dvcr","typ":"JWT"}`, "", ""), []byte(nil))
	f.Add("importer", "a.b.c", []byte(nil))
	f.Add("importer", "..", []byte(nil))
	f.Add("importer", "....", []byte(nil))
	f.Add("importer", fuzzJWT(`{"alg":"none"}`, `{"access":[{"type":"repository","name":"vi/ns/a","actions":["pull"]}]}`, ""), []byte(nil))
	f.Add("importer", fuzzJWT(`{"alg":"ES256"}`, `{"iss":"virtualization-controller"}`)+".AAAA", []byte(nil))
	f.Add("importer", strings.Repeat("a", 4096), []byte(nil))
	f.Add("importer", "\x00\x01\x02", []byte(nil))
	f.Add("importer", "\xff\xfe", []byte(nil))
	f.Add("importer", strings.Repeat("a.", 1024), []byte(nil))

	// Raw claim sets, which f.Fuzz signs with the trusted key so they reach the
	// claim parsing. The first must verify; the rest must not.
	f.Add("importer", "", fuzzClaimSet(fuzzJWTIssuer, fuzzJWTAudience, fuzzClaimNeverExpires, fuzzGrantedAccess()))
	f.Add("importer", "", fuzzClaimSet("someone-else", fuzzJWTAudience, fuzzClaimNeverExpires, fuzzGrantedAccess()))
	f.Add("importer", "", fuzzClaimSet(fuzzJWTIssuer, "another-registry", fuzzClaimNeverExpires, fuzzGrantedAccess()))
	f.Add("importer", "", fuzzClaimSet(fuzzJWTIssuer, fuzzJWTAudience, fuzzClaimLongExpired, fuzzGrantedAccess()))
	f.Add("importer", "", fuzzClaimSet(fuzzJWTIssuer, fuzzJWTAudience, fuzzClaimNeverExpires, nil))
	f.Add("importer", "", fuzzClaimSet(fuzzJWTIssuer, fuzzJWTAudience, fuzzClaimNeverExpires, []*token.ResourceActions{nil}))
	// Claim sets the controller would never mint.
	f.Add("importer", "", []byte(`{"iss":"virtualization-controller","aud":"dvcr","exp":99999999999,"access":[{"type":"repository","name":"../../etc","actions":["*"]}]}`))
	f.Add("importer", "", []byte(`{"iss":"virtualization-controller","aud":["dvcr","other"],"exp":99999999999,"access":[]}`))
	f.Add("importer", "", []byte(`{"iss":"virtualization-controller","aud":"dvcr","exp":99999999999,"access":null}`))
	f.Add("importer", "", []byte(`{"access":[{"type":"repository","name":"vi/ns/a","actions":["pull"]}]}`))
	// Claim names differing in case only: matched case sensitively, so these
	// carry no issuer and no access.
	f.Add("importer", "", []byte(`{"ISS":"virtualization-controller","aud":"dvcr","exp":99999999999,"ACCess":[{"type":"repository","name":"vi/ns/a","actions":["*"]}]}`))
	f.Add("importer", "", []byte(`{"iss":"virtualization-controller","AUD":"dvcr","exp":99999999999,"access":[{"type":"repository","name":"vi/ns/a","actions":["pull"]}]}`))
	// Payloads that are not a claim set at all.
	f.Add("importer", "", []byte("{"))
	f.Add("importer", "", []byte("[]"))
	f.Add("importer", "", []byte("null"))
	f.Add("importer", "", []byte("\xff\xfe"))
	f.Add("importer", "", []byte(""))

	f.Fuzz(func(t *testing.T, username, password string, claims []byte) {
		if len(username) > fuzzMaxPart || len(password) > fuzzMaxPart || len(claims) > fuzzMaxPart {
			t.Skip()
		}

		// Signed here rather than in the corpus, which holds raw claim sets.
		signed := len(claims) > 0
		if signed {
			password = fuzzSignedToken(t, claims)
		}

		ac := fuzzAccessController()

		// Read before the code takes its own; fuzzTimeMargin covers the gap.
		now := time.Now()
		subject, name, err := ac.classify(username, password)

		var (
			expect = verdictReject
			want   []Grant
		)
		if signed {
			expect, want = fuzzVerdict(claims, now)
		}

		if err != nil {
			if expect == verdictVerify {
				t.Fatalf("a claim set the controller could have minted was rejected with %v: %s", err, claims)
			}

			if subject.Role != RoleNone || len(subject.Grants) != 0 || name != "" {
				t.Fatalf("classify failed with %v but still returned role %d, %d grants and name %q", err, subject.Role, len(subject.Grants), name)
			}

			// What a caller ignoring the error would get.
			if Authorize(subject, []Access{{Type: "repository", Name: "vi/ns/a", Action: "pull"}}) {
				t.Fatal("a subject from a failed classification was granted access")
			}

			return
		}

		switch subject.Role {
		case RoleAdmin:
			if username != fuzzAdminUser || password != fuzzAdminPass {
				t.Fatalf("credential %q/%q was classified as the admin role", username, password)
			}
		case RolePuller:
			if username != fuzzPullerUser || password != fuzzPullerPass {
				t.Fatalf("credential %q/%q was classified as the node role", username, password)
			}
		case RoleScoped:
			if !signed {
				t.Fatalf("password %q verified as a scoped token and yielded %d grants", password, len(subject.Grants))
			}

			if expect == verdictReject {
				t.Fatalf("a token that must not verify yielded %d grants: %s", len(subject.Grants), claims)
			}

			requireGrants(t, subject.Grants, want)
		default:
			t.Fatalf("classify succeeded with role %d", subject.Role)
		}

		if name == "" {
			t.Fatalf("credential %q was accepted with role %d and an empty name", username, subject.Role)
		}
	})
}

// fuzzTimeMargin separates the oracle's clock reading from the code's: a token
// within the margin of a validity bound gets no verdict.
const fuzzTimeMargin = time.Second

type verdict int

const (
	verdictReject verdict = iota
	verdictVerify
	verdictEither
)

// fuzzVerdict recomputes what token verification has to decide: reject, verify,
// or either when a validity bound lies within fuzzTimeMargin of now. The grants
// come back whenever the claim set decodes at all.
func fuzzVerdict(claims []byte, now time.Time) (verdict, []Grant) {
	var claimSet token.ClaimSet
	if err := josejson.Unmarshal(claims, &claimSet); err != nil {
		return verdictReject, nil
	}

	var want []Grant
	for _, access := range claimSet.Access {
		if access == nil {
			continue
		}

		want = append(want, Grant{Type: access.Type, Name: access.Name, Actions: access.Actions})
	}

	if claimSet.Issuer != fuzzJWTIssuer || !slices.Contains(claimSet.Audience, fuzzJWTAudience) {
		return verdictReject, want
	}

	var (
		expiry    = time.Unix(claimSet.Expiration, 0).Add(token.Leeway)
		notBefore = time.Unix(claimSet.NotBefore, 0).Add(-token.Leeway)
		earliest  = now.Add(-fuzzTimeMargin)
		latest    = now.Add(fuzzTimeMargin)
	)

	switch {
	case earliest.After(expiry), latest.Before(notBefore):
		return verdictReject, want
	case latest.After(expiry), earliest.Before(notBefore):
		return verdictEither, want
	default:
		return verdictVerify, want
	}
}

// requireGrants checks the grants of a verified token against its access claim.
func requireGrants(t *testing.T, grants, want []Grant) {
	t.Helper()

	if len(grants) != len(want) {
		t.Fatalf("a claim set carrying %d accesses yielded %d grants", len(want), len(grants))
	}

	for i := range want {
		if grants[i].Type != want[i].Type || grants[i].Name != want[i].Name || !slices.Equal(grants[i].Actions, want[i].Actions) {
			t.Fatalf("grant %d is %+v, the access claim was %+v", i, grants[i], want[i])
		}
	}
}

// fuzzAccessController builds the controller the way the registry config does.
func fuzzAccessController() *accessController {
	return &accessController{
		realm:          "dvcr",
		adminUsername:  fuzzAdminUser,
		adminPassword:  []byte(fuzzAdminPass),
		pullerUsername: fuzzPullerUser,
		pullerPassword: []byte(fuzzPullerPass),
		jwtIssuer:      fuzzJWTIssuer,
		jwtAudience:    fuzzJWTAudience,
		trustedKeys:    map[string]crypto.PublicKey{fuzzKeyID: fuzzSigningKey.Public()},
	}
}

// fuzzJWT spells a compact token out of its parts, encoding each and joining
// them with ".". Seeds build tokens through it instead of holding them as
// literals, which secret scanners read as leaked credentials. An empty part
// encodes to an empty one.
func fuzzJWT(parts ...string) string {
	encoded := make([]string, len(parts))
	for i, part := range parts {
		encoded[i] = base64.RawURLEncoding.EncodeToString([]byte(part))
	}

	return strings.Join(encoded, ".")
}

// fuzzSignedToken signs the payload as-is rather than marshalling it from a
// struct: the claim parsing has to survive a claim set no struct could spell.
func fuzzSignedToken(t *testing.T, payload []byte) string {
	t.Helper()

	signed, err := fuzzSigner.Sign(payload)
	if err != nil {
		t.Fatalf("failed to sign a %d byte payload: %v", len(payload), err)
	}

	raw, err := signed.CompactSerialize()
	if err != nil {
		t.Fatalf("failed to serialize the token: %v", err)
	}

	return raw
}

// Validity bounds of the built claim sets, as fixed instants rather than
// offsets from the clock: a seed built from time.Now() is different bytes on
// every run, so nothing accumulates in the corpus.
const (
	fuzzClaimIssuedAt     = 1700000000  // 2023-11-14
	fuzzClaimNeverExpires = 99999999999 // year 5138
	fuzzClaimLongExpired  = fuzzClaimIssuedAt + 3600
)

// fuzzClaimSet builds a claim set the way the controller mints one, so a seed
// can vary one field.
func fuzzClaimSet(issuer, audience string, expiresAt int64, access []*token.ResourceActions) []byte {
	payload, err := josejson.Marshal(token.ClaimSet{
		Issuer:     issuer,
		Subject:    "importer",
		Audience:   token.AudienceList{audience},
		Expiration: expiresAt,
		NotBefore:  fuzzClaimIssuedAt - 60,
		IssuedAt:   fuzzClaimIssuedAt,
		Access:     access,
	})
	if err != nil {
		panic(err)
	}

	return payload
}

// fuzzGrantedAccess is the access claim of an ordinary importer token.
func fuzzGrantedAccess() []*token.ResourceActions {
	return []*token.ResourceActions{{Type: "repository", Name: "vi/ns/a", Actions: []string{"pull", "push"}}}
}
