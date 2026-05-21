package ginutil

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParamID_invalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}

	_, ok := ParamID(c, "id")
	if ok {
		t.Fatal("expected ok=false")
	}
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestQueryPage_defaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?page=0&size=0", nil)

	page, size := QueryPage(c, 100)
	if page != 1 || size != 20 {
		t.Fatalf("page=%d size=%d", page, size)
	}
}
