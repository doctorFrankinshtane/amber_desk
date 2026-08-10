package catalog

import (
	"context"
	"strings"
	"testing"

	public "amberdesk/pkg/catalog"
)

func TestStaticNormalizesAndRejectsUnsafeURLs(t *testing.T) {
	raw := `{"name":"OSINT Framework","children":[{"name":"People","children":[{"name":"Username","children":[{"name":"Example (T) (R)","url":"https://example.test/search","pricing":"free","opsec":"active"},{"name":"Unsafe","url":"javascript:alert(1)"}]}]}]}`
	provider, err := NewStatic(strings.NewReader(raw), public.Metadata{Version: "abc123"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := provider.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Categories) != 1 || snapshot.Categories[0].ToolCount != 1 {
		t.Fatalf("unexpected categories: %+v", snapshot.Categories)
	}
	if len(snapshot.Tools) != 1 || snapshot.Excluded != 1 {
		t.Fatalf("tools = %d, excluded = %d", len(snapshot.Tools), snapshot.Excluded)
	}
	tool := snapshot.Tools[0]
	if tool.Name != "Example" || tool.URL != "https://example.test/search" || strings.Join(tool.Badges, ",") != "T,R" {
		t.Fatalf("unexpected tool: %+v", tool)
	}
	if strings.Join(tool.Path, "/") != "People/Username" || len(tool.ID) != 16 {
		t.Fatalf("unexpected identity: %+v", tool)
	}
}

func TestStaticRejectsEmptyCatalog(t *testing.T) {
	_, err := NewStatic(strings.NewReader(`{"name":"OSINT Framework","children":[]}`), public.Metadata{})
	if err == nil {
		t.Fatal("expected empty catalog error")
	}
}
