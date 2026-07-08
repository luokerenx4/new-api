package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const openAliceManagedModeMessage = "OpenAlice managed mode delegates account signup, billing, and entitlement changes to OpenAlice Cloud."

func requireStandaloneConsumerSelfService(c *gin.Context) bool {
	if !common.OpenAliceManagedMode {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": openAliceManagedModeMessage,
		"code":    "openalice_managed_mode",
	})
	return false
}
