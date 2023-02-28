package monitoring

import (
	"os"
	"strings"
)

var allowedTrueValues = []string{"1", "yes", "y", "true"}

func EnvBool(key string, defaultValue bool) bool {
	valueStr := strings.ToLower(os.Getenv(key))
	if valueStr == "" {
		return defaultValue
	}

	for _, value := range allowedTrueValues {
		if valueStr == value {
			return true
		}
	}

	return false
}
