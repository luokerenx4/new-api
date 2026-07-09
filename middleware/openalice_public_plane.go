package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func OpenAlicePublicDataPlaneOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !OpenAlicePlaneBoundaryConfigured() {
			c.Next()
			return
		}
		if openAliceIsControlPlaneHost(c.Request.Host) {
			c.Next()
			return
		}
		if openAliceIsPublicDataPlanePath(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "not found",
		})
		c.Abort()
	}
}

func OpenAlicePlaneBoundaryConfigured() bool {
	return strings.TrimSpace(os.Getenv("OPENALICE_CONTROL_PLANE_HOSTS")) != "" ||
		strings.TrimSpace(os.Getenv("OPENALICE_PUBLIC_DATA_PLANE_HOSTS")) != ""
}

func openAliceIsControlPlaneHost(requestHost string) bool {
	return openAliceHostInList(requestHost, os.Getenv("OPENALICE_CONTROL_PLANE_HOSTS"))
}

func openAliceHostInList(requestHost string, hosts string) bool {
	hosts = strings.TrimSpace(hosts)
	if hosts == "" {
		return false
	}
	requestHost = normalizeOpenAliceHost(requestHost)
	if requestHost == "" {
		return false
	}
	for _, host := range strings.Split(hosts, ",") {
		if requestHost == normalizeOpenAliceHost(host) {
			return true
		}
	}
	return false
}

func normalizeOpenAliceHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	return strings.Trim(host, "[]")
}

func openAliceIsPublicDataPlanePath(method string, path string) bool {
	if method == http.MethodOptions {
		return true
	}
	path = strings.TrimSpace(path)
	if path == "/api/status" || strings.HasPrefix(path, "/api/usage/token/") {
		return true
	}
	for _, prefix := range []string{
		"/v1",
		"/v1beta",
		"/kling/v1",
		"/mj",
		"/suno",
	} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
