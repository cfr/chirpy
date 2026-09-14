package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

var profanityRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bkerfuffle\b`),
	regexp.MustCompile(`(?i)\bsharbert\b`),
	regexp.MustCompile(`(?i)\bfornax\b`),
}

type apiConfig struct {
	fileserverHits atomic.Int32
}

type errorBody struct {
	Error string `json:"error"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJSON(w, code, errorBody{Error: msg})
}

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		msg := fmt.Sprintf("Error marshalling JSON: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func clean(chirp string) string {
	cleaned := chirp
	for _, re := range profanityRegexes {
		cleaned = re.ReplaceAllString(cleaned, "****")
	}
	return cleaned
}

func validateChirp(w http.ResponseWriter, r *http.Request) {
	type Chirp struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	chirp := Chirp{}
	err := decoder.Decode(&chirp)
	if err != nil {
		msg := fmt.Sprintf("Error decoding chirp: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	if utf8.RuneCountInString(chirp.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}
	cleaned := clean(chirp.Body)
	valid := map[string]any{
		"cleaned_body": cleaned,
	}
	respondWithJSON(w, http.StatusOK, valid)
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) metrics() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		template := `<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`
		msg := fmt.Sprintf(template, cfg.fileserverHits.Load())
		w.Write([]byte(msg))
	})
}

func (cfg *apiConfig) reset() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		cfg.fileserverHits.Store(0)
		msg := []byte("Reset hits: " + strconv.Itoa(int(cfg.fileserverHits.Load())))
		w.Write(msg)
	})
}

func main() {
	cfg := apiConfig{}
	cfg.fileserverHits.Store(0)
	mux := http.NewServeMux()
	mux.Handle("/app", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))
	mux.HandleFunc("GET /api/healthz", healthz)
	mux.HandleFunc("POST /api/validate_chirp", validateChirp)
	mux.Handle("GET /admin/metrics", cfg.metrics())
	mux.Handle("POST /admin/reset", cfg.reset())
	s := &http.Server{
		Addr:           ":8080",
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Print("Starting server on http://localhost:8080")
	log.Fatal(s.ListenAndServe())
}
