module github.com/cfr/chirpy

go 1.27.0

require (
	github.com/google/uuid v1.6.0
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.12.3
	internal/database v0.0.0-00010101000000-000000000000
)

replace internal/database => ./internal/database
