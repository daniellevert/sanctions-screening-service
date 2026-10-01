package screening

import "time"

// Account is one validated source row from the account CSV.
type Account struct {
	ID         string
	FirstName  string
	MiddleName string
	LastName   string
	DOB        time.Time
	Country    string
	Employer   string
}

// Name is one searchable primary name or alias associated with an SDN record.
type Name struct {
	Text     string
	Category string // primary, strong, or weak
}

// BirthDate preserves the original value and its available precision.
type BirthDate struct {
	Raw       string
	Full      time.Time // zero for uncertain or partial dates
	StartYear int
	EndYear   int
}

// Record contains the screening fields retained from one top-level SDN entry.
type Record struct {
	UID   string
	Type  string
	Names []Name
	DOBs  []BirthDate
}

// Snapshot is one fully parsed SDN publication loaded by a Service.
// Keeping publication metadata and records together makes the service and its
// cache belong to one immutable version of the source list.
type Snapshot struct {
	Published string
	Records   []Record
}

// Result is the response for one screened account.
type Result struct {
	AccountID string  `json:"account_id"`
	Matches   []Match `json:"matches"`
}

// Match describes one qualifying top-level SDN entry.
type Match struct {
	UID        string   `json:"uid"`
	Confidence string   `json:"confidence"`
	MatchTypes []string `json:"match_types"`
}
