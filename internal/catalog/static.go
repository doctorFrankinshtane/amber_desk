package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	public "amberdesk/pkg/catalog"
)

const (
	maxCatalogBytes = 8 << 20
	maxDepth        = 16
	maxNodes        = 10000
)

type Static struct {
	snapshot public.Snapshot
}

type rawNode struct {
	Name           string     `json:"name"`
	Type           string     `json:"type"`
	URL            string     `json:"url"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	Pricing        string     `json:"pricing"`
	BestFor        string     `json:"bestFor"`
	Input          string     `json:"input"`
	Output         string     `json:"output"`
	OPSEC          string     `json:"opsec"`
	OPSECNote      string     `json:"opsecNote"`
	LocalInstall   bool       `json:"localInstall"`
	GoogleDork     bool       `json:"googleDork"`
	Registration   bool       `json:"registration"`
	EditURL        bool       `json:"editUrl"`
	API            bool       `json:"api"`
	InvitationOnly bool       `json:"invitationOnly"`
	Deprecated     bool       `json:"deprecated"`
	Children       []*rawNode `json:"children"`
}

func NewStatic(reader io.Reader, metadata public.Metadata) (*Static, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, maxCatalogBytes+1))
	var root rawNode
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("decode catalog snapshot: %w", err)
	}
	if root.Name == "" || len(root.Children) == 0 {
		return nil, errors.New("catalog root is empty")
	}
	snapshot := public.Snapshot{Metadata: metadata, Categories: make([]public.Category, 0, len(root.Children))}
	seen := make(map[string]struct{})
	nodeCount := 0
	for _, categoryNode := range root.Children {
		if categoryNode == nil {
			continue
		}
		categoryName := clean(categoryNode.Name, 160)
		category := public.Category{ID: stableID(categoryName), Name: categoryName}
		before := len(snapshot.Tools)
		if err := walk(categoryNode, []string{categoryName}, 1, &nodeCount, seen, &snapshot); err != nil {
			return nil, err
		}
		category.ToolCount = len(snapshot.Tools) - before
		if category.ToolCount > 0 {
			snapshot.Categories = append(snapshot.Categories, category)
		}
	}
	if len(snapshot.Tools) == 0 {
		return nil, errors.New("catalog contains no valid tools")
	}
	sort.Slice(snapshot.Tools, func(i, j int) bool {
		left, right := strings.Join(snapshot.Tools[i].Path, "\x00")+snapshot.Tools[i].Name, strings.Join(snapshot.Tools[j].Path, "\x00")+snapshot.Tools[j].Name
		return left < right
	})
	return &Static{snapshot: snapshot}, nil
}

func (provider *Static) Snapshot(context.Context) (public.Snapshot, error) {
	return provider.snapshot, nil
}

func walk(node *rawNode, path []string, depth int, nodeCount *int, seen map[string]struct{}, snapshot *public.Snapshot) error {
	if depth > maxDepth {
		return errors.New("catalog exceeds maximum depth")
	}
	*nodeCount++
	if *nodeCount > maxNodes {
		return errors.New("catalog exceeds maximum node count")
	}
	if node.URL != "" {
		parsed, err := url.Parse(strings.TrimSpace(node.URL))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			snapshot.Excluded++
			return nil
		}
		name, badges := parseName(clean(node.Name, 200))
		id := stableID(strings.Join(path, "/") + "\x00" + parsed.String())
		if _, exists := seen[id]; exists {
			return nil
		}
		seen[id] = struct{}{}
		snapshot.Tools = append(snapshot.Tools, public.Tool{
			ID: id, Name: name, URL: parsed.String(), Description: clean(node.Description, 4096), Status: clean(node.Status, 64), Pricing: clean(node.Pricing, 64), BestFor: clean(node.BestFor, 2048), Input: clean(node.Input, 1024), Output: clean(node.Output, 1024), OPSEC: clean(node.OPSEC, 64), OPSECNote: clean(node.OPSECNote, 2048), LocalInstall: node.LocalInstall, GoogleDork: node.GoogleDork, Registration: node.Registration, EditableURL: node.EditURL, API: node.API, InvitationOnly: node.InvitationOnly, Deprecated: node.Deprecated, InsecureURL: parsed.Scheme == "http", Badges: badges, Path: append([]string(nil), path...),
		})
		return nil
	}
	for _, child := range node.Children {
		if child == nil {
			continue
		}
		childPath := path
		if child.URL == "" {
			childPath = append(append([]string(nil), path...), clean(child.Name, 160))
		}
		if err := walk(child, childPath, depth+1, nodeCount, seen, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func parseName(value string) (string, []string) {
	badges := make([]string, 0, 4)
	for _, badge := range []string{"T", "D", "R", "M"} {
		suffix := " (" + badge + ")"
		if strings.Contains(value, suffix) {
			value = strings.ReplaceAll(value, suffix, "")
			badges = append(badges, badge)
		}
	}
	return strings.TrimSpace(value), badges
}

func clean(value string, limit int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}

func stableID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}
