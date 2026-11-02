// Package auth is the OAuth2 surface of ca.account.sony.com as the PlayStation app uses it.
package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openpak/ps5-common/accountcore"
	"github.com/openpak/ps5-common/httpx"
	"github.com/openpak/ps5-common/oauth"
)

const (
	AccessTTL  = time.Hour
	RefreshTTL = 60 * 24 * time.Hour // PSNAWP documents about 2 months
	CodeTTL    = 5 * time.Minute
	NpssoTTL   = 30 * 24 * time.Hour

	scopeCode    = "openpak:code"
	scopeRefresh = "openpak:refresh"
	scopeNpsso   = "openpak:npsso"
)

// Server holds the signer and the account core.
type Server struct {
	Signer *oauth.Signer
	Core   accountcore.Core
	Log    *slog.Logger
	Now    func() time.Time
}

// Handler returns the routes.
func (s *Server) Handler() http.Handler {
	if s.Now == nil {
		s.Now = time.Now
	}
	m := httpx.Mux()
	m.HandleFunc("GET /api/authz/v3/oauth/authorize", s.authorize)
	m.HandleFunc("POST /api/authz/v3/oauth/token", s.token)
	m.HandleFunc("GET /api/authz/v3/oauth/token", s.info)
	m.HandleFunc("GET /api/v1/ssocookie", s.ssocookie)
	m.HandleFunc("/api/authz/", s.unknown)
	m.HandleFunc("/", s.unknown)
	return m
}

func (s *Server) unknown(w http.ResponseWriter, r *http.Request) {
	s.Log.Warn("unknown path", "method", r.Method, "host", r.Host, "path", r.URL.Path, "query", r.URL.RawQuery, "ua", r.UserAgent())
	httpx.WriteError(w, http.StatusNotFound, 4000, "Not found")
}

func (s *Server) issue(c oauth.Claims, scopes []string, ttl time.Duration) (string, error) {
	c.Scopes = scopes
	c.Expires = s.Now().Add(ttl).Unix()
	return s.Signer.Sign(c)
}

func has(scopes []string, x string) bool {
	return oauth.Claims{Scopes: scopes}.HasScope(x)
}

// onlineID looks up the account's PS5 link; empty when there is none.
func (s *Server) onlineID(ctx context.Context, accountID string) string {
	l, err := s.Core.GetActiveLinkByAccount(ctx, accountcore.Namespace, accountID)
	if err != nil || l == nil {
		return ""
	}
	return l.PublicCode
}

// ssocookie stands in for the signed-in web page that hands out an npsso: the caller
// proves an OpenPak account with HTTP Basic (email:password) and gets {"npsso": ...}.
func (s *Server) ssocookie(w http.ResponseWriter, r *http.Request) {
	email, pw, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="OpenPak"`)
		httpx.WriteError(w, http.StatusUnauthorized, 4165, "Authentication required")
		return
	}
	id, status, err := s.Core.VerifyPassword(r.Context(), email, pw)
	if err != nil || id == "" || status != accountcore.StatusActive {
		httpx.WriteError(w, http.StatusUnauthorized, 4165, "Invalid credentials")
		return
	}
	n, err := s.issue(oauth.Claims{AccountID: id}, []string{scopeNpsso}, NpssoTTL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, 1, "token error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"npsso": n})
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect := q.Get("redirect_uri")
	if redirect == "" || q.Get("client_id") == "" || q.Get("response_type") != "code" {
		httpx.WriteError(w, http.StatusBadRequest, 4001, "invalid_request")
		return
	}
	npsso := q.Get("npsso")
	if c, err := r.Cookie("npsso"); err == nil {
		npsso = c.Value
	}
	scope := strings.Fields(q.Get("scope"))
	back := func(v url.Values) {
		sep := "?"
		if strings.Contains(redirect, "?") {
			sep = "&"
		}
		if st := q.Get("state"); st != "" {
			v.Set("state", st)
		}
		w.Header().Set("Location", redirect+sep+v.Encode())
		w.WriteHeader(http.StatusFound)
	}
	cl, err := s.Signer.Verify(npsso)
	if err != nil || !cl.HasScope(scopeNpsso) {
		back(url.Values{"error": {"login_required"}, "error_code": {"4165"}, "error_description": {"Bad or expired npsso"}})
		return
	}
	code, err := s.issue(oauth.Claims{AccountID: cl.AccountID, OnlineID: s.onlineID(r.Context(), cl.AccountID)}, append([]string{scopeCode}, scope...), CodeTTL)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, 1, "token error")
		return
	}
	back(url.Values{"code": {code}, "cid": {q.Get("cid")}})
}

type tokenResp struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_token_expires_in"`
	Scope            string `json:"scope"`
}

func oauthErr(w http.ResponseWriter, status int, e, desc string) {
	httpx.WriteJSON(w, status, map[string]string{"error": e, "error_description": desc})
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthErr(w, 400, "invalid_request", "bad form")
		return
	}
	// Basic client credentials are accepted and not checked.
	var cl oauth.Claims
	var err error
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		cl, err = s.Signer.Verify(r.PostForm.Get("code"))
		if err != nil || !cl.HasScope(scopeCode) {
			oauthErr(w, 400, "invalid_grant", "bad or expired code")
			return
		}
	case "refresh_token":
		cl, err = s.Signer.Verify(r.PostForm.Get("refresh_token"))
		if err != nil || !cl.HasScope(scopeRefresh) {
			oauthErr(w, 400, "invalid_grant", "bad or expired refresh token")
			return
		}
	default:
		oauthErr(w, 400, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
		return
	}
	var scopes []string
	for _, x := range cl.Scopes {
		if x != scopeCode && x != scopeRefresh {
			scopes = append(scopes, x)
		}
	}
	base := oauth.Claims{AccountID: cl.AccountID, OnlineID: cl.OnlineID}
	at, err1 := s.issue(base, scopes, AccessTTL)
	rt, err2 := s.issue(base, append([]string{scopeRefresh}, scopes...), RefreshTTL)
	if err1 != nil || err2 != nil {
		oauthErr(w, 500, "server_error", "token error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, 200, tokenResp{at, "bearer", rt, int(AccessTTL.Seconds()), int(RefreshTTL.Seconds()), strings.Join(scopes, " ")})
}

// info answers GET /oauth/token for the bearer access token (header or access_token query).
func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	tok, ok := oauth.BearerToken(r)
	if !ok {
		tok = r.URL.Query().Get("access_token")
	}
	cl, err := s.Signer.Verify(tok)
	if err != nil || has(cl.Scopes, scopeCode) || has(cl.Scopes, scopeRefresh) || has(cl.Scopes, scopeNpsso) {
		httpx.WriteJSON(w, 200, map[string]any{"active": false})
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{
		"active": true, "token_type": "bearer", "scope": strings.Join(cl.Scopes, " "),
		"account_id": cl.AccountID, "online_id": cl.OnlineID,
		"expires_in": cl.Expires - s.Now().Unix(),
	})
}
