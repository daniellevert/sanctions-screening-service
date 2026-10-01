package screening

import (
	"testing"
	"time"
)

func mustDOB(t *testing.T, raw string) BirthDate {
	t.Helper()
	b, err := ParseBirthDate(raw)
	if err != nil {
		t.Fatalf("ParseBirthDate(%q): %v", raw, err)
	}
	return b
}

func calendar(t *testing.T, raw string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDOBEvidencePrecisionAndAlternatives(t *testing.T) {
	for _, tc := range []struct {
		name, account string
		raw           []string
		want          string
	}{
		{"full", "1976-09-08", []string{"08 Sep 1976"}, "DOB_FULL"},
		{"same year different day", "1976-09-09", []string{"08 Sep 1976"}, "DOB_YEAR"},
		{"year only", "1981-01-01", []string{"1981"}, "DOB_YEAR"},
		{"missing", "1981-01-01", nil, ""},
		{"different year", "1980-01-01", []string{"1981"}, ""},
		{"alternative full wins", "1976-09-08", []string{"1976", "08 Sep 1976", "1981"}, "DOB_FULL"},
		{"alternative year", "1981-09-08", []string{"08 Sep 1976", "1981"}, "DOB_YEAR"},
		{"range lower inclusive", "1970-01-01", []string{"1970 to 1975"}, "DOB_YEAR"},
		{"range upper inclusive", "1975-12-31", []string{"1970 to 1975"}, "DOB_YEAR"},
		{"range outside", "1976-01-01", []string{"1970 to 1975"}, ""},
		{"full range uncertain", "1976-09-08", []string{"08 Sep 1976 to 08 Sep 1976"}, "DOB_YEAR"},
		{"date range uses year", "1976-01-01", []string{"08 Sep 1976 to 10 Oct 1977"}, "DOB_YEAR"},
		{"circa exact stated year", "1981-01-01", []string{"circa 1981"}, "DOB_YEAR"},
		{"circa no tolerance", "1980-12-31", []string{"circa 1981"}, ""},
		{"month year", "1976-09-08", []string{"Sep 1976"}, "DOB_YEAR"},
		{"approximate full", "1976-09-08", []string{"circa 08 Sep 1976"}, "DOB_YEAR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var births []BirthDate
			for _, raw := range tc.raw {
				b := mustDOB(t, raw)
				if b.Raw != raw {
					t.Errorf("lost original date %q: %#v", raw, b)
				}
				births = append(births, b)
			}
			if got := DOBEvidence(calendar(t, tc.account), births); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestParseBirthDateRejectsUnsupportedAndInvalid(t *testing.T) {
	for _, raw := range []string{"", "unknown", "1970 or 1975", "31 Feb 1980", "29 Feb 1981", "Foo 1980", "1975 to 1970", "10 Sep 1976 to 08 Sep 1976", "0000", "circa 31 Feb 1980"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseBirthDate(raw); err == nil {
				t.Fatalf("accepted invalid/unsupported DOB %q", raw)
			}
		})
	}
	if got := mustDOB(t, "29 Feb 1980"); got.Full.IsZero() {
		t.Fatal("valid leap day lost full precision")
	}
}
