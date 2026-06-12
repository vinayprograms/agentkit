package credentials

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// StandardPaths returns the conventional credential-file search locations for
// an application, in priority order (first found wins):
//
//	./credentials.toml
//	~/.config/<app>/credentials.toml
//	~/.<app>/credentials.toml
//
// The home-relative paths are omitted if the home directory cannot be resolved.
func StandardPaths(app string) []string {
	paths := []string{"credentials.toml"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, ".config", app, "credentials.toml"),
			filepath.Join(home, "."+app, "credentials.toml"),
		)
	}
	return paths
}

// Load composes a Lookup from environment variables, the first existing file
// among paths, and the Claude CLI OAuth token (if present), in increasing
// priority: env < file < Claude CLI. A missing file is skipped; an existing
// but unusable file (insecure permissions, parse error) returns an error.
//
// The returned FileStore is the first file that was loaded (nil if none); it
// is handed back so callers can persist updates via FileStore.Save.
func Load(paths ...string) (lookup Lookup, file FileStore, err error) {
	stores := []Lookup{NewEnvStore()}

	for _, path := range paths {
		if _, statErr := os.Stat(path); statErr != nil {
			continue // no file here, try next
		}
		fs, ferr := NewFileStore(path)
		if ferr != nil {
			return nil, nil, ferr
		}
		stores = append(stores, fs)
		file = fs
		break
	}

	if cli := ClaudeCLICredentials(); cli != nil {
		stores = append(stores, cli)
	}

	return NewUnionStore(stores...), file, nil
}

// claudeCLIFile mirrors the structure of ~/.claude/.credentials.json.
type claudeCLIFile struct {
	ClaudeAiOauth *struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"` // Unix timestamp in milliseconds
	} `json:"claudeAiOauth"`
}

// ClaudeCLICredentials reads the Claude Code CLI OAuth token from
// ~/.claude/.credentials.json and returns it as a FileStore under the
// "anthropic" provider, or nil if the file is missing or has no usable token.
func ClaudeCLICredentials() FileStore {
	token := readClaudeCLIToken()
	if token == nil {
		return nil
	}
	return FileStore{"anthropic": {OAuth: token}}
}

func readClaudeCLIToken() *OAuthToken {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json"))
	if err != nil {
		return nil
	}

	var f claudeCLIFile
	if err := json.Unmarshal(data, &f); err != nil || f.ClaudeAiOauth == nil || f.ClaudeAiOauth.AccessToken == "" {
		return nil
	}

	token := &OAuthToken{
		AccessToken:  f.ClaudeAiOauth.AccessToken,
		RefreshToken: f.ClaudeAiOauth.RefreshToken,
	}
	if ms := f.ClaudeAiOauth.ExpiresAt; ms > 0 {
		token.ExpiresAt = time.UnixMilli(ms)
	}
	return token
}
