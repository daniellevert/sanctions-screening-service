package screening

import (
	"os"
	"reflect"
	"testing"
)

func loadSyntheticDataset(t *testing.T, accountsPath string) ([]Account, Snapshot) {
	t.Helper()
	accountsFile, err := os.Open(accountsPath)
	if err != nil {
		t.Fatal(err)
	}
	defer accountsFile.Close()
	accounts, err := LoadAccounts(accountsFile)
	if err != nil {
		t.Fatal(err)
	}

	sdnFile, err := os.Open("../../testdata/sdn.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer sdnFile.Close()
	snapshot, err := LoadSDN(sdnFile)
	if err != nil {
		t.Fatal(err)
	}
	return accounts, snapshot
}

func TestSyntheticDatasetExamples(t *testing.T) {
	accounts, snapshot := loadSyntheticDataset(t, "../../data/accounts.csv")
	service, err := NewService(accounts, snapshot, 0, true)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		accountID string
		matches   []Match
	}{
		{"demo-100", []Match{{UID: "10001", Confidence: "HIGH", MatchTypes: []string{"NAME_EXACT", "DOB_FULL"}}}},
		{"demo-101", []Match{{UID: "10002", Confidence: "MED", MatchTypes: []string{"NAME_FUZZY", "DOB_YEAR"}}}},
		{"demo-102", []Match{{UID: "10001", Confidence: "LOW", MatchTypes: []string{"NAME_EXACT"}}}},
		{"demo-103", []Match{
			{UID: "10003", Confidence: "LOW", MatchTypes: []string{"NAME_EXACT"}},
			{UID: "10004", Confidence: "LOW", MatchTypes: []string{"NAME_EXACT"}},
		}},
		{"demo-104", []Match{}},
		{"demo-105", []Match{{UID: "10006", Confidence: "HIGH", MatchTypes: []string{"NAME_EXACT", "DOB_FULL"}}}},
	}
	for _, test := range tests {
		t.Run(test.accountID, func(t *testing.T) {
			account, ok := service.byAccountID[test.accountID]
			if !ok {
				t.Fatalf("account %s is missing", test.accountID)
			}
			if got := service.Screen(account).Matches; !reflect.DeepEqual(got, test.matches) {
				t.Fatalf("matches = %#v, want %#v", got, test.matches)
			}
		})
	}
}

func TestWeakAliasFlagExample(t *testing.T) {
	accounts, snapshot := loadSyntheticDataset(t, "../../testdata/weak-alias-accounts.csv")
	included, err := NewService(accounts, snapshot, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []Match{{UID: "10001", Confidence: "LOW", MatchTypes: []string{"NAME_EXACT"}}}
	if got := included.Screen(accounts[0]).Matches; !reflect.DeepEqual(got, want) {
		t.Fatalf("included weak alias matches = %#v, want %#v", got, want)
	}

	excluded, err := NewService(accounts, snapshot, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := excluded.Screen(accounts[0]).Matches; len(got) != 0 {
		t.Fatalf("excluded weak alias matches = %#v, want none", got)
	}
}
