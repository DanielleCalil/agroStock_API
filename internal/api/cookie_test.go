package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DanielleCalil/agroStock_API/internal/config"
	"github.com/DanielleCalil/agroStock_API/internal/sessao"
)

func TestAtributosCookieSessao(t *testing.T) {
	casos := []struct {
		secure   bool
		sameSite http.SameSite
	}{
		{secure: true, sameSite: http.SameSiteNoneMode},
		{secure: false, sameSite: http.SameSiteLaxMode},
	}
	for _, c := range casos {
		s := NewServer(nil, sessao.NewStore(time.Hour), config.Config{CookieSecure: c.secure})
		rec := httptest.NewRecorder()
		s.apagarCookie(rec)

		cookies := rec.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("secure=%v: %d cookies, quero 1", c.secure, len(cookies))
		}
		ck := cookies[0]
		if ck.Secure != c.secure || ck.SameSite != c.sameSite || !ck.HttpOnly {
			t.Errorf("secure=%v: cookie = Secure:%v SameSite:%v HttpOnly:%v",
				c.secure, ck.Secure, ck.SameSite, ck.HttpOnly)
		}
	}
}
