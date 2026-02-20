# Chirpy API Server

Go HTTP API for users, authentication, chirps, and admin/webhook operations.

## Overview

- Runtime: Go (`net/http` with `http.ServeMux`)
- Database: PostgreSQL
- Auth:
  - Access token: JWT (`Authorization: Bearer <access_token>`)
  - Refresh token: opaque token (`Authorization: Bearer <refresh_token>`)
  - Webhook API key: `Authorization: ApiKey <POLKA_KEY>`

## Configuration

The server reads environment variables at startup:

- `DB_URL`: PostgreSQL connection URL
- `SECRET`: JWT signing secret
- `PLATFORM`: set to `dev` to enable `POST /admin/reset`
- `POLKA_KEY`: API key for `/api/polka/webhooks`

Example:

```bash
export DB_URL="postgres://user:pass@localhost:5432/chirpy?sslmode=disable"
export SECRET="replace-with-strong-secret"
export PLATFORM="dev"
export POLKA_KEY="replace-with-webhook-key"
```

## Run

```bash
go run .
```

Default bind address: `http://localhost:8080`

## API Conventions

- Base URL: `http://localhost:8080`
- JSON endpoints use `Content-Type: application/json`
- UUIDs are used for `id`, `user_id`, and `chirpID`
- Access-token protected endpoints require `Authorization: Bearer <access_token>`
- Refresh/revoke endpoints require `Authorization: Bearer <refresh_token>`

## Endpoints Summary

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/api/healthz` | No | Health check |
| POST | `/api/users` | No | Register user |
| PUT | `/api/users` | Bearer access token | Update authenticated user email/password |
| POST | `/api/login` | No | Login and receive access + refresh tokens |
| POST | `/api/refresh` | Bearer refresh token | Exchange refresh token for a new access token |
| POST | `/api/revoke` | Bearer refresh token | Revoke refresh token |
| POST | `/api/chirps` | Bearer access token | Create chirp |
| GET | `/api/chirps` | No | List chirps (supports filtering/sorting) |
| GET | `/api/chirps/{chirpID}` | No | Get chirp by ID |
| DELETE | `/api/chirps/{chirpID}` | Bearer access token | Delete chirp owned by authenticated user |
| GET | `/admin/metrics` | No | HTML metrics page |
| POST | `/admin/reset` | No (dev only) | Delete all users and reset visit counter |
| POST | `/api/polka/webhooks` | `ApiKey` header | Handle user upgrade webhook |

## Endpoint Details

### GET `/api/healthz`

Response `200`:

```text
OK
```

### POST `/api/users`

Create a user.

Request body:

```json
{
  "email": "alice@example.com",
  "password": "strong-password"
}
```

Response `201`:

```json
{
  "id": "uuid",
  "created_at": "timestamp",
  "updated_at": "timestamp",
  "email": "alice@example.com",
  "is_chirpy_red": false
}
```

### POST `/api/login`

Authenticate and return both tokens.

Request body:

```json
{
  "email": "alice@example.com",
  "password": "strong-password"
}
```

Response `200`:

```json
{
  "id": "uuid",
  "created_at": "timestamp",
  "updated_at": "timestamp",
  "email": "alice@example.com",
  "token": "jwt-access-token",
  "refresh_token": "opaque-refresh-token",
  "is_chirpy_red": false
}
```

### POST `/api/refresh`

Send refresh token in bearer header.

Header:

```text
Authorization: Bearer <refresh_token>
```

Response `200`:

```json
{
  "token": "new-jwt-access-token"
}
```

### POST `/api/revoke`

Revoke refresh token.

Header:

```text
Authorization: Bearer <refresh_token>
```

Response: `204 No Content`

### PUT `/api/users`

Update authenticated user email/password.

Header:

```text
Authorization: Bearer <access_token>
```

Request body:

```json
{
  "email": "alice.new@example.com",
  "password": "new-password"
}
```

Response `200`:

```json
{
  "id": "uuid",
  "email": "alice.new@example.com",
  "created_at": "timestamp",
  "updated_at": "timestamp",
  "is_chirpy_red": false
}
```

### POST `/api/chirps`

Create a chirp for the authenticated user.

Header:

```text
Authorization: Bearer <access_token>
```

Request body:

```json
{
  "body": "hello chirpy"
}
```

Notes:

- Max body length: 140 chars
- Words `kerfuffle`, `sharbert`, and `fornax` are replaced with `****`

Response `201`:

```json
{
  "id": "uuid",
  "created_at": "timestamp",
  "updated_at": "timestamp",
  "body": "hello chirpy",
  "user_id": "uuid"
}
```

### GET `/api/chirps`

List chirps.

Query params:

- `author_id=<uuid>`: filter by author
- `sort=desc`: newest first (default is ascending)

Response `200`:

```json
[
  {
    "id": "uuid",
    "created_at": "timestamp",
    "updated_at": "timestamp",
    "body": "hello chirpy",
    "user_id": "uuid"
  }
]
```

### GET `/api/chirps/{chirpID}`

Response `200`:

```json
{
  "id": "uuid",
  "created_at": "timestamp",
  "updated_at": "timestamp",
  "body": "hello chirpy",
  "user_id": "uuid"
}
```

Returns `404` if chirp is not found or `chirpID` is invalid.

### DELETE `/api/chirps/{chirpID}`

Delete a chirp owned by the authenticated user.

Header:

```text
Authorization: Bearer <access_token>
```

Response: `204 No Content`

Returns:

- `403` if chirp exists but is owned by another user
- `404` if chirp does not exist or ID is invalid

### GET `/admin/metrics`

Returns an HTML page with file-server hit count.

### POST `/admin/reset`

Development-only reset endpoint.

- Works only if `PLATFORM=dev`
- Deletes users (cascade deletes chirps and refresh tokens)
- Resets file-server hit counter

Responses:

- `200 OK` on success
- `403` when not in dev mode

### POST `/api/polka/webhooks`

Webhook endpoint to upgrade a user.

Header:

```text
Authorization: ApiKey <POLKA_KEY>
```

Request body:

```json
{
  "event": "user.upgraded",
  "data": {
    "user_id": "uuid"
  }
}
```

Behavior:

- Returns `204` for non-`user.upgraded` events
- Returns `204` after successful upgrade
- Returns `401` for missing/invalid API key

## Quick `curl` Flow

Register:

```bash
curl -sS -X POST http://localhost:8080/api/users \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"pass123"}'
```

Login:

```bash
curl -sS -X POST http://localhost:8080/api/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"pass123"}'
```

Create chirp (replace `<ACCESS_TOKEN>`):

```bash
curl -sS -X POST http://localhost:8080/api/chirps \
  -H 'Authorization: Bearer <ACCESS_TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{"body":"Hello world"}'
```

Refresh access token (replace `<REFRESH_TOKEN>`):

```bash
curl -sS -X POST http://localhost:8080/api/refresh \
  -H 'Authorization: Bearer <REFRESH_TOKEN>'
```

## Notes for Future Improvement

- Add a consistent JSON error format (`{"error":"..."}`) for all non-2xx responses
- Return `400 Bad Request` for malformed JSON instead of `500`
- Set `Content-Type` header before `WriteHeader` in JSON handlers
- Add OpenAPI spec (`openapi.yaml`) and generate docs from source
