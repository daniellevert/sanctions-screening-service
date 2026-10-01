# Sanctions screening service

A local Go HTTP service for screening account names against the U.S. Treasury's Specially Designated Nationals (SDN) list. It supports complete-dataset and single-account screening, deterministic confidence rules, aliases, partial dates of birth, request cancellation, and a bounded candidate cache.

> This repository is an engineering demonstration, not a compliance product or legal guidance. Real sanctions programs require organization-specific policies, broader identifiers, audit controls, list-management procedures, and human review.

## Why this stack

Go provides strong standard packages for HTTP, CSV, XML, JSON, and dates, along with straightforward concurrency, race detection, and a single compiled executable. [`go-edlib`](https://github.com/hbollon/go-edlib) supplies a tested, pure-Go Jaro-Winkler implementation so this project does not maintain its own string-scoring algorithm. [`DESIGN.md`](DESIGN.md) describes the architecture, technology tradeoffs, and policy assumptions.

## Quick start with synthetic data

Install [Go 1.22 or newer](https://go.dev/dl/), then run the service against the included synthetic fixture:

```sh
go mod download
go run ./cmd/screening \
  -accounts data/accounts.csv \
  -sdn testdata/sdn.xml \
  -listen 127.0.0.1:8080
```

In another terminal:

```sh
curl -X POST http://127.0.0.1:8080/screenings
curl http://127.0.0.1:8080/screenings/demo-100
```

`demo-100` produces an exact-name, full-DOB match:

```json
{"account_id":"demo-100","matches":[{"uid":"10001","confidence":"HIGH","match_types":["NAME_EXACT","DOB_FULL"]}]}
```

Every name and identifier under `data/` and `testdata/` is synthetic.

## Run with the current OFAC list

Omit `-sdn` to download, validate, and atomically save the current full SDN XML:

```sh
go run ./cmd/screening \
  -accounts data/accounts.csv \
  -listen 127.0.0.1:8080
```

The downloaded `data/sdn.xml` is intentionally excluded from Git. A failed download or invalid XML stops startup; the service never silently substitutes an older file.

## Command-line flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `-accounts <path>` | `data/accounts.csv` | Account CSV loaded and validated at startup. |
| `-sdn <path>` | none | Existing local SDN XML. Supplying it skips the network download and ignores `-snapshot`. |
| `-snapshot <path>` | `data/sdn.xml` | Destination for a downloaded and validated SDN snapshot. |
| `-listen <host:port>` | `127.0.0.1:8080` | HTTP server address and port. |
| `-cache-size <nonnegative-integer>` | `16000` | Maximum cached normalized-name searches. Use `0` to disable caching. |
| `-weak-aliases <include\|exclude>` | `include` | Include or exclude OFAC aliases categorized as weak. |
| `-h` | — | Print command-line help and exit. `--help` is equivalent. |

## API

| Action | Method and path | Request | Response |
| --- | --- | --- | --- |
| Screen all accounts | `POST /screenings` | No body | JSON array in CSV order |
| Screen one account | `GET /screenings/{account_id}` | No body | One JSON result or HTTP 404 |

Each result has a non-null `matches` array. Each match returns the containing top-level SDN entry UID:

```json
{
  "account_id": "demo-101",
  "matches": [
    {
      "uid": "10002",
      "confidence": "MED",
      "match_types": ["NAME_FUZZY", "DOB_YEAR"]
    }
  ]
}
```

Bulk screening is synchronous and logs start, progress, cancellation, and completion. Single-account requests log the matched source name, alias category, full-precision score, entry UID, confidence, and evidence labels.

## Matching policy

- Compose and compare complete names in source order; comparison is case-insensitive.
- Preserve punctuation, accents, hyphens, middle names, and token order.
- Retain every Jaro-Winkler candidate at or above `0.90`.
- Consolidate primary and alias matches by their containing top-level SDN UID.
- Return every qualifying UID in deterministic order.
- Treat full-date agreement as `DOB_FULL` and year, range, month/year, or approximate agreement as `DOB_YEAR`.
- Keep name candidates when DOB disagrees; DOB changes confidence rather than eligibility.
- Screen all SDN record types because the account schema has no type classification.
- Preserve and validate account country and employer without using them as identity evidence.

| Name evidence | DOB evidence | Confidence |
| --- | --- | --- |
| Fuzzy | None | LOW |
| Fuzzy | Year | MED |
| Fuzzy | Full date | HIGH |
| Exact after case folding | Year or full date | HIGH |

An exact name without DOB evidence remains LOW. These labels rank review candidates; they do not establish identity.

### Weak-alias example

The synthetic entry `10001` assigns `Secret Alias` to the weak category. With the default `-weak-aliases include`, account `demo-102` returns a LOW exact-name match. Restart with `-weak-aliases exclude`, and that account returns an empty match list.

Weak aliases can create many false positives. See [OFAC's weak-alias guidance](https://ofac.treasury.gov/faqs/124) and [match-assessment guidance](https://ofac.treasury.gov/faqs/5).

## Testing

Run every test with individual names visible:

```sh
go test -count=1 -v ./...
```

Run concurrency and static checks:

```sh
go test -race ./...
go vet ./...
```

Tests cover CSV/XML validation, all supported DOB precisions, the complete confidence matrix, threshold boundaries, primary/strong/weak aliases, every SDN record type, nested versus entry UIDs, multiple candidates, cache isolation and eviction, deterministic ordering, cancellation, HTTP errors, atomic downloads, and the documented synthetic examples.

## Project layout

- `cmd/screening/` — flags, source loading, and HTTP server startup
- `internal/screening/` — parsing, matching, confidence, cache, HTTP handlers, and tests
- `data/accounts.csv` — synthetic demonstration accounts
- `testdata/` — synthetic SDN and weak-alias fixtures

The normal live source is OFAC's stable [full SDN XML endpoint](https://sanctionslistservice.ofac.treas.gov/api/download/SDN.XML). The latest-changes feed is a delta and cannot replace a complete snapshot.

## License

Licensed under the [MIT License](LICENSE). The project depends on [`github.com/hbollon/go-edlib`](https://github.com/hbollon/go-edlib) v1.7.0, which is also MIT-licensed.
