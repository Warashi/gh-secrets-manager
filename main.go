package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"golang.org/x/crypto/nacl/box"
)

// SecretEntry represents a single secret in the JSON file
type SecretEntry struct {
	Owner           string    `json:"owner"`
	Repository      string    `json:"repository"`
	Name            string    `json:"name"`
	EncryptedSecret string    `json:"encrypted_secret"`
	Env             string    `json:"env"`
	AddedAt         time.Time `json:"added_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// SecretsFile is the root structure of secrets.json
type SecretsFile struct {
	Version int           `json:"version"`
	Secrets []SecretEntry `json:"secrets"`
}

type manager struct {
	client  *api.RESTClient
	pubKeys map[string][32]byte
}

func (m *manager) getPublicKey(owner, repo string) ([32]byte, error) {
	if pk, ok := m.pubKeys[owner+"/"+repo]; ok {
		return pk, nil
	}

	var pk struct {
		Key string `json:"key"`
	}
	if err := m.client.Get(fmt.Sprintf("repos/%s/%s/actions/secrets/public-key", owner, repo), &pk); err != nil {
		return [32]byte{}, err
	}

	keyBytes, err := base64.StdEncoding.DecodeString(pk.Key)
	if err != nil {
		return [32]byte{}, err
	}
	if len(keyBytes) != 32 {
		return [32]byte{}, fmt.Errorf("unexpected public key length: %d", len(keyBytes))
	}
	var pubKey [32]byte
	copy(pubKey[:], keyBytes)

	m.pubKeys[owner+"/"+repo] = pubKey

	return pubKey, nil
}

func (m *manager) encryptSecret(owner, repo, name, env string, secret string) (string, error) {
	pubKey, err := m.getPublicKey(owner, repo)
	if err != nil {
		return "", err
	}

	sealed, err := box.SealAnonymous(nil, []byte(secret), &pubKey, rand.Reader)
	if err != nil {
		return "", err
	}
	enc := base64.StdEncoding.EncodeToString(sealed)
	return enc, nil
}

func (m *manager) runSet(args []string) error {
	fs := flag.NewFlagSet("set", flag.ExitOnError)
	file := fs.String("file", "secrets.json", "Path to secrets JSON file")
	owner := fs.String("owner", "", "GitHub owner")
	repo := fs.String("repository", "", "GitHub repository (name)")
	name := fs.String("name", "", "Name of the secret")
	env := fs.String("env", "", "Environment variable name to read the plaintext secret from")
	fs.Parse(args)

	if *owner == "" || *repo == "" || *name == "" || *env == "" {
		return fmt.Errorf("--owner, --repository, --name and --env are required")
	}

	val := os.Getenv(*env)
	if val == "" {
		return fmt.Errorf("environment variable %s is empty or not set", *env)
	}

	encrypted, err := m.encryptSecret(*owner, *repo, *name, *env, val)
	if err != nil {
		return fmt.Errorf("encryption failed: %v", err)
	}

	sf, err := m.loadSecrets(*file)
	if err != nil {
		return fmt.Errorf("failed to load secrets file: %v", err)
	}

	now := time.Now().UTC()

	// Check if secret already exists
	found := false
	for i, s := range sf.Secrets {
		if s.Owner == *owner && s.Repository == *repo && s.Name == *name {
			sf.Secrets[i].EncryptedSecret = encrypted
			sf.Secrets[i].UpdatedAt = now
			found = true
			break
		}
	}
	// If not found, add new secret
	if !found {
		sf.Secrets = append(sf.Secrets, SecretEntry{
			Owner:           *owner,
			Repository:      *repo,
			Name:            *name,
			EncryptedSecret: encrypted,
			Env:             *env,
			AddedAt:         now,
			UpdatedAt:       now,
		})
	}

	// Save
	if err := m.saveSecrets(*file, sf); err != nil {
		return fmt.Errorf("failed to save secrets file: %v", err)
	}
	fmt.Printf("Secret %s set to %s in %s/%s\n", *name, *env, *owner, *repo)
	return nil
}

func (m *manager) runRotate(args []string) error {
	fs := flag.NewFlagSet("rotate", flag.ExitOnError)
	file := fs.String("file", "secrets.json", "Path to secrets JSON file")
	env := fs.String("env", "", "Environment variable name to read new plaintext secrets from")
	fs.Parse(args)

	if *env == "" {
		return fmt.Errorf("--env is required")
	}

	val := os.Getenv(*env)
	if val == "" {
		return fmt.Errorf("environment variable %s is empty or not set", *env)
	}

	sf, err := m.loadSecrets(*file)
	if err != nil {
		return fmt.Errorf("failed to load secrets file: %v", err)
	}

	now := time.Now().UTC()
	updated := 0
	for i, s := range sf.Secrets {
		if s.Env != *env {
			continue
		}
		encrypted, err := m.encryptSecret(s.Owner, s.Repository, s.Name, *env, val)
		if err != nil {
			return fmt.Errorf(
				"encryption failed for %s in %s/%s: %v",
				s.Name,
				s.Owner,
				s.Repository,
				err,
			)
		}
		sf.Secrets[i].EncryptedSecret = encrypted
		sf.Secrets[i].UpdatedAt = now
		updated++
	}

	if updated == 0 {
		return fmt.Errorf("no secrets found for environment %s", *env)
	}

	if err := m.saveSecrets(*file, sf); err != nil {
		return fmt.Errorf("failed to save secrets file: %v", err)
	}

	fmt.Printf("Rotated %d secrets for env %s\n", updated, *env)
	return nil
}

func (m *manager) runDelete(args []string) error {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	file := fs.String("file", "secrets.json", "Path to secrets JSON file")
	owner := fs.String("owner", "", "GitHub owner")
	repo := fs.String("repository", "", "GitHub repository (name)")
	name := fs.String("name", "", "Name of the secret")
	fs.Parse(args)

	if *owner == "" || *repo == "" || *name == "" {
		return fmt.Errorf("--owner, --repository and --name are required")
	}

	sf, err := m.loadSecrets(*file)
	if err != nil {
		return fmt.Errorf("failed to load secrets file: %v", err)
	}

	newList := make([]SecretEntry, 0, len(sf.Secrets))
	deleted := 0
	for _, s := range sf.Secrets {
		if s.Owner == *owner && s.Repository == *repo && s.Name == *name {
			deleted++
			continue
		}
		newList = append(newList, s)
	}

	if deleted == 0 {
		return fmt.Errorf("secret %s in %s/%s not found", *name, *owner, *repo)
	}
	sf.Secrets = newList

	if err := m.saveSecrets(*file, sf); err != nil {
		return fmt.Errorf("failed to save secrets file: %v", err)
	}
	fmt.Printf("Deleted secret %s from %s/%s\n", *name, *owner, *repo)
	return nil
}

func (m *manager) loadSecrets(path string) (*SecretsFile, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Initialize new
		return &SecretsFile{Version: 1, Secrets: []SecretEntry{}}, nil
	} else if err != nil {
		return nil, err
	}
	var sf SecretsFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, err
	}
	return &sf, nil
}

func (m *manager) saveSecrets(path string, sf *SecretsFile) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

func _main(args []string) int {
	if len(args) < 2 {
		usage()
		return 1
	}

	client, err := api.DefaultRESTClient()
	if err != nil {
		fmt.Println(err)
		return 1
	}

	m := &manager{
		client:  client,
		pubKeys: make(map[string][32]byte),
	}

	cmd := args[1]
	switch cmd {
	case "set":
		if err := m.runSet(args[2:]); err != nil {
			fmt.Println(err)
			return 1
		}
	case "rotate":
		if err := m.runRotate(args[2:]); err != nil {
			fmt.Println(err)
			return 1
		}
	case "delete":
		if err := m.runDelete(args[2:]); err != nil {
			fmt.Println(err)
			return 1
		}
	default:
		usage()
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  gh-secret-manager set --file <file> --owner <owner> --repository <repo> --name <name> --env <ENV_VAR>
  gh-secret-manager rotate --file <file> --env <ENV_VAR>
  gh-secret-manager delete --file <file> --owner <owner> --repository <repo> --name <name>
`)
}

func main() {
	os.Exit(_main(os.Args))
}
