package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMemoryModelRequestRateLimitCountsOnlySuccessfulRequestsForSuccessCap(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalRedisEnabled := common.RedisEnabled
	originalEnabled := setting.ModelRequestRateLimitEnabled
	originalDuration := setting.ModelRequestRateLimitDurationMinutes
	originalCount := setting.ModelRequestRateLimitCount
	originalSuccessCount := setting.ModelRequestRateLimitSuccessCount
	originalGroup := setting.ModelRequestRateLimitGroup2JSONString()
	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		setting.ModelRequestRateLimitEnabled = originalEnabled
		setting.ModelRequestRateLimitDurationMinutes = originalDuration
		setting.ModelRequestRateLimitCount = originalCount
		setting.ModelRequestRateLimitSuccessCount = originalSuccessCount
		_ = setting.UpdateModelRequestRateLimitGroupByJSONString(originalGroup)
		inMemoryRateLimiter = common.InMemoryRateLimiter{}
	})

	common.RedisEnabled = false
	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 1
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{"free":[0,1]}`))
	inMemoryRateLimiter = common.InMemoryRateLimiter{}

	router := gin.New()
	withGatewayIdentity := func(c *gin.Context) {
		c.Set("id", 42)
		c.Set(string(constant.ContextKeyTokenGroup), "free")
		c.Next()
	}
	router.GET("/fail", withGatewayIdentity, ModelRequestRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})
	router.GET("/ok", withGatewayIdentity, ModelRequestRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	require.Equal(t, http.StatusInternalServerError, performModelRateLimitRequest(router, "/fail").Code)
	require.Equal(t, http.StatusInternalServerError, performModelRateLimitRequest(router, "/fail").Code)
	require.Equal(t, http.StatusOK, performModelRateLimitRequest(router, "/ok").Code)
	require.Equal(t, http.StatusTooManyRequests, performModelRateLimitRequest(router, "/ok").Code)
}

func performModelRateLimitRequest(router http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	router.ServeHTTP(recorder, req)
	return recorder
}
