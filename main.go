package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"internal/database"
)

var profanityRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bkerfuffle\b`),
	regexp.MustCompile(`(?i)\bsharbert\b`),
	regexp.MustCompile(`(?i)\bfornax\b`),
}

type apiConfig struct {
	fileserverHits atomic.Int32
	dbQueries      *database.Queries
	platform       string
}

type errorBody struct {
	Error string `json:"error"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

func marshalUser(u database.User) User {
	return User{
		ID:        u.ID,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		Email:     u.Email,
	}
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

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	type CreateUser struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	createUser := CreateUser{}
	err := decoder.Decode(&createUser)
	if err != nil {
		msg := fmt.Sprintf("Error decoding user email: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}

	created, err := cfg.dbQueries.CreateUser(r.Context(), createUser.Email)
	if err != nil {
		msg := fmt.Sprintf("Error creating user: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	respondWithJSON(w, http.StatusCreated, marshalUser(created))
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
		if cfg.platform != "dev" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		err := cfg.dbQueries.DeleteUsers(r.Context())
		if err != nil {
			msg := fmt.Sprintf("Error deleting users: %s", err)
			log.Print(msg)
			respondWithError(w, http.StatusInternalServerError, msg)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		cfg.fileserverHits.Store(0)
		msg := []byte("Reset hits: " + strconv.Itoa(int(cfg.fileserverHits.Load())))
		w.Write(msg)
	})
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL must be set")
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database: %s", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("Error connecting to database: %s", err)
	}

	cfg := apiConfig{}
	cfg.dbQueries = database.New(db)
	cfg.fileserverHits.Store(0)
	cfg.platform = os.Getenv("PLATFORM")

	mux := http.NewServeMux()
	mux.Handle("/app", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /api/healthz", healthz)
	mux.HandleFunc("POST /api/validate_chirp", validateChirp)
	mux.HandleFunc("POST /api/users", cfg.createUser)

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
