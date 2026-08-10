package obsidian

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"amberdesk/pkg/connectors"
)

var stateKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func (c *Connector) ReadWorkspaceState(_ context.Context, key string) ([]byte, error) {
	path, err := c.workspaceStatePath(key, false)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, connectors.ErrEntityAbsent
	}
	if err != nil {
		return nil, fmt.Errorf("read obsidian workspace state: %w", err)
	}
	return data, nil
}

func (c *Connector) WriteWorkspaceState(_ context.Context, key string, data []byte) error {
	path, err := c.workspaceStatePath(key, true)
	if err != nil {
		return err
	}
	if err := atomicWrite(path, data); err != nil {
		return fmt.Errorf("write obsidian workspace state: %w", err)
	}
	return nil
}

func (c *Connector) workspaceStatePath(key string, create bool) (string, error) {
	if !c.configured {
		return "", connectors.ErrNotConfigured
	}
	if !stateKeyPattern.MatchString(key) {
		return "", errors.New("invalid workspace state key")
	}
	directory := filepath.Join(c.vaultPath, filepath.Dir(c.dossierDir), ".state")
	if create {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return "", fmt.Errorf("create workspace state directory: %w", err)
		}
	}
	path := filepath.Join(directory, key+".json")
	if err := ensureInside(c.vaultPath, path); err != nil {
		return "", err
	}
	return path, nil
}
