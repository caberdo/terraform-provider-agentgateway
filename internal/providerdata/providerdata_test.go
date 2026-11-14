package providerdata

import (
	"testing"
	"time"

	"github.com/caberdo/terraform-provider-agentgateway/internal/client"
)

// TestDataShape pins the typed provider configuration handed to every resource
// and data source. The struct is the contract between Configure and the CRUD
// harness, so a field rename or reordering that would break a consumer shows up
// here instead of at apply time.
func TestDataShape(t *testing.T) {
	t.Parallel()

	var data Data

	if data.Client != nil {
		t.Errorf("zero Data.Client = %v, want nil", data.Client)
	}
	if data.Timeout != 0 {
		t.Errorf("zero Data.Timeout = %s, want 0", data.Timeout)
	}
	if data.StoreMode != "" {
		t.Errorf("zero Data.StoreMode = %q, want empty", data.StoreMode)
	}
	if data.Version != "" {
		t.Errorf("zero Data.Version = %q, want empty", data.Version)
	}

	data = Data{
		Client:    nil,
		Timeout:   time.Minute,
		StoreMode: "hybrid",
		Version:   "1.7.0",
	}
	if data.Timeout != time.Minute {
		t.Errorf("Data.Timeout = %s, want 1m", data.Timeout)
	}
	if data.StoreMode != "hybrid" {
		t.Errorf("Data.StoreMode = %q, want hybrid", data.StoreMode)
	}
	if data.Version != "1.7.0" {
		t.Errorf("Data.Version = %q, want 1.7.0", data.Version)
	}

	// The Client field must keep accepting a client.Client, which is what the
	// provider's Configure stores.
	var _ client.Client = data.Client
}
