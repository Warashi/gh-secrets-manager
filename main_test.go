package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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
