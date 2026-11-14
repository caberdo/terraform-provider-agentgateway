package providerdata

import (
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
)

// Data is the typed provider configuration passed to resources and data sources.
type Data struct {
	Client    client.Client
	Timeout   time.Duration
	StoreMode string // "file" | "hybrid" | "readOnly" | "" when the probe was skipped
	Version   string // build.version, "" when unknown
}
