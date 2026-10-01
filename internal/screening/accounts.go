package screening

import (
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

func joinName(parts ...string) string {
	var present []string
	for _, part := range parts {
		if part != "" {
			present = append(present, part)
		}
	}
	return strings.Join(present, " ")
}

// Name composes the account's populated name fields in source order.
func (a Account) Name() string { return joinName(a.FirstName, a.MiddleName, a.LastName) }

// LoadAccounts validates and parses account CSV data in source order.
func LoadAccounts(r io.Reader) ([]Account, error) {
	reader := csv.NewReader(r)
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV header: %w", err)
	}
	want := []string{"account_id", "first_name", "middle_name", "last_name", "dob", "country", "employer"}
	if !slices.Equal(header, want) {
		return nil, fmt.Errorf("CSV header: expected %s", strings.Join(want, ","))
	}
	accounts := make([]Account, 0)
	seen := make(map[string]int)
	for row := 2; ; row++ {
		cells, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV row %d: %w", row, err)
		}
		for i, cell := range cells {
			if !utf8.ValidString(cell) {
				return nil, fmt.Errorf("CSV row %d field %s: must be valid UTF-8", row, want[i])
			}
			if strings.TrimSpace(cell) != cell {
				return nil, fmt.Errorf("CSV row %d field %s: outer whitespace is not allowed", row, want[i])
			}
			if i != 2 && cell == "" {
				return nil, fmt.Errorf("CSV row %d field %s: value is required", row, want[i])
			}
		}
		if first, ok := seen[cells[0]]; ok {
			return nil, fmt.Errorf("CSV row %d field account_id: duplicate %q (first seen row %d)", row, cells[0], first)
		}
		dob, err := time.Parse("2006-01-02", cells[4])
		if err != nil || dob.Year() < 1 {
			return nil, fmt.Errorf("CSV row %d field dob: invalid ISO calendar date %q", row, cells[4])
		}
		seen[cells[0]] = row
		accounts = append(accounts, Account{ID: cells[0], FirstName: cells[1], MiddleName: cells[2], LastName: cells[3], DOB: dob, Country: cells[5], Employer: cells[6]})
	}
	return accounts, nil
}
