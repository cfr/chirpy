package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

	"github.com/cfr/chirpy/internal/auth"
	"github.com/cfr/chirpy/internal/database"
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
	secret         string
}

type errorBody struct {
	Error string `json:"error"`
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
}

func marshalUser(u database.User) User {
	return User{
		ID:        u.ID,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		Email:     u.Email,
	}
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func marshalChirp(c database.Chirp) Chirp {
	return Chirp{
		ID:        c.ID,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
		Body:      c.Body,
		UserID:    c.UserID,
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

func (cfg *apiConfig) createChirp(w http.ResponseWriter, r *http.Request) {
	type CreateChirp struct {
		Body string `json:"body"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		msg := fmt.Sprintf("Error getting token: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}
	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		msg := fmt.Sprintf("Error getting token: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}

	decoder := json.NewDecoder(r.Body)
	createChirp := CreateChirp{}
	err = decoder.Decode(&createChirp)
	if err != nil {
		msg := fmt.Sprintf("Error decoding chirp: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	if utf8.RuneCountInString(createChirp.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}
	cleaned := clean(createChirp.Body)
	params := database.CreateChirpParams{Body: cleaned, UserID: userID}
	created, err := cfg.dbQueries.CreateChirp(r.Context(), params)
	if err != nil {
		msg := fmt.Sprintf("Error creating chirp: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	respondWithJSON(w, http.StatusCreated, marshalChirp(created))
}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, r *http.Request) {
	rawChirps, err := cfg.dbQueries.GetChirps(r.Context())
	if err != nil {
		msg := fmt.Sprintf("Error getting chirps: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	chirps := make([]Chirp, 0, len(rawChirps))
	for _, c := range rawChirps {
		chirps = append(chirps, marshalChirp(c))
	}
	respondWithJSON(w, http.StatusOK, chirps)
}

func (cfg *apiConfig) getChirp(w http.ResponseWriter, r *http.Request) {
	chirpID := r.PathValue("chirpID")
	chirpUUID, err := uuid.Parse(chirpID)
	if err != nil {
		msg := fmt.Sprintf("Malformed chirp id: %s", chirpID)
		log.Print(msg)
		respondWithError(w, http.StatusNotFound, msg)
		return
	}
	rawChirp, err := cfg.dbQueries.GetChirp(r.Context(), chirpUUID)
	if err != nil {
		msg := fmt.Sprintf("Error getting chirp %s: %s", chirpID, err)
		log.Print(msg)
		respondWithError(w, http.StatusNotFound, msg)
		return
	}
	respondWithJSON(w, http.StatusOK, marshalChirp(rawChirp))
}

func (cfg *apiConfig) createTokens(ctx context.Context, user database.User) (User, error) {
	token, err := auth.MakeJWT(user.ID, cfg.secret, time.Hour)
	if err != nil {
		msg := fmt.Sprintf("Failed to create token: %s", err)
		return User{}, errors.New(msg)
	}
	userWToken := marshalUser(user)
	userWToken.Token = token
	userWToken.RefreshToken = auth.MakeRefreshToken()

	params := database.SaveRefreshTokenParams{
		Token:     userWToken.RefreshToken,
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(time.Duration(24*60) * time.Hour),
	}
	_, err = cfg.dbQueries.SaveRefreshToken(ctx, params)
	if err != nil {
		msg := fmt.Sprintf("Failed to save refresh token: %s", err)
		log.Print(msg)
		return User{}, errors.New(msg)
	}
	return userWToken, nil
}

func (cfg *apiConfig) login(w http.ResponseWriter, r *http.Request) {
	type Login struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	login := Login{}
	err := decoder.Decode(&login)
	if err != nil {
		msg := fmt.Sprintf("Error decoding user credentials: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}

	user, err := cfg.dbQueries.UserByEmail(r.Context(), login.Email)
	if err != nil {
		msg := fmt.Sprintf("Failed to get user: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}

	match, err := auth.CheckPasswordHash(login.Password, user.HashedPassword)
	if err != nil {
		msg := fmt.Sprintf("Failed to hash password: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}
	if !match {
		msg := fmt.Sprintf("Wrong password fo user %s", user.Email)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}

	userWTokens, err := cfg.createTokens(r.Context(), user)

	if err != nil {
		msg := fmt.Sprintf("Failed to create token: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}

	respondWithJSON(w, http.StatusOK, userWTokens)
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	type CreateUser struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	createUser := CreateUser{}
	err := decoder.Decode(&createUser)
	if err != nil {
		msg := fmt.Sprintf("Error decoding user credentials: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}

	hash, err := auth.HashPassword(createUser.Password)
	if err != nil {
		msg := fmt.Sprintf("Failed to hash password: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}

	params := database.CreateUserParams{Email: createUser.Email, HashedPassword: hash}
	created, err := cfg.dbQueries.CreateUser(r.Context(), params)
	if err != nil {
		msg := fmt.Sprintf("Error creating user: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusInternalServerError, msg)
		return
	}

	// TODO: separate create user and login
	userWTokens, err := cfg.createTokens(r.Context(), created)

	if err != nil {
		msg := fmt.Sprintf("Failed to create token: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}
	respondWithJSON(w, http.StatusCreated, userWTokens)
}

func (cfg *apiConfig) refreshToken(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		msg := fmt.Sprintf("Failed to get refresh token: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}
	refresh, err := cfg.dbQueries.GetRefreshToken(r.Context(), refreshToken)
	if err != nil {
		msg := fmt.Sprintf("Failed to get refresh token: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}
	if time.Now().After(refresh.ExpiresAt) {
		msg := fmt.Sprintf("Expired refresh token")
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}
	if refresh.RevokedAt.Valid {
		msg := fmt.Sprintf("Revoked refresh token")
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}
	token, err := auth.MakeJWT(refresh.UserID, cfg.secret, time.Hour)
	if err != nil {
		msg := fmt.Sprintf("Failed to create token: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}
	type Token struct {
		Token string `json:"token"`
	}
	body := Token{Token: token}
	respondWithJSON(w, http.StatusOK, body)
}

func (cfg *apiConfig) revokeToken(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		msg := fmt.Sprintf("Failed to get refresh token: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}
	err = cfg.dbQueries.RevokeRefreshToken(r.Context(), refreshToken)
	if err != nil {
		msg := fmt.Sprintf("Failed to revoke token: %s", err)
		log.Print(msg)
		respondWithError(w, http.StatusUnauthorized, msg)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

	secret := os.Getenv("SECRET")
	if secret == "" {
		log.Fatal("SECRET must be set")
	}

	cfg := apiConfig{}
	cfg.dbQueries = database.New(db)
	cfg.fileserverHits.Store(0)
	cfg.platform = os.Getenv("PLATFORM")
	cfg.secret = secret

	mux := http.NewServeMux()
	mux.Handle("/app", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /api/healthz", healthz)
	mux.HandleFunc("POST /api/chirps", cfg.createChirp)
	mux.HandleFunc("GET /api/chirps", cfg.getChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.getChirp)
	mux.HandleFunc("POST /api/users", cfg.createUser)
	mux.HandleFunc("POST /api/login", cfg.login)
	mux.HandleFunc("POST /api/refresh", cfg.refreshToken)
	mux.HandleFunc("POST /api/revoke", cfg.revokeToken)

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
