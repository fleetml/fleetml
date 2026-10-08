package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fleetml/fleetml/internal/artifact"
)

// Version is a model version on disk.
type Version struct {
	VersionID string `json:"version_id"`
	Version   string `json:"version"`
	VariantID string `json:"variant_id"`
	SHA256    string `json:"sha256"`
	Dir       string `json:"dir"` // holds model.bin
}

// ModelState is the active version of a model and the one before it.
type ModelState struct {
	Name          string   `json:"name"`
	Active        *Version `json:"active,omitempty"`
	LastKnownGood *Version `json:"last_known_good,omitempty"`
}

// State is everything the agent persists across restarts.
type State struct {
	Salt         string                 `json:"salt"` // per install, part of the fingerprint
	DeviceID     string                 `json:"device_id,omitempty"`
	Token        string                 `json:"token,omitempty"`
	TokenExpires time.Time              `json:"token_expires,omitempty"`
	TokenIssued  time.Time              `json:"token_issued,omitempty"`
	Signature    string                 `json:"signature,omitempty"` // labels and agent version at registration
	Models       map[string]*ModelState `json:"models"`              // by model ID
	Failed       map[string]time.Time   `json:"failed"`              // variant ID -> failed validation at
	// SigningKeys are the project's artifact signing keys, as last sent by the server.
	SigningKeys []artifact.Key `json:"signing_keys,omitempty"`
}

// Store loads and atomically saves the state file.
type Store struct {
	path string
	mu   sync.Mutex
	s    State
}

// OpenStore loads path, or starts empty if it does not exist.
func OpenStore(path string) (*Store, error) {
	st := &Store{path: path, s: State{Models: map[string]*ModelState{}, Failed: map[string]time.Time{}}}
	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return nil, fmt.Errorf("reading state: %w", err)
	default:
		if err := json.Unmarshal(data, &st.s); err != nil {
			return nil, fmt.Errorf("parsing state %s: %w", path, err)
		}
		if st.s.Models == nil {
			st.s.Models = map[string]*ModelState{}
		}
		if st.s.Failed == nil {
			st.s.Failed = map[string]time.Time{}
		}
	}
	return st, nil
}

// Read calls f with the state under the lock.
func (st *Store) Read(f func(s *State)) {
	st.mu.Lock()
	defer st.mu.Unlock()
	f(&st.s)
}

// Update changes the state under the lock and saves it.
func (st *Store) Update(f func(s *State)) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	f(&st.s)
	return st.save()
}

func (st *Store) save() error {
	data, err := json.MarshalIndent(st.s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o700); err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}
