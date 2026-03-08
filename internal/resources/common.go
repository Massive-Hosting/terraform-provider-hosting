package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// ProviderData is passed from provider.Configure to each resource.
type ProviderData struct {
	Client     *hosting.Client
	CustomerID string
}

// DefaultTimeout for async resource provisioning.
const DefaultTimeout = 5 * time.Minute

// configureClient extracts the client from provider data.
func configureClient(req any, resp *resource.ConfigureResponse) *ProviderData {
	var raw any
	switch r := req.(type) {
	case resource.ConfigureRequest:
		raw = r.ProviderData
	}

	if raw == nil {
		return nil
	}

	data, ok := raw.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Data", fmt.Sprintf("Expected *ProviderData, got %T", raw))
		return nil
	}

	return data
}

// waitForActive polls until the resource reaches "active" status.
func waitForActive[T any](ctx context.Context, c *hosting.Client, path string, getStatus func(*T) string) (*T, error) {
	return hosting.WaitForStatus(ctx, c, path, getStatus, DefaultTimeout)
}
