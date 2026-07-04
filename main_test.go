package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"golang.org/x/crypto/nacl/box"
)

// redirectTransport rewrites every outgoing request to target the given
// httptest server, allowing api.RESTClient to be exercised against a local
// mock instead of the real GitHub API.
type redirectTransport struct {
	target *url.URL
}

func (t redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = t.target.Scheme
	req.URL.Host = t.target.Host
	req.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// newTestManager builds a manager whose REST client sends every request to
// the given httptest.Server.
func newTestManager(t *testing.T, server *httptest.Server) *manager {
	t.Helper()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server URL: %v", err)
	}

	client, err := api.NewRESTClient(api.ClientOptions{
		Host:      "localhost",
		AuthToken: "test-token",
		Transport: redirectTransport{target: target},
	})
	if err != nil {
		t.Fatalf("failed to create REST client: %v", err)
	}

	return &manager{
		client:  client,
		pubKeys: make(map[string]publicKey),
	}
}

// newPublicKeyServer returns an httptest.Server that serves the given key ID
// and NaCl box public key for the public-key endpoint, and the caller's
// matching private key for decrypting sealed secrets in assertions.
func newPublicKeyServer(t *testing.T) (server *httptest.Server, keyID string, priv *[32]byte) {
	t.Helper()

	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}
	keyID = "test-key-id-123"

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			KeyID string `json:"key_id"`
			Key   string `json:"key"`
		}{
			KeyID: keyID,
			Key:   base64.StdEncoding.EncodeToString(pub[:]),
		})
	}))

	return server, keyID, priv
}

func TestGetPublicKey(t *testing.T) {
	server, wantKeyID, priv := newPublicKeyServer(t)
	defer server.Close()

	m := newTestManager(t, server)

	pk, err := m.getPublicKey("owner", "repo")
	if err != nil {
		t.Fatalf("getPublicKey returned error: %v", err)
	}
	if pk.ID != wantKeyID {
		t.Errorf("key ID = %q, want %q", pk.ID, wantKeyID)
	}

	// Sanity check that the parsed key is usable for opening a box sealed
	// with the matching private key.
	msg := []byte("hello")
	sealed, err := box.SealAnonymous(nil, msg, &pk.Key, rand.Reader)
	if err != nil {
		t.Fatalf("SealAnonymous failed: %v", err)
	}
	opened, ok := box.OpenAnonymous(nil, sealed, &pk.Key, priv)
	if !ok {
		t.Fatalf("OpenAnonymous failed")
	}
	if string(opened) != string(msg) {
		t.Errorf("opened = %q, want %q", opened, msg)
	}

	// Cached result should be returned without issuing another request;
	// clearing the client would panic if getPublicKey tried to use it.
	m.client = nil
	pk2, err := m.getPublicKey("owner", "repo")
	if err != nil {
		t.Fatalf("getPublicKey (cached) returned error: %v", err)
	}
	if pk2.ID != wantKeyID {
		t.Errorf("cached key ID = %q, want %q", pk2.ID, wantKeyID)
	}
}

func TestEncryptSecret(t *testing.T) {
	server, wantKeyID, priv := newPublicKeyServer(t)
	defer server.Close()

	m := newTestManager(t, server)

	encrypted, keyID, err := m.encryptSecret("owner", "repo", "NAME", "ENV", "s3cr3t")
	if err != nil {
		t.Fatalf("encryptSecret returned error: %v", err)
	}
	if keyID != wantKeyID {
		t.Errorf("keyID = %q, want %q", keyID, wantKeyID)
	}

	sealed, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("failed to decode encrypted value: %v", err)
	}
	pub := m.pubKeys["owner/repo"].Key
	opened, ok := box.OpenAnonymous(nil, sealed, &pub, priv)
	if !ok {
		t.Fatalf("failed to open sealed box")
	}
	if string(opened) != "s3cr3t" {
		t.Errorf("decrypted secret = %q, want %q", opened, "s3cr3t")
	}
}

func TestRunSetNewEntry(t *testing.T) {
	server, wantKeyID, _ := newPublicKeyServer(t)
	defer server.Close()

	m := newTestManager(t, server)
	file := filepath.Join(t.TempDir(), "secrets.json")
	t.Setenv("MY_SECRET_ENV", "s3cr3t")

	if err := m.runSet([]string{
		"--file", file,
		"--owner", "myorg",
		"--repository", "myrepo",
		"--name", "SECRET_NAME",
		"--env", "MY_SECRET_ENV",
	}); err != nil {
		t.Fatalf("runSet returned error: %v", err)
	}

	sf, err := m.loadSecrets(file)
	if err != nil {
		t.Fatalf("failed to load secrets file: %v", err)
	}
	if len(sf.Secrets) != 1 {
		t.Fatalf("len(sf.Secrets) = %d, want 1", len(sf.Secrets))
	}
	if got := sf.Secrets[0].KeyID; got != wantKeyID {
		t.Errorf("KeyID = %q, want %q", got, wantKeyID)
	}
	if sf.Secrets[0].EncryptedSecret == "" {
		t.Errorf("EncryptedSecret is empty")
	}
}

func TestRunSetUpdatesExistingEntryKeyID(t *testing.T) {
	server, wantKeyID, _ := newPublicKeyServer(t)
	defer server.Close()

	m := newTestManager(t, server)
	file := filepath.Join(t.TempDir(), "secrets.json")
	t.Setenv("MY_SECRET_ENV", "s3cr3t")

	setArgs := []string{
		"--file", file,
		"--owner", "myorg",
		"--repository", "myrepo",
		"--name", "SECRET_NAME",
		"--env", "MY_SECRET_ENV",
	}
	if err := m.runSet(setArgs); err != nil {
		t.Fatalf("first runSet returned error: %v", err)
	}

	// Simulate a pre-existing entry created before key_id support existed.
	sf, err := m.loadSecrets(file)
	if err != nil {
		t.Fatalf("failed to load secrets file: %v", err)
	}
	sf.Secrets[0].KeyID = ""
	if err := m.saveSecrets(file, sf); err != nil {
		t.Fatalf("failed to save secrets file: %v", err)
	}

	if err := m.runSet(setArgs); err != nil {
		t.Fatalf("second runSet returned error: %v", err)
	}

	sf, err = m.loadSecrets(file)
	if err != nil {
		t.Fatalf("failed to load secrets file: %v", err)
	}
	if len(sf.Secrets) != 1 {
		t.Fatalf("len(sf.Secrets) = %d, want 1", len(sf.Secrets))
	}
	if got := sf.Secrets[0].KeyID; got != wantKeyID {
		t.Errorf("KeyID = %q, want %q", got, wantKeyID)
	}
}

func TestRunRotateUpdatesKeyID(t *testing.T) {
	server, wantKeyID, _ := newPublicKeyServer(t)
	defer server.Close()

	m := newTestManager(t, server)
	file := filepath.Join(t.TempDir(), "secrets.json")

	sf := &SecretsFile{
		Version: 1,
		Secrets: []SecretEntry{
			{Owner: "myorg", Repository: "repo-a", Name: "SECRET_NAME", Env: "MY_SECRET_ENV"},
			{Owner: "myorg", Repository: "repo-b", Name: "SECRET_NAME", Env: "MY_SECRET_ENV"},
			{Owner: "myorg", Repository: "repo-c", Name: "OTHER_SECRET", Env: "OTHER_ENV"},
		},
	}
	if err := m.saveSecrets(file, sf); err != nil {
		t.Fatalf("failed to save secrets file: %v", err)
	}
	t.Setenv("MY_SECRET_ENV", "new-s3cr3t")

	if err := m.runRotate([]string{"--file", file, "--env", "MY_SECRET_ENV"}); err != nil {
		t.Fatalf("runRotate returned error: %v", err)
	}

	got, err := m.loadSecrets(file)
	if err != nil {
		t.Fatalf("failed to load secrets file: %v", err)
	}
	for _, s := range got.Secrets {
		if s.Env != "MY_SECRET_ENV" {
			if s.KeyID != "" {
				t.Errorf(
					"unrelated entry %s/%s got KeyID = %q, want empty",
					s.Owner,
					s.Repository,
					s.KeyID,
				)
			}
			continue
		}
		if s.KeyID != wantKeyID {
			t.Errorf("entry %s/%s KeyID = %q, want %q", s.Owner, s.Repository, s.KeyID, wantKeyID)
		}
		if s.EncryptedSecret == "" {
			t.Errorf("entry %s/%s EncryptedSecret is empty", s.Owner, s.Repository)
		}
	}
}
