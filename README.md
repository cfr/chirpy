# chirpy

boot.dev go http servers project

## Setup

```
brew install postgresql@17 && brew services start postgresql@17
createdb chirpy
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go mod download
```

## .env

```
cat > .env <<EOF
DB_URL=postgres://$USER@localhost:5432/chirpy?sslmode=disable
SECRET=<random string>
POLKA_KEY=<random string>
PLATFORM=dev
EOF
```

## Migrations & codegen

```
goose -dir sql/schema postgres "postgres://$USER:@localhost:5432/chirpy" up
sqlc generate
```

## Run

```
go run .
```

