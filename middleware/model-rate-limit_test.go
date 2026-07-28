package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMemoryModelRequestRateLimitCountsOnlySuccessfulRequestsForSuccessCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureMemoryModelRateLimit(t, `{"free":[0,1]}`, 0, 1)

	router := gin.New()
	router.GET("/fail", gatewayIdentity(4201, "free"), ModelRequestRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})
	router.GET("/ok", gatewayIdentity(4201, "free"), ModelRequestRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	require.Equal(t, http.StatusInternalServerError, performModelRateLimitRequest(router, "/fail").Code)
	require.Equal(t, http.StatusInternalServerError, performModelRateLimitRequest(router, "/fail").Code)
	require.Equal(t, http.StatusOK, performModelRateLimitRequest(router, "/ok").Code)
	limited := performModelRateLimitRequest(router, "/ok")
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	requireOpenAIErrorMessage(t, limited, "请求数限制")
}

func TestMemoryModelRequestRateLimitReturnsOpenAIErrorForTotalCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureMemoryModelRateLimit(t, `{"free":[1,0]}`, 0, 0)

	router := gin.New()
	router.GET("/ok", gatewayIdentity(4202, "free"), ModelRequestRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	require.Equal(t, http.StatusOK, performModelRateLimitRequest(router, "/ok").Code)
	limited := performModelRateLimitRequest(router, "/ok")
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	requireOpenAIErrorMessage(t, limited, "总请求数限制")
}

func TestMemoryModelRequestRateLimitPrioritizesSuccessCapOverTotalCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureMemoryModelRateLimit(t, `{"free":[1,1]}`, 0, 0)

	router := gin.New()
	router.GET("/ok", gatewayIdentity(4203, "free"), ModelRequestRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	require.Equal(t, http.StatusOK, performModelRateLimitRequest(router, "/ok").Code)
	successLimited := performModelRateLimitRequest(router, "/ok")
	require.Equal(t, http.StatusTooManyRequests, successLimited.Code)
	requireOpenAIErrorMessage(t, successLimited, "请求数限制")
}

func TestMemoryModelRequestRateLimitInheritsUserGroupWhenTokenGroupIsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configureMemoryModelRateLimit(t, `{"scale":[1,1]}`, 100, 100)

	router := gin.New()
	router.GET("/ok", func(c *gin.Context) {
		c.Set("id", 4204)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "scale")
		c.Next()
	}, ModelRequestRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	require.Equal(t, http.StatusOK, performModelRateLimitRequest(router, "/ok").Code)
	limited := performModelRateLimitRequest(router, "/ok")
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	requireOpenAIErrorMessage(t, limited, "请求数限制")
}

func performModelRateLimitRequest(router http.Handler, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	router.ServeHTTP(recorder, req)
	return recorder
}

func configureMemoryModelRateLimit(t *testing.T, groupJSON string, globalTotal, globalSuccess int) {
	t.Helper()
	originalRedisEnabled := common.RedisEnabled
	originalEnabled := setting.ModelRequestRateLimitEnabled
	originalDuration := setting.ModelRequestRateLimitDurationMinutes
	originalCount := setting.ModelRequestRateLimitCount
	originalSuccessCount := setting.ModelRequestRateLimitSuccessCount
	originalGroup := setting.ModelRequestRateLimitGroup2JSONString()

	common.RedisEnabled = false
	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = globalTotal
	setting.ModelRequestRateLimitSuccessCount = globalSuccess
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(groupJSON))

	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		setting.ModelRequestRateLimitEnabled = originalEnabled
		setting.ModelRequestRateLimitDurationMinutes = originalDuration
		setting.ModelRequestRateLimitCount = originalCount
		setting.ModelRequestRateLimitSuccessCount = originalSuccessCount
		_ = setting.UpdateModelRequestRateLimitGroupByJSONString(originalGroup)
	})
}

func gatewayIdentity(userID int, group string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("id", userID)
		c.Set(string(constant.ContextKeyTokenGroup), group)
		c.Next()
	}
}

func requireOpenAIErrorMessage(t *testing.T, recorder *httptest.ResponseRecorder, contains string) {
	t.Helper()
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, "new_api_error", payload.Error.Type)
	require.True(t, strings.Contains(payload.Error.Message, contains), "message %q should contain %q", payload.Error.Message, contains)
}
