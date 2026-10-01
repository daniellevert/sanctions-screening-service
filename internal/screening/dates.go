package screening

import (
	"fmt"
	"strings"
	"time"
)

// datePart parses a calendar value without making a partial value a full DOB.
func datePart(value string) (time.Time, bool, error) {
	for _, layout := range []string{"02 Jan 2006", "Jan 2006", "2006"} {
		if parsed, err := time.Parse(layout, value); err == nil && parsed.Year() > 0 && parsed.Format(layout) == value {
			return parsed, layout == "02 Jan 2006", nil
		}
	}
	return time.Time{}, false, fmt.Errorf("unsupported or invalid DOB %q", value)
}

// ParseBirthDate parses the date precisions present in the standard SDN XML.
func ParseBirthDate(raw string) (BirthDate, error) {
	value := strings.TrimSpace(raw)
	approximate := strings.HasPrefix(value, "circa ")
	if approximate {
		value = strings.TrimPrefix(value, "circa ")
	}
	parts := strings.Split(value, " to ")
	if len(parts) > 2 {
		return BirthDate{}, fmt.Errorf("unsupported DOB %q", raw)
	}
	start, full, err := datePart(parts[0])
	if err != nil {
		return BirthDate{}, err
	}
	dob := BirthDate{Raw: raw, StartYear: start.Year(), EndYear: start.Year()}
	if len(parts) == 2 {
		end, _, err := datePart(parts[1])
		if err != nil {
			return BirthDate{}, err
		}
		if end.Before(start) {
			return BirthDate{}, fmt.Errorf("reversed DOB range %q", raw)
		}
		dob.EndYear = end.Year()
	} else if full && !approximate {
		dob.Full = start
	}
	return dob, nil
}

// DOBEvidence returns the strongest date evidence across all SDN alternatives.
func DOBEvidence(account time.Time, dates []BirthDate) string {
	evidence := ""
	for _, dob := range dates {
		if !dob.Full.IsZero() && account.Year() == dob.Full.Year() && account.Month() == dob.Full.Month() && account.Day() == dob.Full.Day() {
			return "DOB_FULL"
		}
		if account.Year() >= dob.StartYear && account.Year() <= dob.EndYear {
			evidence = "DOB_YEAR"
		}
	}
	return evidence
}
