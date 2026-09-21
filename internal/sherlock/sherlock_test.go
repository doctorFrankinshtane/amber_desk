package sherlock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	input := "username,name,url_main,url_user,exists,http_status,response_time_s\nhandle,GitHub,https://github.com,https://github.com/handle,Claimed,200,0.125\nhandle,Example,https://example.com,https://example.com/handle,Available,404,0.5\n"
	report, err := ParseCSV(strings.NewReader(input), "handle")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 2 || report.Results[0].Status != "claimed" || report.Results[0].ResponseTimeMS != 125 || report.Results[0].ID == "" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestParseCSVRejectsUnsafeURL(t *testing.T) {
	input := "username,name,url_main,url_user,exists,http_status,response_time_s\nhandle,Bad,https://example.com,javascript:alert(1),Claimed,200,0.1\n"
	if _, err := ParseCSV(strings.NewReader(input), "handle"); err == nil {
		t.Fatal("expected unsafe URL error")
	}
}

func TestParseCSVAllowsMissingURLForUnavailableSite(t *testing.T) {
	input := "username,name,url_main,url_user,exists,http_status,response_time_s\nhandle,Rate Limited,,,Unknown,429,0.1\nhandle,Illegal,,,Illegal,0,\n"
	report, err := ParseCSV(strings.NewReader(input), "handle")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 2 || report.Results[0].ProfileURL != "" || report.Results[0].Status != "unknown" || report.Results[1].Status != "illegal" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestValidUsername(t *testing.T) {
	for _, value := range []string{"handle", "john.doe", "name_1", "a-b"} {
		if !ValidUsername(value) {
			t.Fatalf("expected %q to be valid", value)
		}
	}
	for _, value := range []string{"", "has space", "x;whoami", strings.Repeat("a", 101)} {
		if ValidUsername(value) {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestDefaultPythonPrefersRuntimeNextToExecutable(t *testing.T) {
	relative := defaultPython()
	if filepath.IsAbs(relative) {
		t.Fatalf("expected working-directory fallback, got %q", relative)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	root := filepath.Join(filepath.Dir(exe), ".tools")
	if _, err := os.Stat(root); err == nil {
		t.Skip(".tools already exists next to test binary")
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	bundled := filepath.Join(filepath.Dir(exe), relative)
	if err := os.MkdirAll(filepath.Dir(bundled), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundled, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := defaultPython(); got != bundled {
		t.Fatalf("defaultPython() = %q, want %q", got, bundled)
	}
}
