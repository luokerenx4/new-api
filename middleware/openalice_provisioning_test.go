package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setOpenAliceProvisioningTokenForTest(t *testing.T, token string) {
	t.Helper()
	original := common.OpenAliceProvisioningToken
	t.Cleanup(func() {
		common.OpenAliceProvisioningToken = original
	})
	common.OpenAliceProvisioningToken = token
}

func performOpenAliceProvisioningAuthRequest(token string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/openalice/provisioning/ping", OpenAliceProvisioningAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/openalice/provisioning/ping", nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestOpenAliceProvisioningAuthDisabledWithoutConfiguredToken(t *testing.T) {
	setOpenAliceProvisioningTokenForTest(t, "")

	recorder := performOpenAliceProvisioningAuthRequest("anything")

	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestOpenAliceProvisioningAuthRejectsInvalidToken(t *testing.T) {
	setOpenAliceProvisioningTokenForTest(t, "secret")

	recorder := performOpenAliceProvisioningAuthRequest("wrong")

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestOpenAliceProvisioningAuthAcceptsBearerToken(t *testing.T) {
	setOpenAliceProvisioningTokenForTest(t, "secret")

	recorder := performOpenAliceProvisioningAuthRequest("secret")

	require.Equal(t, http.StatusOK, recorder.Code)
}
