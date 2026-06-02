// Package checkpoint persists the coordinator's queue State to local storage for
// crash recovery. The file lives at {WORKSPACE_FOLDER}/checkpt/coordinator.json.
//
// Writes are atomic (temp file + rename) so a crash mid-write never corrupts the
// last good checkpoint. IO is done by the caller OUTSIDE the queue mutex (the caller
// passes a Snapshot), so a slow disk never blocks request handling.
package checkpoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/elective/cohort-backend/internal/queue"
)

const fileName = "coordinator.json"

// Store reads/writes checkpoints under a single directory.
type Store struct {
	dir string
}

// New returns a Store rooted at {workspace}/checkpt, creating the directory if needed.
func New(workspace string) (*Store, error) {
	dir := filepath.Join(workspace, "checkpt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create checkpoint dir %q: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

// Path is the full checkpoint file path (useful for logs/verification).
func (s *Store) Path() string { return filepath.Join(s.dir, fileName) }

// Save atomically writes the state: marshal -> temp file -> fsync -> rename.
func (s *Store) Save(state queue.State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, fileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp checkpoint: %w", err)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail before the rename.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp checkpoint: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync temp checkpoint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp checkpoint: %w", err)
	}
	if err := os.Rename(tmpName, s.Path()); err != nil {
		return fmt.Errorf("rename checkpoint into place: %w", err)
	}
	return nil
}

// Load reads the last checkpoint. Returns (state, true, nil) when present,
// (zero, false, nil) when no checkpoint exists yet, and an error on a corrupt or
// unreadable file.
func (s *Store) Load() (queue.State, bool, error) {
	data, err := os.ReadFile(s.Path())
	if errors.Is(err, os.ErrNotExist) {
		return queue.State{}, false, nil
	}
	if err != nil {
		return queue.State{}, false, fmt.Errorf("read checkpoint: %w", err)
	}
	var state queue.State
	if err := json.Unmarshal(data, &state); err != nil {
		return queue.State{}, false, fmt.Errorf("unmarshal checkpoint (corrupt?): %w", err)
	}
	return state, true, nil
}
