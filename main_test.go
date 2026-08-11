package main

import (
	"os"
	"testing"
)

func TestBundledCatalogMetadata(t *testing.T) {
	data, err := os.ReadFile("web/data/osint-framework.meta.json")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := loadCatalogMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != "OSINT Framework" || metadata.Version == "" || metadata.Source == "" || metadata.License == "" {
		t.Fatalf("incomplete catalog metadata: %+v", metadata)
	}
}

func TestCatalogMetadataRejectsInvalidProvenance(t *testing.T) {
	for name, data := range map[string]string{
		"unknown field":   `{"name":"OSINT Framework","source":"https://example.test/repo","version":"a744e613d7ded0aaa854896feb2a1069de34d2f8","importedAt":"2026-08-10","license":"MIT","extra":true}`,
		"remote source":   `{"name":"OSINT Framework","source":"http://example.test/repo","version":"a744e613d7ded0aaa854896feb2a1069de34d2f8","importedAt":"2026-08-10","license":"MIT"}`,
		"invalid version": `{"name":"OSINT Framework","source":"https://example.test/repo","version":"latest","importedAt":"2026-08-10","license":"MIT"}`,
		"invalid date":    `{"name":"OSINT Framework","source":"https://example.test/repo","version":"a744e613d7ded0aaa854896feb2a1069de34d2f8","importedAt":"10.08.2026","license":"MIT"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadCatalogMetadata([]byte(data)); err == nil {
				t.Fatal("expected invalid metadata error")
			}
		})
	}
}
