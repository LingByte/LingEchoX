package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestT_enUS(t *testing.T) {
	if got := T(LocaleEnUS, KeyAuthInvalidCredentials); got != "Invalid email or password" {
		t.Fatalf("got %q", got)
	}
}

func TestTGin_acceptLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	SetLocaleOnGin(c, ParseAcceptLanguage(c.Request.Header.Get("Accept-Language")))
	if got := TGin(c, KeyTenantRegisterDisabled); got == "" || got == KeyTenantRegisterDisabled {
		t.Fatalf("unexpected %q", got)
	}
	if got := TGin(c, KeyTenantRegisterDisabled); got != enUS[KeyTenantRegisterDisabled] {
		t.Fatalf("got %q", got)
	}
}
