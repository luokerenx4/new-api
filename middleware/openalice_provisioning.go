package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func OpenAliceProvisioningAuth() func(c *gin.Context) {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(common.OpenAliceProvisioningToken)
		if expected == "" {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "OpenAlice provisioning API is not configured.",
			})
			c.Abort()
			return
		}

		token := strings.TrimSpace(c.GetHeader("X-OpenAlice-Provisioning-Token"))
		if token == "" {
			token = strings.TrimSpace(c.GetHeader("Authorization"))
			token = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(token, "Bearer "), "bearer "))
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Invalid OpenAlice provisioning token.",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
