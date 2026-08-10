package sherlock

import (
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
