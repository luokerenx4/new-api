package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAlicePublicDataPlaneOnlyDisabledWithoutHosts(t *testing.T) {
	t.Setenv("OPENALICE_PUBLIC_DATA_PLANE_HOSTS", "")
	t.Setenv("OPENALICE_CONTROL_PLANE_HOSTS", "")
	router := openAlicePublicDataPlaneTestRouter()

	recorder := performOpenAlicePublicDataPlaneRequest(router, "alpha-gateway.wond.dev", "/api/openalice/provisioning/ping")

	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestOpenAlicePublicDataPlaneOnlyBlocksControlRoutesOnPublicHost(t *testing.T) {
	t.Setenv("OPENALICE_PUBLIC_DATA_PLANE_HOSTS", "alpha-gateway.wond.dev")
	t.Setenv("OPENALICE_CONTROL_PLANE_HOSTS", "alice-ai-gateway-staging.railway.internal")
	router := openAlicePublicDataPlaneTestRouter()

	recorder := performOpenAlicePublicDataPlaneRequest(router, "alpha-gateway.wond.dev", "/api/openalice/provisioning/ping")

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestOpenAlicePublicDataPlaneOnlyAllowsRelayAndStatusOnPublicHost(t *testing.T) {
	t.Setenv("OPENALICE_PUBLIC_DATA_PLANE_HOSTS", "alpha-gateway.wond.dev")
	t.Setenv("OPENALICE_CONTROL_PLANE_HOSTS", "alice-ai-gateway-staging.railway.internal")
	router := openAlicePublicDataPlaneTestRouter()

	for _, path := range []string{"/api/status", "/api/usage/token/", "/v1/chat/completions", "/v1beta/models/gemini:generateContent", "/mj/submit/imagine", "/suno/fetch"} {
		recorder := performOpenAlicePublicDataPlaneRequest(router, "alpha-gateway.wond.dev:443", path)
		require.Equal(t, http.StatusOK, recorder.Code, path)
	}
}

func TestOpenAlicePublicDataPlaneOnlyBlocksSlashlessUsageRouteRedirect(t *testing.T) {
	t.Setenv("OPENALICE_PUBLIC_DATA_PLANE_HOSTS", "alpha-gateway.wond.dev")
	t.Setenv("OPENALICE_CONTROL_PLANE_HOSTS", "alice-ai-gateway-staging.railway.internal")
	router := openAlicePublicDataPlaneTestRouter()

	recorder := performOpenAlicePublicDataPlaneRequest(router, "alpha-gateway.wond.dev", "/api/usage/token")

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestOpenAlicePublicDataPlaneOnlyAllowsPrivateHost(t *testing.T) {
	t.Setenv("OPENALICE_PUBLIC_DATA_PLANE_HOSTS", "alpha-gateway.wond.dev")
	t.Setenv("OPENALICE_CONTROL_PLANE_HOSTS", "alice-ai-gateway-staging.railway.internal")
	router := openAlicePublicDataPlaneTestRouter()

	recorder := performOpenAlicePublicDataPlaneRequest(router, "alice-ai-gateway-staging.railway.internal:3000", "/api/openalice/provisioning/ping")

	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestOpenAlicePublicDataPlaneOnlyTreatsUnknownHostsAsDataPlane(t *testing.T) {
	t.Setenv("OPENALICE_PUBLIC_DATA_PLANE_HOSTS", "alpha-gateway.wond.dev")
	t.Setenv("OPENALICE_CONTROL_PLANE_HOSTS", "alice-ai-gateway-staging.railway.internal")
	router := openAlicePublicDataPlaneTestRouter()

	controlRoute := performOpenAlicePublicDataPlaneRequest(router, "random-railway-domain.up.railway.app", "/api/openalice/provisioning/ping")
	relayRoute := performOpenAlicePublicDataPlaneRequest(router, "random-railway-domain.up.railway.app", "/v1/chat/completions")

	require.Equal(t, http.StatusNotFound, controlRoute.Code)
	require.Equal(t, http.StatusOK, relayRoute.Code)
}

func openAlicePublicDataPlaneTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(OpenAlicePublicDataPlaneOnly())
	router.Any("/*path", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	return router
}

func performOpenAlicePublicDataPlaneRequest(router http.Handler, host string, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	router.ServeHTTP(recorder, req)
	return recorder
}
