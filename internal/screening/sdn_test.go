package screening

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sdnFixture = `<?xml version="1.0"?>
<sdnList xmlns="http://www.example.com/sdn">
  <publshInformation><Publish_Date>15 Jan 2025</Publish_Date></publshInformation>
  <sdnEntry><uid>10</uid><firstName>Jane Anne</firstName><lastName>Doe</lastName><sdnType>Individual</sdnType>
    <akaList><aka><uid>999</uid><firstName>J. A.</firstName><lastName>Doe</lastName><category>strong</category></aka><aka><firstName>Jane</firstName><lastName>Smith</lastName><category>weak</category></aka></akaList>
    <dateOfBirthList><dateOfBirthItem><dateOfBirth>08 Sep 1976</dateOfBirth><mainEntry>false</mainEntry></dateOfBirthItem><dateOfBirthItem><dateOfBirth>1981</dateOfBirth></dateOfBirthItem></dateOfBirthList>
  </sdnEntry>
  <sdnEntry><uid>20</uid><lastName>Acme</lastName><sdnType>Entity</sdnType></sdnEntry>
  <sdnEntry><uid>30</uid><firstName>Sea</firstName><lastName>Star</lastName><sdnType>Vessel</sdnType></sdnEntry>
  <sdnEntry><uid>40</uid><firstName>Jet</firstName><lastName>One</lastName><sdnType>Aircraft</sdnType></sdnEntry>
</sdnList>`

func TestLoadSDNAllTypesAliasesAndDirectUID(t *testing.T) {
	snapshot, err := LoadSDN(strings.NewReader(sdnFixture))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Published != "15 Jan 2025" {
		t.Fatalf("publication metadata = %q", snapshot.Published)
	}
	if len(snapshot.Records) != 4 {
		t.Fatalf("got %d records", len(snapshot.Records))
	}
	wantTypes := []string{"Individual", "Entity", "Vessel", "Aircraft"}
	for i, want := range wantTypes {
		if snapshot.Records[i].Type != want {
			t.Errorf("record %d type = %q, want %q", i, snapshot.Records[i].Type, want)
		}
	}
	r := snapshot.Records[0]
	if r.UID != "10" {
		t.Fatalf("entry uid = %q; nested alias UID must be ignored", r.UID)
	}
	if len(r.Names) != 3 || r.Names[0] != (Name{Text: "Jane Anne Doe", Category: "primary"}) || r.Names[1] != (Name{Text: "J. A. Doe", Category: "strong"}) || r.Names[2] != (Name{Text: "Jane Smith", Category: "weak"}) {
		t.Fatalf("names: %#v", r.Names)
	}
	if len(r.DOBs) != 2 || r.DOBs[0].Full.IsZero() || r.DOBs[0].StartYear != 1976 || r.DOBs[1].StartYear != 1981 {
		t.Fatalf("alternative DOBs were not retained: %#v", r.DOBs)
	}
	if len(snapshot.Records[1].DOBs) != 0 {
		t.Fatal("missing DOB should be accepted")
	}
}

func TestLoadSDNRejectsBadOrDuplicateEntries(t *testing.T) {
	for name, xml := range map[string]string{
		"bad root":           `<root><sdnEntry><uid>1</uid><lastName>A</lastName></sdnEntry></root>`,
		"duplicate UID":      `<sdnList><sdnEntry><uid>1</uid><lastName>A</lastName></sdnEntry><sdnEntry><uid>1</uid><lastName>B</lastName></sdnEntry></sdnList>`,
		"missing UID":        `<sdnList><sdnEntry><lastName>A</lastName></sdnEntry></sdnList>`,
		"missing name":       `<sdnList><sdnEntry><uid>1</uid></sdnEntry></sdnList>`,
		"empty alias":        `<sdnList><sdnEntry><uid>1</uid><lastName>A</lastName><akaList><aka><category>strong</category></aka></akaList></sdnEntry></sdnList>`,
		"bad alias category": `<sdnList><sdnEntry><uid>1</uid><lastName>A</lastName><akaList><aka><firstName>B</firstName><category>other</category></aka></akaList></sdnEntry></sdnList>`,
		"bad DOB":            `<sdnList><sdnEntry><uid>1</uid><lastName>A</lastName><dateOfBirthList><dateOfBirthItem><dateOfBirth>unknown</dateOfBirth></dateOfBirthItem></dateOfBirthList></sdnEntry></sdnList>`,
		"malformed XML":      `<sdnList><sdnEntry>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadSDN(strings.NewReader(xml)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDownloadSDNRedirectAndAtomicPreservation(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(sdnFixture)) }))
	defer final.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, final.URL, http.StatusFound) }))
	defer redirect.Close()
	dest := filepath.Join(t.TempDir(), "sdn.xml")
	if err := os.WriteFile(dest, []byte("old snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := DownloadSDN(context.Background(), redirect.Client(), redirect.URL, dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Records) != 4 {
		t.Fatalf("download snapshot has %d records", len(snapshot.Records))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "<sdnList") {
		t.Fatal("destination was not replaced with validated XML")
	}

	for name, handler := range map[string]http.HandlerFunc{
		"status":    func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusServiceUnavailable) },
		"malformed": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<sdnList><sdnEntry>")) },
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(handler)
			defer srv.Close()
			before, err := os.ReadFile(dest)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DownloadSDN(context.Background(), srv.Client(), srv.URL, dest); err == nil {
				t.Fatal("expected download error")
			}
			after, err := os.ReadFile(dest)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(before) {
				t.Fatal("failed refresh changed existing snapshot")
			}
		})
	}
}
