package screening

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SDNURL is OFAC's stable endpoint for the complete standard SDN XML.
const SDNURL = "https://sanctionslistservice.ofac.treas.gov/api/download/SDN.XML"

type xmlEntry struct {
	UID     string `xml:"uid"`
	First   string `xml:"firstName"`
	Last    string `xml:"lastName"`
	Type    string `xml:"sdnType"`
	Aliases []struct {
		First    string `xml:"firstName"`
		Last     string `xml:"lastName"`
		Category string `xml:"category"`
	} `xml:"akaList>aka"`
	Dates []string `xml:"dateOfBirthList>dateOfBirthItem>dateOfBirth"`
}

// LoadSDN streams the standard SDN XML through encoding/xml, retaining only
// fields needed for screening and reporting validation errors with entry context.
func LoadSDN(r io.Reader) (Snapshot, error) {
	decoder := xml.NewDecoder(r)
	var snapshot Snapshot
	seen := make(map[string]bool)
	depth := 0
	rootSeen := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Snapshot{}, fmt.Errorf("SDN XML: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				if rootSeen || element.Name.Local != "sdnList" {
					return Snapshot{}, fmt.Errorf("SDN XML: expected one sdnList root")
				}
				rootSeen = true
			}
			if depth != 2 {
				continue
			}
			switch element.Name.Local {
			case "publshInformation":
				var info struct {
					Published string `xml:"Publish_Date"`
				}
				if err := decoder.DecodeElement(&info, &element); err != nil {
					return Snapshot{}, fmt.Errorf("SDN publication: %w", err)
				}
				snapshot.Published = info.Published
				depth--
			case "sdnEntry":
				var entry xmlEntry
				if err := decoder.DecodeElement(&entry, &element); err != nil {
					return Snapshot{}, fmt.Errorf("SDN entry %d: %w", len(snapshot.Records)+1, err)
				}
				depth--
				if strings.TrimSpace(entry.UID) == "" || strings.TrimSpace(entry.UID) != entry.UID {
					return Snapshot{}, fmt.Errorf("SDN entry %d: missing or invalid uid", len(snapshot.Records)+1)
				}
				if seen[entry.UID] {
					return Snapshot{}, fmt.Errorf("SDN duplicate uid %q", entry.UID)
				}
				seen[entry.UID] = true
				name := joinName(entry.First, entry.Last)
				if strings.TrimSpace(name) == "" {
					return Snapshot{}, fmt.Errorf("SDN uid %s: missing primary name", entry.UID)
				}
				record := Record{UID: entry.UID, Type: entry.Type, Names: []Name{{Text: name, Category: "primary"}}}
				for _, alias := range entry.Aliases {
					name := joinName(alias.First, alias.Last)
					if strings.TrimSpace(name) == "" {
						return Snapshot{}, fmt.Errorf("SDN uid %s: empty alias", entry.UID)
					}
					if alias.Category != "strong" && alias.Category != "weak" {
						return Snapshot{}, fmt.Errorf("SDN uid %s: unsupported alias category %q", entry.UID, alias.Category)
					}
					record.Names = append(record.Names, Name{Text: name, Category: alias.Category})
				}
				for _, raw := range entry.Dates {
					dob, err := ParseBirthDate(raw)
					if err != nil {
						return Snapshot{}, fmt.Errorf("SDN uid %s: %w", entry.UID, err)
					}
					record.DOBs = append(record.DOBs, dob)
				}
				snapshot.Records = append(snapshot.Records, record)
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(element)) != "" {
				return Snapshot{}, fmt.Errorf("SDN XML: text outside root")
			}
		}
	}
	if !rootSeen || len(snapshot.Records) == 0 {
		return Snapshot{}, fmt.Errorf("SDN XML: no records")
	}
	return snapshot, nil
}

// DownloadSDN saves only a completely downloaded and validated snapshot.
// The caller supplies a client with a timeout; normal http.Client redirect rules apply.
func DownloadSDN(ctx context.Context, client *http.Client, url, path string) (Snapshot, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("SDN request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return Snapshot{}, fmt.Errorf("SDN download: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("SDN download: HTTP %d", response.StatusCode)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".sdn-*.tmp")
	if err != nil {
		return Snapshot{}, fmt.Errorf("SDN temporary file: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := io.Copy(file, response.Body); err != nil {
		return Snapshot{}, fmt.Errorf("SDN download body: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Snapshot{}, err
	}
	snapshot, err := LoadSDN(file)
	if err != nil {
		return Snapshot{}, err
	}
	if err := file.Sync(); err != nil {
		return Snapshot{}, err
	}
	if err := file.Close(); err != nil {
		return Snapshot{}, err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return Snapshot{}, fmt.Errorf("SDN save: %w", err)
	}
	return snapshot, nil
}
