package screening

import (
	"context"
	"fmt"
	"io"
	"log"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	edlib "github.com/hbollon/go-edlib"
)

const similarityThreshold float32 = 0.90

func normalizeName(name string) string { return strings.ToLower(name) }

type searchableName struct {
	UID        string
	Text       string
	Category   string
	normalized string
}

type Service struct {
	accounts    []Account
	byAccountID map[string]Account
	byUID       map[string]Record
	names       []searchableName
	cache       *candidateCache
	logger      *log.Logger
}

// NewService builds an immutable in-memory screening index for one snapshot.
func NewService(accounts []Account, snapshot Snapshot, cacheSize int, includeWeak bool) (*Service, error) {
	if cacheSize < 0 {
		return nil, fmt.Errorf("cache size cannot be negative")
	}
	service := &Service{
		accounts:    append([]Account(nil), accounts...),
		byAccountID: make(map[string]Account, len(accounts)),
		byUID:       make(map[string]Record, len(snapshot.Records)),
		cache:       newCandidateCache(cacheSize),
		logger:      log.New(io.Discard, "", 0),
	}
	for _, account := range accounts {
		if _, exists := service.byAccountID[account.ID]; exists {
			return nil, fmt.Errorf("duplicate account id %q", account.ID)
		}
		service.byAccountID[account.ID] = account
	}
	for _, record := range snapshot.Records {
		if _, exists := service.byUID[record.UID]; exists {
			return nil, fmt.Errorf("duplicate SDN uid %q", record.UID)
		}
		service.byUID[record.UID] = record
		for _, name := range record.Names {
			if name.Category == "weak" && !includeWeak {
				continue
			}
			service.names = append(service.names, searchableName{UID: record.UID, Text: name.Text, Category: name.Category, normalized: normalizeName(name.Text)})
		}
	}
	return service, nil
}

// SetLogger enables request and bulk-progress logs. Call it before serving requests.
func (s *Service) SetLogger(logger *log.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

func (s *Service) candidatesForName(name string) []candidate {
	key := normalizeName(name)
	if cached, ok := s.cache.Get(key); ok {
		return cached
	}
	best := make(map[string]candidate)
	for _, listed := range s.names {
		score := edlib.JaroWinklerSimilarity(key, listed.normalized)
		if score < similarityThreshold {
			continue
		}
		current, exists := best[listed.UID]
		exact := key == listed.normalized
		if !exists || score > current.Score || (exact && !current.Exact) {
			best[listed.UID] = candidate{UID: listed.UID, MatchedName: listed.Text, NameCategory: listed.Category, Score: score, Exact: exact}
		}
	}
	result := make([]candidate, 0, len(best))
	for _, found := range best {
		result = append(result, found)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UID < result[j].UID })
	s.cache.Put(key, result)
	return result
}

func (s *Service) screenWithCandidates(account Account, candidates []candidate) Result {
	result := Result{AccountID: account.ID, Matches: make([]Match, 0, len(candidates))}
	for _, found := range candidates {
		dob := DOBEvidence(account.DOB, s.byUID[found.UID].DOBs)
		nameType := "NAME_FUZZY"
		confidence := "LOW"
		if found.Exact {
			nameType = "NAME_EXACT"
		}
		if dob == "DOB_FULL" || (found.Exact && dob == "DOB_YEAR") {
			confidence = "HIGH"
		} else if dob == "DOB_YEAR" {
			confidence = "MED"
		}
		matchTypes := []string{nameType}
		if dob != "" {
			matchTypes = append(matchTypes, dob)
		}
		result.Matches = append(result.Matches, Match{UID: found.UID, Confidence: confidence, MatchTypes: matchTypes})
	}
	return result
}

// Screen evaluates one loaded account against the service snapshot.
func (s *Service) Screen(account Account) Result {
	return s.screenWithCandidates(account, s.candidatesForName(account.Name()))
}

func (s *Service) screenAll(ctx context.Context) ([]Result, error) {
	// Discover each distinct name once, in parallel, then apply DOB evidence per account.
	started := time.Now()
	unique := make(map[string]string)
	for _, account := range s.accounts {
		unique[normalizeName(account.Name())] = account.Name()
	}
	type job struct{ key, name string }
	jobs := make(chan job)
	found := make(map[string][]candidate, len(unique))
	var mutex sync.Mutex
	var workers sync.WaitGroup
	count := runtime.GOMAXPROCS(0)
	total := int64(len(unique))
	step := (total + 9) / 10
	if step == 0 {
		step = 1
	}
	var processed atomic.Int64
	s.logger.Printf("bulk screening started: accounts=%d distinct_names=%d searchable_sdn_names=%d workers=%d", len(s.accounts), total, len(s.names), count)
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-jobs:
					if !ok {
						return
					}
					candidates := s.candidatesForName(item.name)
					mutex.Lock()
					found[item.key] = candidates
					mutex.Unlock()
					done := processed.Add(1)
					if done%step == 0 || done == total {
						s.logger.Printf("bulk screening progress: distinct_names=%d/%d percent=%.0f elapsed=%s", done, total, float64(done)*100/float64(total), time.Since(started).Round(time.Second))
					}
				}
			}
		}()
	}

	feedCanceled := false
feed:
	for key, name := range unique {
		select {
		case jobs <- job{key: key, name: name}:
		case <-ctx.Done():
			feedCanceled = true
			break feed
		}
	}
	close(jobs)
	workers.Wait()
	if feedCanceled || ctx.Err() != nil {
		return nil, ctx.Err()
	}
	results := make([]Result, len(s.accounts))
	matchCount := 0
	for i, account := range s.accounts {
		results[i] = s.screenWithCandidates(account, found[normalizeName(account.Name())])
		matchCount += len(results[i].Matches)
	}
	s.logger.Printf("bulk screening completed: accounts=%d matches=%d elapsed=%s", len(results), matchCount, time.Since(started).Round(time.Millisecond))
	return results, nil
}
