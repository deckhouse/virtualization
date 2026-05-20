package releases

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const modulePageHTML = `
<!doctype html>
<html>
<body>
<table>
<thead>
<tr><th>Edition</th><th>Alpha</th><th>Beta</th><th>Early Access</th><th>Stable</th><th>Rock Solid</th></tr>
</thead>
<tbody>
<tr><td><a href="/fe">Flant Edition</a></td><td>1.8.2</td><td>1.7.2</td><td>1.7.2</td><td>1.7.1</td><td>1.7.1</td></tr>
<tr><td><a href="/ee">Enterprise Edition</a></td><td>1.8.2</td><td>1.7.2</td><td>1.7.2</td><td>1.7.1</td><td>1.7.1</td></tr>
<tr><td><a href="/se-plus">Standard Edition+</a></td><td>1.8.2</td><td>1.7.2</td><td>1.7.2</td><td>1.7.1</td><td>1.7.1</td></tr>
<tr><td><a href="/ce">Community Edition</a></td><td>1.8.2</td><td>1.7.2</td><td>1.7.2</td><td>1.7.1</td><td>1.7.1</td></tr>
</tbody>
</table>
</body>
</html>`

func TestVerifyVersionOnModulePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(modulePageHTML))
	}))
	defer server.Close()

	checkPassed, versionInfo, err := VerifyVersionOnModulePage(server.URL, "alpha", "v1.8.2", "virtualization")
	if err != nil {
		t.Fatalf("VerifyVersionOnModulePage returned error: %v", err)
	}
	if !checkPassed {
		t.Fatal("expected version check to pass")
	}
	if len(versionInfo.Versions) != 4 {
		t.Fatalf("expected 4 matching editions, got %d", len(versionInfo.Versions))
	}
}

func TestVerifyVersionOnModulePageNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(modulePageHTML))
	}))
	defer server.Close()

	_, _, err := VerifyVersionOnModulePage(server.URL, "alpha", "1.8.3", "virtualization")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "version 1.8.3 not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}
