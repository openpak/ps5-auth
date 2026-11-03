# Changelog: ps5-auth

## v0.1.0 (2026-11-03)

- 2026-11-03: refuses to start without `PS5_ACCOUNT_CORE_URL` / `PS5_ACCOUNT_CORE_TOKEN` unless `PS5_DEV_MEMORY=1` (the in-memory
  account fallback is development only).

- First cut. `GET /api/authz/v3/oauth/authorize` (npsso cookie, 302 with `code`), `POST /api/authz/v3/oauth/token`
  (authorization_code, refresh_token), `GET /api/authz/v3/oauth/token` (token info), `GET /api/v1/ssocookie`
  (OpenPak npsso via Basic account credentials), logged 404 catch-all. Round-trip tests. Not deployed, no tag.
