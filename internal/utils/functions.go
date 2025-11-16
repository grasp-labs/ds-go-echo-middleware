package utils

import (
	"fmt"
	"strings"

	"github.com/grasp-labs/ds-go-echo-middleware/middleware/interfaces"
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
