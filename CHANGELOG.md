# Changelog: ps5-auth

## Unreleased

- First cut. `GET /api/authz/v3/oauth/authorize` (npsso cookie, 302 with `code`), `POST /api/authz/v3/oauth/token`
  (authorization_code, refresh_token), `GET /api/authz/v3/oauth/token` (token info), `GET /api/v1/ssocookie`
  (OpenPak npsso via Basic account credentials), logged 404 catch-all. Round-trip tests. Not deployed, no tag.
