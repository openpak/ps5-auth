# Protocol: ps5-auth

Replaces `ca.account.sony.com` (and, as unverified aliases, `auth.api.sonyentertainmentnetwork.com`,
`np.ac.playstation.net` is NOT ours: `auth.api.np.ac` and `csla` are served by np-account for PS3/PS4). Sourced from psn-api (`src/authenticate`) and PSNAWP (`authenticator.py`).

## Sourced

| Request | Answer |
|---|---|
| `GET /api/authz/v3/oauth/authorize?response_type=code&client_id&redirect_uri&scope&access_type=offline` with `Cookie: npsso=` | `302`, `Location: <redirect_uri>?code=...&cid=...` (`state` echoed). Bad npsso: `302` to `<redirect_uri>?error=login_required&error_code=4165` |
| `POST /api/authz/v3/oauth/token` form `grant_type=authorization_code`, `code`, `redirect_uri`, `token_format` | `200` `{access_token, token_type:"bearer", refresh_token, expires_in:3600, refresh_token_expires_in, scope}` |
| same, `grant_type=refresh_token`, `refresh_token`, `scope` | same shape, new tokens |

Basic client credentials are accepted and never checked. Errors on the token endpoint use the OAuth2 shape
`{error, error_description}` (`invalid_grant`, `unsupported_grant_type`, `invalid_request`).

## OpenPak choices (not Sony behaviour)

- Tokens are ps5-common `oauth` tokens (HMAC, not JWT). `token_format=jwt` is ignored; `id_token` is not issued.
- Access TTL 1 h, refresh 60 days, code 5 min (stateless: a code is not single-use).
- The npsso is an OpenPak token. `GET /api/v1/ssocookie` with HTTP Basic (OpenPak email and password, checked by the
  account core `VerifyPassword`) returns `{"npsso": "..."}`; valid 30 days. Sony's real endpoint uses a web session.
- `redirect_uri` is echoed, not validated against a list.
- `GET /api/authz/v3/oauth/token` with `Authorization: Bearer` (or `?access_token=`) returns
  `{active, token_type, scope, account_id, online_id, expires_in}`; `{"active": false}` for anything invalid. This
  path is a guess at "token info"; the research doc lists no introspection endpoint.
- `online_id` is the `PublicCode` of the account's active link in namespace `ps5`, empty if none.
- Without `PS5_ACCOUNT_CORE_URL` an in-memory core is used (`PS5_DEV_ACCOUNT=email:password:accountId[:onlineId]`; the online id becomes the active `ps5` link public code).

## Unknown

The console's own device sign-in (different client id, device grant, NP ticket issuance) is not documented in any
read source. Every other path, under `/api/authz/` or elsewhere, is logged (method, host, path, query, user agent)
and answered `404` with the PSN error body, so a console capture shows what it asks for.
