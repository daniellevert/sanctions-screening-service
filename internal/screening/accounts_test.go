package screening

import (
	"strings"
	"testing"
)

const accountHeader = "account_id,first_name,middle_name,last_name,dob,country,employer\n"

func TestLoadAccountsPreservesValuesAndOrder(t *testing.T) {
	rows := accountHeader + "010,Renée,Élise,Dubois-Martin,1980-02-29,FR,Example Ltd\n2,Ada,,Lovelace,1815-12-10,GB,Analytical Engines\n"
	got, err := LoadAccounts(strings.NewReader(rows))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "010" || got[1].ID != "2" {
		t.Fatalf("lost IDs/order: %#v", got)
	}
	if got[0].Name() != "Renée Élise Dubois-Martin" || got[1].Name() != "Ada Lovelace" {
		t.Fatalf("incorrect name composition: %#v", got)
	}
	if got[0].Country != "FR" || got[0].Employer != "Example Ltd" || got[0].DOB.Format("2006-01-02") != "1980-02-29" {
		t.Fatalf("source values changed: %#v", got[0])
	}
}

func TestLoadAccountsRejectsInvalidRows(t *testing.T) {
	valid := "1,Jane,,Smith,1980-01-02,US,Acme\n"
	tests := map[string]string{
		"duplicate ID":       accountHeader + valid + valid,
		"missing ID":         accountHeader + ",Jane,,Smith,1980-01-02,US,Acme\n",
		"missing first":      accountHeader + "1,,,Smith,1980-01-02,US,Acme\n",
		"missing last":       accountHeader + "1,Jane,,,1980-01-02,US,Acme\n",
		"missing DOB":        accountHeader + "1,Jane,,Smith,,US,Acme\n",
		"missing country":    accountHeader + "1,Jane,,Smith,1980-01-02,,Acme\n",
		"missing employer":   accountHeader + "1,Jane,,Smith,1980-01-02,US,\n",
		"invalid calendar":   accountHeader + "1,Jane,,Smith,1981-02-29,US,Acme\n",
		"non ISO DOB":        accountHeader + "1,Jane,,Smith,02 Jan 1980,US,Acme\n",
		"extra field":        accountHeader + strings.TrimSpace(valid) + ",extra\n",
		"missing field":      accountHeader + "1,Jane,,Smith,1980-01-02,US\n",
		"outer whitespace":   accountHeader + "1, Jane,,Smith,1980-01-02,US,Acme\n",
		"whitespace middle":  accountHeader + "1,Jane, ,Smith,1980-01-02,US,Acme\n",
		"wrong header":       "id,first_name,middle_name,last_name,dob,country,employer\n" + valid,
		"wrong column order": "first_name,account_id,middle_name,last_name,dob,country,employer\n" + valid,
		"malformed CSV":      accountHeader + "1,\"Jane,,Smith,1980-01-02,US,Acme\n",
		"invalid UTF-8":      accountHeader + "1,Ja" + string([]byte{0xff}) + "ne,,Smith,1980-01-02,US,Acme\n",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := LoadAccounts(strings.NewReader(input))
			if err == nil {
				t.Fatal("expected startup validation failure")
			}
			if name != "wrong header" && name != "wrong column order" && !strings.Contains(err.Error(), "2") && !strings.Contains(err.Error(), "3") {
				t.Errorf("diagnostic lacks row context: %v", err)
			}
		})
	}
}
