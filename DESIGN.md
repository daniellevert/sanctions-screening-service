# Design

## Goals and boundaries

The service loads an account CSV and one complete SDN snapshot at startup, then exposes bulk and single-account screening over HTTP.

```text
accounts.csv ─┐
              ├─> validated in-memory service ─> HTTP results
sdn.xml ──────┘             │
                            └─> bounded candidate cache
```

The repository focuses on deterministic local processing. It has no database, user interface, automatic background refresh, or compliance case-management workflow.

## Technology choices

| Choice | Reason | Tradeoff |
| --- | --- | --- |
| Go 1.22 | Static types, a strong HTTP/data-format standard library, bounded concurrency, race detection, and single-binary builds fit a small CPU-heavy service. | Explicit error handling and streaming decode code are more verbose than some dynamic-language alternatives. |
| Go standard library | `net/http`, `encoding/csv`, `encoding/xml`, `encoding/json`, and `time` cover the required protocols and formats without a framework. | Routing and validation remain explicit. |
| [`go-edlib`](https://github.com/hbollon/go-edlib) v1.7.0 | A focused, pure-Go, Unicode-compatible, MIT-licensed Jaro-Winkler implementation avoids maintaining a subtle scoring algorithm locally. | It is the sole external runtime dependency. Normalization, thresholds, UID consolidation, and confidence remain application policy. |
| In-memory snapshot and cache | Validating once makes requests deterministic and avoids database setup. A bounded cache reuses repeated name searches. | Refreshing requires a new service instance, and memory use follows source and cache size. |

An asynchronous job API could better suit very large workloads. This implementation keeps a simple synchronous contract while using a bounded worker pool internally.

## Parsing and model boundaries

The account loader preserves source order and values, composes names in first/middle/last order, and rejects malformed rows, duplicate IDs, invalid UTF-8, outer whitespace, and invalid dates.

The SDN loader streams with `encoding/xml.Decoder`. `DecodeElement` maps one `<sdnEntry>` at a time into private wire structs, then the loader constructs the smaller screening model. Streaming limits temporary memory and allows entry-specific validation errors.

The models have separate responsibilities:

- `Account` represents one validated source row.
- `Snapshot` groups one publication date with its parsed SDN records.
- `Record` contains one top-level UID, record type, searchable names, and DOB alternatives.
- `Result` and `Match` define the JSON response.
- Private XML structs isolate external schema details from matching code.

Nested alias, address, DOB, and identification UIDs describe components of an entry. Results always return the containing `sdnEntry/uid`.

## Matching and confidence

Names are composed and compared as complete strings. Case is folded; punctuation, accents, hyphens, token order, and middle names remain significant. Jaro-Winkler scores at or above `0.90` qualify before rounding.

For each UID, the matcher selects the strongest qualifying name and strongest DOB evidence across all alternatives. Full-date agreement produces `DOB_FULL`; year-only, ranged, month/year, and approximate values produce `DOB_YEAR`. A DOB disagreement does not remove a name candidate.

| Name evidence | DOB evidence | Confidence |
| --- | --- | --- |
| Fuzzy | None | LOW |
| Fuzzy | Year | MED |
| Fuzzy | Full date | HIGH |
| Exact after case folding | Year or full date | HIGH |

## Policy assumptions

- One immutable SDN snapshot is authoritative for a service instance.
- Every SDN record type is eligible because accounts have no explicit person/entity classification.
- Primary, strong, and weak aliases are eligible by default; weak aliases can be excluded explicitly.
- Partial and approximate DOBs provide year evidence only; `circa` has no invented tolerance.
- All qualifying UIDs are returned rather than selecting a presumed identity.
- Country and employer are retained but do not affect identity confidence. Their semantics differ from OFAC nationality, address, registration, flag, and related-entity data.
- An employer-name hit would describe a possible relationship with an organization, not prove that the account holder is that entry. Related-entity screening needs a separate result and policy.
- Invalid or unfamiliar source data fails explicitly rather than being repaired or guessed.

These rules produce review candidates, not legal identity determinations.

## Cache, concurrency, and cancellation

The concurrency-safe LRU cache stores immutable candidate lists by normalized account name. It contains no account DOB or final confidence, so repeated names with different DOBs cannot share identity evidence. Empty candidate lists are cached; eviction only causes recomputation.

Bulk screening searches each distinct name through a bounded worker pool, applies DOB evidence per account, and writes results in original CSV order. Request cancellation stops unfinished work. The race detector exercises concurrent cache and request paths.

## Source lifecycle

Normal startup downloads from OFAC's stable full-list endpoint, follows redirects, validates the complete XML, synchronizes the temporary file, and atomically renames it into place. Failures leave any prior file untouched. Passing `-sdn` selects an explicit local snapshot and disables downloading.

The repository excludes downloaded snapshots from version control. Checked-in accounts and XML fixtures are synthetic and deterministic.

## Verification

Unit and integration tests cover:

- strict CSV and XML validation;
- every supported DOB precision and invalid format;
- every confidence-table branch;
- the inclusive similarity boundary;
- primary, strong, and weak aliases;
- individual, entity, vessel, and aircraft entries;
- containing-entry UID behavior and multiple qualifying UIDs;
- cache immutability, eviction, and DOB isolation;
- HTTP success, error, ordering, cancellation, and empty-array behavior; and
- redirect handling and atomic snapshot preservation.

Synthetic dataset tests make expected results reviewable without network access or third-party personal data.
