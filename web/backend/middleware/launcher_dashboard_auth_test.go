package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestNewLauncherDashboardSessionCookie(t *testing.T) {
	a, err := NewLauncherDashboardSessionCookie()
	if err != nil {
		t.Fatalf("NewLauncherDashboardSessionCookie() error = %v", err)
	}
	b, err := NewLauncherDashboardSessionCookie()
	if err != nil {
		t.Fatalf("NewLauncherDashboardSessionCookie() second error = %v", err)
	}
	if a == "" || b == "" {
		t.Fatalf("session cookie values should be non-empty: %q %q", a, b)
	}
	if a == b {
		t.Fatal("session cookie values should be random")
	}
}

func mustLocalAutoLogin(t *testing.T, ttl time.Duration) *LauncherDashboardLocalAutoLogin {
	t.Helper()
	autoLogin, err := NewLauncherDashboardLocalAutoLogin(ttl)
	if err != nil {
		t.Fatalf("NewLauncherDashboardLocalAutoLogin() error = %v", err)
	}
	return autoLogin
}

func TestLauncherDashboardAuth_AllowsPublicPaths(t *testing.T) {
	cfg := LauncherDashboardAuthConfig{ExpectedCookie: "deadbeef"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := LauncherDashboardAuth(cfg, next)

	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/launcher-login", http.StatusTeapot},
		{http.MethodGet, "/launcher-setup", http.StatusTeapot},
		{http.MethodGet, "/assets/index.js", http.StatusTeapot},
		{http.MethodPost, "/api/auth/login", http.StatusTeapot},
		{http.MethodGet, "/api/auth/status", http.StatusTeapot},
		{http.MethodPost, "/api/auth/setup", http.StatusTeapot},
		{http.MethodPost, "/api/auth/logout", http.StatusTeapot},
		{http.MethodGet, "/api/auth/logout", http.StatusUnauthorized},
		{http.MethodGet, "/api/config", http.StatusUnauthorized},
		{http.MethodGet, "/pico/ws", http.StatusUnauthorized},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s %s: status = %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}

func TestLauncherDashboardAuth_QueryTokenDoesNotAuthenticate(t *testing.T) {
	cfg := LauncherDashboardAuthConfig{ExpectedCookie: "deadbeef"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not run without session cookie")
	})
	h := LauncherDashboardAuth(cfg, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/launcher-login" {
		t.Fatalf("GET /?token=secret: code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestLauncherDashboardAuth_LocalAutoLogin(t *testing.T) {
	const cookieVal = "session-cookie-value"
	autoLogin := mustLocalAutoLogin(t, time.Minute)
	cfg := LauncherDashboardAuthConfig{
		ExpectedCookie: cookieVal,
		LocalAutoLogin: autoLogin,
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := LauncherDashboardAuth(cfg, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, LauncherDashboardLocalAutoLoginPath, nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/launcher-login" ||
		len(rec.Result().Cookies()) != 0 {
		t.Fatalf(
			"auto-login without nonce code=%d loc=%q cookies=%#v",
			rec.Code,
			rec.Header().Get("Location"),
			rec.Result().Cookies(),
		)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, LauncherDashboardLocalAutoLoginPath+"?nonce=wrong", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/launcher-login" ||
		len(rec.Result().Cookies()) != 0 {
		t.Fatalf(
			"auto-login with wrong nonce code=%d loc=%q cookies=%#v",
			rec.Code,
			rec.Header().Get("Location"),
			rec.Result().Cookies(),
		)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodHead, autoLogin.URLPath(), nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/launcher-login" ||
		len(rec.Result().Cookies()) != 0 {
		t.Fatalf(
			"auto-login HEAD code=%d loc=%q cookies=%#v",
			rec.Code,
			rec.Header().Get("Location"),
			rec.Result().Cookies(),
		)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, autoLogin.URLPath(), nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("local auto-login code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != LauncherDashboardCookieName || cookies[0].Value != cookieVal {
		t.Fatalf("cookies = %#v", cookies)
	}
	if cookies[0].MaxAge != 31*24*3600 {
		t.Fatalf("session cookie MaxAge = %d, want 31 days", cookies[0].MaxAge)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: LauncherDashboardCookieName, Value: cookieVal})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie auth after auto-login status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, autoLogin.URLPath(), nil)
	req.AddCookie(&http.Cookie{Name: LauncherDashboardCookieName, Value: cookieVal})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("auto-login path with existing session code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, autoLogin.URLPath(), nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/launcher-login" {
		t.Fatalf("consumed auto-login code=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestLauncherDashboardAuth_LocalAutoLoginRequiresValidNonceAndUnexpired(t *testing.T) {
	const cookieVal = "session-cookie-value"
	newHandler := func(autoLogin *LauncherDashboardLocalAutoLogin) http.Handler {
		return LauncherDashboardAuth(LauncherDashboardAuthConfig{
			ExpectedCookie: cookieVal,
			LocalAutoLogin: autoLogin,
		}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	}

	autoLogin := mustLocalAutoLogin(t, time.Minute)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, autoLogin.URLPath(), nil)
	req.RemoteAddr = "192.168.1.50:12345"
	req.Host = "192.168.1.50:18800"
	newHandler(autoLogin).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || len(rec.Result().Cookies()) != 1 {
		t.Fatalf("capability auto-login code=%d cookies=%#v", rec.Code, rec.Result().Cookies())
	}

	expired := mustLocalAutoLogin(t, -time.Second)
	h := newHandler(expired)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, expired.URLPath(), nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("expired auto-login code=%d cookies=%#v", rec.Code, rec.Result().Cookies())
	}
}

func TestLauncherDashboardAuth_DotDotCannotBypass(t *testing.T) {
	cfg := LauncherDashboardAuthConfig{ExpectedCookie: "deadbeef"}
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not run without auth")
	})
	h := LauncherDashboardAuth(cfg, next)

	for _, p := range []string{
		"/assets/../api/config",
		"/launcher-login/../api/config",
		"/./api/config",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%q: status = %d, want %d", p, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestLauncherDashboardAuth_CookieOnly(t *testing.T) {
	cookieVal := "session-cookie-value"
	cfg := LauncherDashboardAuthConfig{ExpectedCookie: cookieVal}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := LauncherDashboardAuth(cfg, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: LauncherDashboardCookieName, Value: cookieVal})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie auth: status = %d", rec.Code)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req2.Header.Set("Authorization", "Bearer dashboard-secret-9")
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("bearer auth should not be accepted: status = %d", rec2.Code)
	}
}

func TestLauncherDashboardAuth_WebSocketUnauthorizedDoesNotRedirect(t *testing.T) {
	cfg := LauncherDashboardAuthConfig{ExpectedCookie: "deadbeef"}
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not run without auth")
	})
	h := LauncherDashboardAuth(cfg, next)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pico/ws", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("Location"); got != "" {
		t.Fatalf("Location = %q, want empty", got)
	}
}

func TestLauncherDashboardAuth_RedirectWithXForwardedPrefix(t *testing.T) {
	cfg := LauncherDashboardAuthConfig{ExpectedCookie: "deadbeef"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not run without session cookie")
	})
	h := LauncherDashboardAuth(cfg, next)

	for _, tc := range []struct {
		prefix string
		want   string
	}{
		{"", "/launcher-login"},
		{"/picoclaw", "/picoclaw/launcher-login"},
		{"/picoclaw/", "/picoclaw/launcher-login"},
		{"picoclaw", "/picoclaw/launcher-login"},
		{"/", "/launcher-login"},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
		if tc.prefix != "" {
			req.Header.Set("X-Forwarded-Prefix", tc.prefix)
		}
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			t.Fatalf("prefix %q: code = %d, want %d", tc.prefix, rec.Code, http.StatusFound)
		}
		if got := rec.Header().Get("Location"); got != tc.want {
			t.Fatalf("prefix %q: Location = %q, want %q", tc.prefix, got, tc.want)
		}
	}
}

func TestLauncherDashboardAuth_RedirectWithEnvVar(t *testing.T) {
	cfg := LauncherDashboardAuthConfig{ExpectedCookie: "deadbeef"}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not run without session cookie")
	})
	h := LauncherDashboardAuth(cfg, next)

	// Test PICOCLAW_BASE_PATH
	os.Setenv("PICOCLAW_BASE_PATH", "/pico-env")
	defer os.Unsetenv("PICOCLAW_BASE_PATH")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Location"); got != "/pico-env/launcher-login" {
		t.Fatalf("PICOCLAW_BASE_PATH env redirect Location = %q, want %q", got, "/pico-env/launcher-login")
	}

	// Test BASE_PATH fallback
	os.Unsetenv("PICOCLAW_BASE_PATH")
	os.Setenv("BASE_PATH", "/base-env")
	defer os.Unsetenv("BASE_PATH")

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	h.ServeHTTP(rec2, req2)
	if got := rec2.Header().Get("Location"); got != "/base-env/launcher-login" {
		t.Fatalf("BASE_PATH env redirect Location = %q, want %q", got, "/base-env/launcher-login")
	}

	// Test X-Forwarded-Prefix priority over env vars
	os.Setenv("PICOCLAW_BASE_PATH", "/pico-env")
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	req3.Header.Set("X-Forwarded-Prefix", "/header-prefix")
	h.ServeHTTP(rec3, req3)
	if got := rec3.Header().Get("Location"); got != "/header-prefix/launcher-login" {
		t.Fatalf("X-Forwarded-Prefix header should override env vars: Location = %q, want %q", got, "/header-prefix/launcher-login")
	}
}


