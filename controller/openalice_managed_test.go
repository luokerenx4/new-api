package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setOpenAliceManagedModeForTest(t *testing.T, enabled bool) {
	t.Helper()
	original := common.OpenAliceManagedMode
	t.Cleanup(func() {
		common.OpenAliceManagedMode = original
	})
	common.OpenAliceManagedMode = enabled
}

func TestRequireStandaloneConsumerSelfServiceAllowsWhenManagedModeOff(t *testing.T) {
	setOpenAliceManagedModeForTest(t, false)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	require.True(t, requireStandaloneConsumerSelfService(c))
}

func TestRequireStandaloneConsumerSelfServiceBlocksWhenManagedModeOn(t *testing.T) {
	setOpenAliceManagedModeForTest(t, true)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	require.False(t, requireStandaloneConsumerSelfService(c))
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "openalice_managed_mode")
}
