package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// ///////////////////////////////////////////////
// parseSemver Tests
// ///////////////////////////////////////////////

func TestParseSemver(t *testing.T) {
	tests := []struct {
		input string
		want  []int
	}{
		{"1.2.3", []int{1, 2, 3}},
		{"v1.2.3", []int{1, 2, 3}},
		{"0.0.0", []int{0, 0, 0}},
		{"0.0.0-dev", []int{0, 0, 0}},
		{"1.0.0-beta+build123", []int{1, 0, 0}},
		{"v0.1.0", []int{0, 1, 0}},
		{"10.20.30", []int{10, 20, 30}},
		{"1.2.3-rc.1", []int{1, 2, 3}},
		{"1.2.3+metadata", []int{1, 2, 3}},

		// Invalid inputs should return nil.
		{"", nil},
		{"1.2", nil},
		{"1", nil},
		{"not.a.version", nil},
		{"v", nil},
		{"1.2.x", nil},
		{"a.b.c", nil},
		{"1.2.3.4", nil}, // SplitN with 3 means "3.4" is treated as one part; "3.4" has no '-' or '+', and '.' is not a digit, so nil
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSemver(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseSemver(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// semverLess Tests
// ///////////////////////////////////////////////

func TestSemverLess(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"equal versions", "1.2.3", "1.2.3", false},
		{"a < b major", "0.9.9", "1.0.0", true},
		{"a > b major", "2.0.0", "1.9.9", false},
		{"a < b minor", "1.0.0", "1.1.0", true},
		{"a > b minor", "1.2.0", "1.1.0", false},
		{"a < b patch", "1.0.0", "1.0.1", true},
		{"a > b patch", "1.0.2", "1.0.1", false},
		{"with v prefix", "v0.1.0", "v0.2.0", true},
		{"mixed prefix", "0.1.0", "v0.2.0", true},
		{"pre-release stripped", "0.0.0-dev", "0.1.0", true},
		{"same with pre-release", "1.0.0-alpha", "1.0.0-beta", false}, // both parse to 1.0.0; no ordering between different pre-releases
		{"pre-release less than release", "0.1.0-dev", "0.1.0", true},
		{"release not less than pre-release", "0.1.0", "0.1.0-dev", false},
		{"pre-release less than release with v", "v1.0.0-rc.1", "v1.0.0", true},
		{"both pre-release equal numeric", "1.0.0-alpha", "1.0.0-alpha", false},
		{"invalid a", "invalid", "1.0.0", false},
		{"invalid b", "1.0.0", "invalid", false},
		{"both invalid", "foo", "bar", false},
		{"empty a", "", "1.0.0", false},
		{"empty b", "1.0.0", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := semverLess(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("semverLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// ///////////////////////////////////////////////
// Check Tests (via httptest mock)
// ///////////////////////////////////////////////

func TestCheck_NewerVersionAvailable(t *testing.T) {
	manifest := map[string]string{".": "1.2.0"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	// Override the package-level manifestURL for this test.
	old := manifestURL
	manifestURL = server.URL
	defer func() { manifestURL = old }()

	// Check should not panic — it logs but does not return errors.
	// We verify indirectly that it completes without error.
	Check("1.0.0")
}

func TestCheck_SameVersion(t *testing.T) {
	manifest := map[string]string{".": "1.0.0"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	old := manifestURL
	manifestURL = server.URL
	defer func() { manifestURL = old }()

	Check("1.0.0")
}

func TestCheck_EmptyManifestURL(t *testing.T) {
	old := manifestURL
	manifestURL = ""
	defer func() { manifestURL = old }()

	// Should return early without error.
	Check("1.0.0")
}

// ///////////////////////////////////////////////
// fetchLatest Tests
// ///////////////////////////////////////////////

func TestFetchLatest_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	old := manifestURL
	manifestURL = server.URL
	defer func() { manifestURL = old }()

	_, err := fetchLatest()
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestFetchLatest_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not json`))
	}))
	defer server.Close()

	old := manifestURL
	manifestURL = server.URL
	defer func() { manifestURL = old }()

	_, err := fetchLatest()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFetchLatest_ValidManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{".": "2.0.0"}`))
	}))
	defer server.Close()

	old := manifestURL
	manifestURL = server.URL
	defer func() { manifestURL = old }()

	version, err := fetchLatest()
	if err != nil {
		t.Fatalf("fetchLatest: %v", err)
	}
	if version != "2.0.0" {
		t.Errorf("version = %q, want %q", version, "2.0.0")
	}
}
