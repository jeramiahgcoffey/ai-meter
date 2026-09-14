package meter

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

var safeID = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type Cache struct{ Dir string }

func (c *Cache) path(id string) string {
	return filepath.Join(c.Dir, safeID.ReplaceAllString(id, "_")+".json")
}

func (c *Cache) Store(snapshot Snapshot) error {
	if c == nil || c.Dir == "" {
		return nil
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	target := c.path(snapshot.ID)
	tmp, err := os.CreateTemp(c.Dir, ".snapshot-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

func (c *Cache) Load(id string) (Snapshot, error) {
	if c == nil || c.Dir == "" {
		return Snapshot{}, errors.New("cache disabled")
	}
	data, err := os.ReadFile(c.path(id))
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	if snapshot.ID != id {
		return Snapshot{}, errors.New("cache identity mismatch")
	}
	return snapshot, nil
}
