# Deploy: ps5-auth

TLS for `ca.account.sony.com` (and the aliases) is terminated in front (Traefik) and forwarded to `PS5_ADDR`.

## Run

```sh
cp example.env .env
set -a; source .env; set +a
go run ./cmd/ps5-auth
```

```sh
GOWORK=off go test ./... && GOWORK=off go vet ./...
docker build -t ps5-auth .
```

## Configuration

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `PS5_ADDR` | no | `:8080` | listen address |
| `PS5_TOKEN_KEY` | yes | | HMAC key, at least 32 bytes |
| `PS5_ACCOUNT_CORE_URL` / `PS5_ACCOUNT_CORE_TOKEN` | no | | account core; both or neither; unset means in-memory |
| `PS5_DEV_ACCOUNT` | no | | dev only, `email:password:accountId` |
| `PS5_LOG_LEVEL` | no | `info` | debug, info, warn, error |

## Vendoring

`replace github.com/openpak/ps5-common => ../ps5-common` and a committed `vendor/`; after changing ps5-common run
`GOWORK=off go mod vendor` here.

## Release

Tag `v*.*.*` publishes `ghcr.io/openpak/ps5-auth`.
