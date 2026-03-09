package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/massive-hosting/go-hosting"
)

var (
	_ resource.Resource                = &webhookEndpointResource{}
	_ resource.ResourceWithImportState = &webhookEndpointResource{}
)

type webhookEndpointResource struct {
	data *ProviderData
}

type webhookEndpointModel struct {
	ID          types.String `tfsdk:"id"`
	CustomerID  types.String `tfsdk:"customer_id"`
	TenantID    types.String `tfsdk:"tenant_id"`
	URL         types.String `tfsdk:"url"`
	Description types.String `tfsdk:"description"`
	Events      types.List   `tfsdk:"events"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Secret      types.String `tfsdk:"secret"`
	CreatedAt   types.String `tfsdk:"created_at"`
}

type webhookEndpointAPI struct {
	ID          string   `json:"id"`
	TenantID    string   `json:"tenant_id"`
	URL         string   `json:"url"`
	Description string   `json:"description"`
	Events      []string `json:"events"`
	Enabled     bool     `json:"enabled"`
	Secret      string   `json:"secret,omitempty"`
	CreatedAt   string   `json:"created_at"`
}

func NewWebhookEndpoint() resource.Resource {
	return &webhookEndpointResource{}
}

func (r *webhookEndpointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook_endpoint"
}

func (r *webhookEndpointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a webhook endpoint that receives HMAC-signed notifications for infrastructure events.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Webhook endpoint ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"customer_id": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Customer ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"tenant_id": schema.StringAttribute{
				Required: true, Description: "Tenant ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"url": schema.StringAttribute{
				Required: true, Description: "Webhook delivery URL.",
			},
			"description": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Human-readable description.",
			},
			"events": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Event types to subscribe to (deploy.success, deploy.failed, backup.completed, ssl.expiring, cron.failed, webapp.status_changed).",
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Description: "Whether the endpoint is enabled.",
				Default: booldefault.StaticBool(true),
			},
			"secret": schema.StringAttribute{
				Computed: true, Sensitive: true, Description: "HMAC signing secret (shown only on create).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				Computed: true, Description: "Creation timestamp.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *webhookEndpointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Provider Data", fmt.Sprintf("Expected *ProviderData, got %T", req.ProviderData))
		return
	}
	r.data = data
}

func (r *webhookEndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookEndpointModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customerID := resolveCustomerID(plan.CustomerID, r.data)
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "Set customer_id on the resource or in the provider config.")
		return
	}

	var events []string
	resp.Diagnostics.Append(plan.Events.ElementsAs(ctx, &events, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"tenant_id":   plan.TenantID.ValueString(),
		"url":         plan.URL.ValueString(),
		"description": plan.Description.ValueString(),
		"events":      events,
	}

	result, err := hosting.Post[webhookEndpointAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/webhook-endpoints", customerID), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Webhook Endpoint Failed", err.Error())
		return
	}

	mapWebhookEndpoint(ctx, result, &plan, customerID, &resp.Diagnostics)
	// Secret is only available on create response.
	if result.Secret != "" {
		plan.Secret = types.StringValue(result.Secret)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookEndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookEndpointModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[webhookEndpointAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/webhook-endpoints/%s", state.ID.ValueString()))
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}

	customerID := state.CustomerID.ValueString()
	if customerID == "" {
		customerID = r.data.CustomerID
	}
	// Preserve secret from state (not returned by GET).
	secret := state.Secret
	mapWebhookEndpoint(ctx, result, &state, customerID, &resp.Diagnostics)
	state.Secret = secret
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webhookEndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webhookEndpointModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var events []string
	resp.Diagnostics.Append(plan.Events.ElementsAs(ctx, &events, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"url":         plan.URL.ValueString(),
		"description": plan.Description.ValueString(),
		"events":      events,
		"enabled":     plan.Enabled.ValueBool(),
	}

	result, err := hosting.Patch[webhookEndpointAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/webhook-endpoints/%s", plan.ID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Update Webhook Endpoint Failed", err.Error())
		return
	}

	customerID := plan.CustomerID.ValueString()
	if customerID == "" {
		customerID = r.data.CustomerID
	}
	// Preserve secret from state.
	var state webhookEndpointModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	secret := state.Secret

	mapWebhookEndpoint(ctx, result, &plan, customerID, &resp.Diagnostics)
	plan.Secret = secret
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookEndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookEndpointModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/webhook-endpoints/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete Webhook Endpoint Failed", err.Error())
	}
}

func (r *webhookEndpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[webhookEndpointAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/webhook-endpoints/%s", req.ID))
	if err != nil {
		resp.Diagnostics.AddError("Import Webhook Endpoint Failed", err.Error())
		return
	}

	customerID := r.data.CustomerID
	var state webhookEndpointModel
	mapWebhookEndpoint(ctx, result, &state, customerID, &resp.Diagnostics)
	// Secret is not available on import; it will be unknown.
	state.Secret = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapWebhookEndpoint(ctx context.Context, api *webhookEndpointAPI, state *webhookEndpointModel, customerID string, diags *diag.Diagnostics) {
	state.ID = types.StringValue(api.ID)
	state.TenantID = types.StringValue(api.TenantID)
	state.URL = types.StringValue(api.URL)
	state.Description = types.StringValue(api.Description)
	state.Enabled = types.BoolValue(api.Enabled)
	state.CreatedAt = types.StringValue(api.CreatedAt)
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}

	eventsList, d := types.ListValueFrom(ctx, types.StringType, api.Events)
	diags.Append(d...)
	state.Events = eventsList
}
