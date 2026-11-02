package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/openpak/ps5-common/accountcore"
	"github.com/openpak/ps5-common/oauth"
)

func srv(t *testing.T) *httptest.Server {
	sg, _ := oauth.NewSigner([]byte(strings.Repeat("k", 32)))
	mem := accountcore.NewMemory()
	mem.AddAccount("a@b.c", "pw", "1001", accountcore.StatusActive)
	s := &Server{Signer: sg, Core: mem, Log: slog.Default()}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestRoundTrip(t *testing.T) {
	ts := srv(t)
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/ssocookie", nil)
	req.SetBasicAuth("a@b.c", "pw")
	resp, _ := hc.Do(req)
	var sso map[string]string
	json.NewDecoder(resp.Body).Decode(&sso)
	if sso["npsso"] == "" {
		t.Fatal("no npsso")
	}

	q := url.Values{"response_type": {"code"}, "client_id": {"x"}, "access_type": {"offline"},
		"redirect_uri": {"com.playstation.PlayStationApp://redirect"}, "scope": {"psn:mobile.v2.core psn:clientapp"}}
	req, _ = http.NewRequest("GET", ts.URL+"/api/authz/v3/oauth/authorize?"+q.Encode(), nil)
	req.AddCookie(&http.Cookie{Name: "npsso", Value: sso["npsso"]})
	resp, _ = hc.Do(req)
	loc, _ := url.Parse(resp.Header.Get("Location"))
	code := loc.Query().Get("code")
	if resp.StatusCode != 302 || code == "" {
		t.Fatalf("authorize: %d %v", resp.StatusCode, loc)
	}

	post := func(f url.Values) map[string]any {
		req, _ := http.NewRequest("POST", ts.URL+"/api/authz/v3/oauth/token", strings.NewReader(f.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth("cid", "secret")
		r, _ := hc.Do(req)
		var m map[string]any
		json.NewDecoder(r.Body).Decode(&m)
		return m
	}
	tok := post(url.Values{"grant_type": {"authorization_code"}, "code": {code}, "token_format": {"jwt"}})
	for _, k := range []string{"access_token", "token_type", "refresh_token", "expires_in", "scope"} {
		if tok[k] == nil {
			t.Fatalf("missing %s: %v", k, tok)
		}
	}
	if tok["scope"] != "psn:mobile.v2.core psn:clientapp" {
		t.Fatal(tok["scope"])
	}
	if m := post(url.Values{"grant_type": {"authorization_code"}, "code": {tok["access_token"].(string)}}); m["error"] != "invalid_grant" {
		t.Fatal("access token accepted as code")
	}
	ref := post(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tok["refresh_token"].(string)}})
	if ref["access_token"] == nil || ref["scope"] != tok["scope"] {
		t.Fatalf("refresh: %v", ref)
	}

	req, _ = http.NewRequest("GET", ts.URL+"/api/authz/v3/oauth/token", nil)
	req.Header.Set("Authorization", "Bearer "+ref["access_token"].(string))
	r, _ := hc.Do(req)
	var info map[string]any
	json.NewDecoder(r.Body).Decode(&info)
	if info["active"] != true || info["account_id"] != "1001" {
		t.Fatal(info)
	}
}

func TestBadNpsso(t *testing.T) {
	ts := srv(t)
	hc := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, _ := hc.Get(ts.URL + "/api/authz/v3/oauth/authorize?response_type=code&client_id=x&redirect_uri=app://r&npsso=bad")
	if r.StatusCode != 302 || !strings.Contains(r.Header.Get("Location"), "error_code=4165") {
		t.Fatal(r.StatusCode, r.Header.Get("Location"))
	}
	r, _ = hc.Get(ts.URL + "/api/authz/v3/oauth/whatever")
	if r.StatusCode != 404 {
		t.Fatal(r.StatusCode)
	}
}
