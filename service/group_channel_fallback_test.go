package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestResolveFallbackLookupGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("nil context returns empty", func(t *testing.T) {
		assert.Equal(t, "", ResolveFallbackLookupGroup(nil, &RetryParam{TokenGroup: "default"}))
	})

	t.Run("auto group uses actual auto_group", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyAutoGroup, "vip")

		got := ResolveFallbackLookupGroup(c, &RetryParam{TokenGroup: "auto"})
		assert.Equal(t, "vip", got)
	})

	t.Run("using_group wins when it is not auto", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "pro")

		got := ResolveFallbackLookupGroup(c, &RetryParam{TokenGroup: "default"})
		assert.Equal(t, "pro", got)
	})

	t.Run("falls back to token group", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		got := ResolveFallbackLookupGroup(c, &RetryParam{TokenGroup: "default"})
		assert.Equal(t, "default", got)
	})

	t.Run("auto without resolved group returns empty", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")

		got := ResolveFallbackLookupGroup(c, &RetryParam{TokenGroup: "auto"})
		assert.Equal(t, "", got)
	})
}
