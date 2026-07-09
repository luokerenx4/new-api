package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCollectionRouteRegistersBothTrailingSlashForms(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.RedirectTrailingSlash = false
	collection := router.Group("/api/channel")
	collectionRoute(collection, http.MethodGet, func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for _, path := range []string{"/api/channel", "/api/channel/"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusNoContent, recorder.Code, path)
	}
}

func TestSetAPIRouterRegistersWithoutConflictingPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.RedirectTrailingSlash = false

	require.NotPanics(t, func() {
		SetApiRouter(router)
	})

	routes := make(map[string]struct{}, len(router.Routes()))
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, route := range []string{
		http.MethodGet + " /api/channel",
		http.MethodGet + " /api/channel/",
		http.MethodPost + " /api/channel",
		http.MethodPost + " /api/channel/",
		http.MethodGet + " /api/data",
		http.MethodGet + " /api/data/",
	} {
		_, ok := routes[route]
		require.True(t, ok, "missing route %s", route)
	}
}
