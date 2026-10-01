package screening

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testAccount(id, name, dob string) Account {
	parts := strings.Split(name, " ")
	account := Account{ID: id}
	switch len(parts) {
	case 1:
		account.FirstName = parts[0]
	case 2:
		account.FirstName, account.LastName = parts[0], parts[1]
	default:
		account.FirstName, account.MiddleName, account.LastName = parts[0], strings.Join(parts[1:len(parts)-1], " "), parts[len(parts)-1]
	}
	account.DOB, _ = time.Parse("2006-01-02", dob)
	return account
}

func TestServiceConfidenceEvidenceAndExplicitWeakPolicy(t *testing.T) {
	snapshot, err := LoadSDN(strings.NewReader(sdnFixture))
	if err != nil {
		t.Fatal(err)
	}
	accounts := []Account{
		testAccount("1", "Jane Anne Doe", "1976-09-08"),
		testAccount("2", "Jane Anne Doe", "1981-01-01"),
		testAccount("3", "Jane Anne Doe", "1990-01-01"),
		testAccount("4", "Jane Smith", "1990-01-01"),
	}
	for _, includeWeak := range []bool{false, true} {
		svc, err := NewService(accounts, snapshot, 4, includeWeak)
		if err != nil {
			t.Fatal(err)
		}
		full := svc.Screen(accounts[0])
		if len(full.Matches) != 1 || full.Matches[0].UID != "10" || full.Matches[0].Confidence != "HIGH" || !hasAll(full.Matches[0].MatchTypes, "NAME_EXACT", "DOB_FULL") {
			t.Fatalf("full DOB match = %#v", full)
		}
		year := svc.Screen(accounts[1])
		if year.Matches[0].Confidence != "HIGH" || !hasAll(year.Matches[0].MatchTypes, "NAME_EXACT", "DOB_YEAR") {
			t.Fatalf("exact plus year = %#v", year)
		}
		noDOBMatch := svc.Screen(accounts[2])
		if noDOBMatch.Matches[0].Confidence != "LOW" || !hasAll(noDOBMatch.Matches[0].MatchTypes, "NAME_EXACT") {
			t.Fatalf("exact without DOB = %#v", noDOBMatch)
		}
		weak := svc.Screen(accounts[3])
		if includeWeak && (len(weak.Matches) != 1 || weak.Matches[0].UID != "10") {
			t.Fatalf("weak alias should be included with explicit true: %#v", weak)
		}
		if !includeWeak && len(weak.Matches) != 0 {
			t.Fatalf("weak alias should be excluded with explicit false: %#v", weak)
		}
	}
}

func TestConfidenceRuleMatrix(t *testing.T) {
	fullDOB := calendar(t, "1990-04-05")
	for _, test := range []struct {
		name       string
		account    string
		dates      []BirthDate
		confidence string
		matchTypes []string
	}{
		{"fuzzy name only", "MARHTA", []BirthDate{{StartYear: 1970, EndYear: 1970}}, "LOW", []string{"NAME_FUZZY"}},
		{"fuzzy plus year", "MARHTA", []BirthDate{{StartYear: 1990, EndYear: 1990}}, "MED", []string{"NAME_FUZZY", "DOB_YEAR"}},
		{"fuzzy plus full DOB", "MARHTA", []BirthDate{{Full: fullDOB, StartYear: 1990, EndYear: 1990}}, "HIGH", []string{"NAME_FUZZY", "DOB_FULL"}},
		{"exact name only", "martha", []BirthDate{{StartYear: 1970, EndYear: 1970}}, "LOW", []string{"NAME_EXACT"}},
		{"exact plus year", "martha", []BirthDate{{StartYear: 1990, EndYear: 1990}}, "HIGH", []string{"NAME_EXACT", "DOB_YEAR"}},
		{"exact plus full DOB", "martha", []BirthDate{{Full: fullDOB, StartYear: 1990, EndYear: 1990}}, "HIGH", []string{"NAME_EXACT", "DOB_FULL"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			account := testAccount("account", test.account, "1990-04-05")
			snapshot := Snapshot{Records: []Record{{UID: "entry", Type: "Individual", Names: []Name{{Text: "MARTHA", Category: "primary"}}, DOBs: test.dates}}}
			service, err := NewService([]Account{account}, snapshot, 0, true)
			if err != nil {
				t.Fatal(err)
			}
			result := service.Screen(account)
			if len(result.Matches) != 1 {
				t.Fatalf("matches = %#v", result.Matches)
			}
			match := result.Matches[0]
			if match.Confidence != test.confidence || !hasAll(match.MatchTypes, test.matchTypes...) || len(match.MatchTypes) != len(test.matchTypes) {
				t.Fatalf("match = %#v, want confidence %s and types %v", match, test.confidence, test.matchTypes)
			}
		})
	}
}

func TestAllRecordTypesAndNameCategoriesAreEligible(t *testing.T) {
	account := testAccount("account", "Shared Target", "1990-04-05")
	snapshot := Snapshot{Records: []Record{
		{UID: "1", Type: "Individual", Names: []Name{{Text: "Shared Target", Category: "primary"}}},
		{UID: "2", Type: "Entity", Names: []Name{{Text: "Other Entity", Category: "primary"}, {Text: "Shared Target", Category: "strong"}}},
		{UID: "3", Type: "Vessel", Names: []Name{{Text: "Ocean Vessel", Category: "primary"}, {Text: "Shared Target", Category: "weak"}}},
		{UID: "4", Type: "Aircraft", Names: []Name{{Text: "Shared Target", Category: "primary"}}},
	}}

	for _, test := range []struct {
		name        string
		includeWeak bool
		wantUIDs    []string
	}{
		{"include weak aliases", true, []string{"1", "2", "3", "4"}},
		{"exclude weak aliases", false, []string{"1", "2", "4"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewService([]Account{account}, snapshot, 0, test.includeWeak)
			if err != nil {
				t.Fatal(err)
			}
			matches := service.Screen(account).Matches
			if len(matches) != len(test.wantUIDs) {
				t.Fatalf("matches = %#v", matches)
			}
			for i, uid := range test.wantUIDs {
				if matches[i].UID != uid || matches[i].Confidence != "LOW" || !hasAll(matches[i].MatchTypes, "NAME_EXACT") {
					t.Fatalf("match %d = %#v, want UID %s", i, matches[i], uid)
				}
			}
		})
	}
}

func TestFullNameThresholdIsInclusive(t *testing.T) {
	account := Account{ID: "account", FirstName: "Morgan", LastName: "Stone", DOB: calendar(t, "1990-04-05")}
	snapshot := Snapshot{Records: []Record{{UID: "vessel", Type: "Vessel", Names: []Name{{Text: "MORGAN", Category: "primary"}}}}}
	service, err := NewService([]Account{account}, snapshot, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	result := service.Screen(account)
	if len(result.Matches) != 1 || result.Matches[0].UID != "vessel" || result.Matches[0].Confidence != "LOW" || !hasAll(result.Matches[0].MatchTypes, "NAME_FUZZY") {
		t.Fatalf("0.90 full-name boundary match = %#v", result)
	}
}

func hasAll(values []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, v := range values {
			if v == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestCacheImmutabilityEvictionAndDOBIsolation(t *testing.T) {
	cache := newCandidateCache(2)
	original := []candidate{{UID: "a", Score: .91}}
	cache.Put("first", original)
	original[0].UID = "changed"
	got, ok := cache.Get("first")
	if !ok || got[0].UID != "a" {
		t.Fatalf("cache retained caller mutation: %#v %v", got, ok)
	}
	got[0].UID = "also changed"
	again, _ := cache.Get("first")
	if again[0].UID != "a" {
		t.Fatalf("cache returned mutable internal slice: %#v", again)
	}
	cache.Put("second", nil)
	_, _ = cache.Get("first")
	cache.Put("third", nil)
	if _, ok := cache.Get("second"); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	if _, ok := cache.Get("first"); !ok {
		t.Fatal("recently used entry was evicted")
	}

	snapshot, err := LoadSDN(strings.NewReader(sdnFixture))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService([]Account{testAccount("a", "Jane Anne Doe", "1976-09-08"), testAccount("b", "Jane Anne Doe", "1990-01-01")}, snapshot, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	a, b := svc.Screen(svc.accounts[0]), svc.Screen(svc.accounts[1])
	if a.Matches[0].Confidence != "HIGH" || b.Matches[0].Confidence != "LOW" {
		t.Fatalf("DOB-specific evidence leaked through name cache: %#v / %#v", a, b)
	}
}

func TestHTTPHandlerBulkSingleUnknownAndEmptyMatches(t *testing.T) {
	snapshot, err := LoadSDN(strings.NewReader(sdnFixture))
	if err != nil {
		t.Fatal(err)
	}
	accounts := []Account{testAccount("b", "Nobody Else", "2000-01-01"), testAccount("a", "Jane Anne Doe", "1976-09-08")}
	svc, err := NewService(accounts, snapshot, 3, false)
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	svc.SetLogger(log.New(&logs, "", 0))
	h := svc.Handler()
	request := httptest.NewRequest(http.MethodPost, "/screenings", nil)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("bulk status %d: %s", response.Code, response.Body)
	}
	var bulk []Result
	if err := json.Unmarshal(response.Body.Bytes(), &bulk); err != nil {
		t.Fatal(err)
	}
	if len(bulk) != 2 || bulk[0].AccountID != "b" || bulk[1].AccountID != "a" {
		t.Fatalf("bulk order: %#v", bulk)
	}
	if bulk[0].Matches == nil || len(bulk[0].Matches) != 0 {
		t.Fatalf("no-match list must be []: %#v", bulk[0])
	}
	if output := logs.String(); !strings.Contains(output, "bulk screening started") || !strings.Contains(output, "bulk screening progress") || !strings.Contains(output, "bulk screening completed") {
		t.Fatalf("missing bulk progress logs: %q", output)
	}

	request = httptest.NewRequest(http.MethodGet, "/screenings/a", nil)
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	var single Result
	if err := json.Unmarshal(response.Body.Bytes(), &single); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || single.AccountID != "a" || len(single.Matches) != 1 {
		t.Fatalf("single response %d: %#v", response.Code, single)
	}
	if !strings.Contains(logs.String(), `single screening completed: account_id="a"`) {
		t.Fatalf("missing single screening log: %q", logs.String())
	}
	if output := logs.String(); !strings.Contains(output, `uid="10"`) || !strings.Contains(output, `matched_name="Jane Anne Doe"`) || !strings.Contains(output, `name_category="primary"`) || !strings.Contains(output, `score=1.000000`) {
		t.Fatalf("missing single-match evidence log: %q", output)
	}
	request = httptest.NewRequest(http.MethodGet, "/screenings/missing", nil)
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("unknown response %d: %s", response.Code, response.Body)
	}
	request = httptest.NewRequest(http.MethodPost, "/screenings", strings.NewReader("{}"))
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("body should be rejected, status %d", response.Code)
	}
}

func TestBulkScreeningStopsWhenRequestIsCanceled(t *testing.T) {
	snapshot, err := LoadSDN(strings.NewReader(sdnFixture))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService([]Account{testAccount("a", "Jane Anne Doe", "1976-09-08")}, snapshot, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if results, err := svc.screenAll(ctx); err != context.Canceled || results != nil {
		t.Fatalf("canceled bulk result = %#v, %v", results, err)
	}
}
