package resourceutil

import (
	"errors"
	"fmt"
	"strings"

	"github.com/caberdo/terraform-provider-agentgateway/internal/configresource"
)

// ParseImportID accepts a bare id or a kind-qualified id ("kind/id"). A
// mismatched kind is rejected.
func ParseImportID(kind configresource.Kind, raw string) (string, error) {
	if raw == "" {
		return "", errors.New("import id is empty")
	}

	parts := strings.SplitN(raw, "/", 2)
	if len(parts) == 1 {
		return raw, nil
	}

	if configresource.Kind(parts[0]) != kind {
		return "", fmt.Errorf("expected import id for kind %s", kind)
	}
	if parts[1] == "" {
		return "", errors.New("import id is empty")
	}

	return parts[1], nil
}
