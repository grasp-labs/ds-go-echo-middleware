package utils

import (
	"fmt"
	"strings"

	"github.com/grasp-labs/ds-go-echo-middleware/v2/middleware/interfaces"
	"github.com/labstack/echo/v4"
)

// Move to package utils
func GetMajorVersion(v string) string {
	version := strings.TrimPrefix(v, "v")
	parts := strings.Split(version, ".")
	if len(parts) > 0 && parts[0] != "" {
		return "v" + parts[0]
	}
	return "v1"
}

func CreateServicePrincipleID(cfg interfaces.Config) string {
	mv := GetMajorVersion(cfg.Version())
	return fmt.Sprintf("%s.%s.%s.%s", cfg.Domain(), cfg.ServiceGroup(), cfg.Name(), mv)
}

// Locale is a helper function that safely parses
// interface to string, returning def (default) on error
func Locale(c echo.Context, def string) string {
	v := c.Get("locale")
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}
