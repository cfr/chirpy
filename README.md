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


# API

## Health

### Get health

`GET /api/healthz`

=>

```
200 "OK"
```

```
curl localhost:8080/api/healthz
```

## Users

### Create user

`POST /api/users`

JSON body:
+ `email`
+ `password`

=>

```
201 { id, created_at, updated_at, email, is_chirpy_red, token, refresh_token }
```

```
curl -X POST localhost:8080/api/users -d '{"email":"p@example.com","password":"1234"}'
```

### Update user

`PUT /api/users`

Header: `Authorization: Bearer $TOKEN`

JSON body:
+ `email`
+ `password`

=>

```
200 { id, created_at, updated_at, email, is_chirpy_red }
```

```
curl -X PUT localhost:8080/api/users -H "Authorization: Bearer $TOKEN" -d '{"email":"p@example.com","password":"4321"}'
```

## Login & tokens

### Login

`POST /api/login`

JSON body:
+ `email`
+ `password`

=>

```
200 { id, created_at, updated_at, email, is_chirpy_red, token, refresh_token }
```

```
curl -X POST localhost:8080/api/login -d '{"email":"p@example.com","password":"1234"}'
```

### Refresh access token

`POST /api/refresh`

Header: `Authorization: Bearer $REFRESH_TOKEN`

=>

```
200 { token }
```

```
curl -X POST localhost:8080/api/refresh -H "Authorization: Bearer $REFRESH_TOKEN"
```

### Revoke refresh token

`POST /api/revoke`

Header: `Authorization: Bearer $REFRESH_TOKEN`

=>

```
204
```

```
curl -X POST localhost:8080/api/revoke -H "Authorization: Bearer $REFRESH_TOKEN"
```

## Chirps

### Create chirp

`POST /api/chirps`

Header: `Authorization: Bearer $TOKEN`

JSON body:
+ `body` (max 140 runes)

=>

```
201 { id, created_at, updated_at, body, user_id }
```

```
curl -X POST localhost:8080/api/chirps -H "Authorization: Bearer $TOKEN" -d '{"body":"hello"}'
```

### List chirps

`GET /api/chirps`

Query:
+ `author_id` (optional, uuid)
+ `sort` (optional, `asc`|`desc`, default `asc`)

=>

```
200 [ { id, created_at, updated_at, body, user_id } ]
```

```
curl localhost:8080/api/chirps
curl "localhost:8080/api/chirps?author_id=<uuid>"
curl "localhost:8080/api/chirps?sort=desc"
curl "localhost:8080/api/chirps?author_id=<uuid>&sort=desc"
```

### Get chirp

`GET /api/chirps/{chirpID}`

=>

```
200 { id, created_at, updated_at, body, user_id }
```

```
curl localhost:8080/api/chirps/$CHIRP_ID
```

### Delete chirp

`DELETE /api/chirps/{chirpID}`

Header: `Authorization: Bearer $TOKEN`

=>

```
204
```

```
curl -X DELETE localhost:8080/api/chirps/$CHIRP_ID -H "Authorization: Bearer $TOKEN"
```

## Webhooks

### Upgrade user

`POST /api/polka/webhooks`

Header: `Authorization: ApiKey $POLKA_KEY`

JSON body:
+ `event`
+ `data.user_id`

=>

```
204
```

```
curl -X POST localhost:8080/api/polka/webhooks -H "Authorization: ApiKey $POLKA_KEY" -d '{"event":"user.upgraded","data":{"user_id":"<uuid>"}}'
```

## Admin

### Get metrics

`GET /admin/metrics`

=>

```
200 HTML
```

```
curl localhost:8080/admin/metrics
```

### Reset

`POST /admin/reset`

Env: `PLATFORM=dev`

=>

```
200 "Reset hits: 0"
```

```
curl -X POST localhost:8080/admin/reset
```

