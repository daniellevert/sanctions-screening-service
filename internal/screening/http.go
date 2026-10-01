package screening

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// Handler returns the service's bulk and single-account HTTP routes.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/screenings", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/screenings" {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}
		var one [1]byte
		if read, _ := r.Body.Read(one[:]); read != 0 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "request body must be empty"})
			return
		}
		results, err := s.screenAll(r.Context())
		if err != nil {
			s.logger.Printf("bulk screening stopped: %v", err)
			return
		}
		writeJSON(w, http.StatusOK, results)
	})
	mux.HandleFunc("/screenings/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/screenings/")
		if id == "" || strings.Contains(id, "/") {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "account not found"})
			return
		}
		account, ok := s.byAccountID[id]
		if !ok {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "account not found"})
			return
		}
		started := time.Now()
		candidates := s.candidatesForName(account.Name())
		result := s.screenWithCandidates(account, candidates)
		for i, match := range result.Matches {
			candidate := candidates[i]
			s.logger.Printf("single screening match: account_id=%q uid=%q matched_name=%q name_category=%q score=%.6f confidence=%s match_types=%s", id, match.UID, candidate.MatchedName, candidate.NameCategory, candidate.Score, match.Confidence, strings.Join(match.MatchTypes, ","))
		}
		s.logger.Printf("single screening completed: account_id=%q matches=%d elapsed=%s", id, len(result.Matches), time.Since(started).Round(time.Millisecond))
		writeJSON(w, http.StatusOK, result)
	})
	return mux
}
