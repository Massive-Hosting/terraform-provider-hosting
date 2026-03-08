package resources

import (
	"context"
	"fmt"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &wireGuardPeerResource{}
	_ resource.ResourceWithImportState = &wireGuardPeerResource{}
)

type wireGuardPeerResource struct {
	data *ProviderData
}

type wireGuardPeerModel struct {
	ID           types.String `tfsdk:"id"`
	CustomerID   types.String `tfsdk:"customer_id"`
	TenantID     types.String `tfsdk:"tenant_id"`
	Name         types.String `tfsdk:"name"`
	PublicKey    types.String `tfsdk:"public_key"`
	AssignedIP   types.String `tfsdk:"assigned_ip"`
	Endpoint     types.String `tfsdk:"endpoint"`
	PrivateKey   types.String `tfsdk:"private_key"`
	ClientConfig types.String `tfsdk:"client_config"`
	Status       types.String `tfsdk:"status"`
}

type wireGuardPeerAPI struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	Name       string `json:"name"`
	PublicKey  string `json:"public_key"`
	AssignedIP string `json:"assigned_ip"`
	Endpoint   string `json:"endpoint"`
	Status     string `json:"status"`
}

type wireGuardCreateResult struct {
	Peer         wireGuardPeerAPI `json:"peer"`
	PrivateKey   string           `json:"private_key"`
	ClientConfig string           `json:"client_config"`
}

func NewWireGuardPeer() resource.Resource {
	return &wireGuardPeerResource{}
}

func (r *wireGuardPeerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wireguard_peer"
}

func (r *wireGuardPeerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a WireGuard VPN peer.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Peer ID.",
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
			"name": schema.StringAttribute{
				Required: true, Description: "Peer name.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_key": schema.StringAttribute{
				Computed: true, Description: "WireGuard public key.",
			},
			"assigned_ip": schema.StringAttribute{
				Computed: true, Description: "Assigned VPN IP address.",
			},
			"endpoint": schema.StringAttribute{
				Computed: true, Description: "WireGuard endpoint address.",
			},
			"private_key": schema.StringAttribute{
				Computed: true, Sensitive: true,
				Description: "WireGuard private key. Only available on creation.",
			},
			"client_config": schema.StringAttribute{
				Computed: true, Sensitive: true,
				Description: "Complete WireGuard client configuration. Only available on creation.",
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *wireGuardPeerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *wireGuardPeerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wireGuardPeerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customerID := resolveCustomerID(plan.CustomerID, r.data)
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "Set customer_id on the resource or in the provider config.")
		return
	}

	result, err := hosting.Post[wireGuardCreateResult](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/wireguard", customerID), map[string]any{
		"tenant_id": plan.TenantID.ValueString(),
		"name":      plan.Name.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Create WireGuard Peer Failed", err.Error())
		return
	}

	plan.ID = types.StringValue(result.Peer.ID)
	plan.TenantID = types.StringValue(result.Peer.TenantID)
	plan.Name = types.StringValue(result.Peer.Name)
	plan.PublicKey = types.StringValue(result.Peer.PublicKey)
	plan.AssignedIP = types.StringValue(result.Peer.AssignedIP)
	plan.Endpoint = types.StringValue(result.Peer.Endpoint)
	plan.PrivateKey = types.StringValue(result.PrivateKey)
	plan.ClientConfig = types.StringValue(result.ClientConfig)
	plan.Status = types.StringValue(result.Peer.Status)
	plan.CustomerID = types.StringValue(customerID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wireGuardPeerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wireGuardPeerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[wireGuardPeerAPI](ctx, r.data.Client, "/api/v1/wireguard/"+state.ID.ValueString())
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read WireGuard Peer Failed", err.Error())
		return
	}

	// Preserve one-time secrets from state
	privateKey := state.PrivateKey
	clientConfig := state.ClientConfig
	customerID := state.CustomerID

	state.ID = types.StringValue(result.ID)
	state.TenantID = types.StringValue(result.TenantID)
	state.Name = types.StringValue(result.Name)
	state.PublicKey = types.StringValue(result.PublicKey)
	state.AssignedIP = types.StringValue(result.AssignedIP)
	state.Endpoint = types.StringValue(result.Endpoint)
	state.Status = types.StringValue(result.Status)
	state.PrivateKey = privateKey
	state.ClientConfig = clientConfig
	state.CustomerID = customerID

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *wireGuardPeerResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "WireGuard peers cannot be updated. Delete and recreate instead.")
}

func (r *wireGuardPeerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wireGuardPeerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/wireguard/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete WireGuard Peer Failed", err.Error())
	}
}

func (r *wireGuardPeerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[wireGuardPeerAPI](ctx, r.data.Client, "/api/v1/wireguard/"+req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import WireGuard Peer Failed", err.Error())
		return
	}

	var state wireGuardPeerModel
	state.ID = types.StringValue(result.ID)
	state.TenantID = types.StringValue(result.TenantID)
	state.Name = types.StringValue(result.Name)
	state.PublicKey = types.StringValue(result.PublicKey)
	state.AssignedIP = types.StringValue(result.AssignedIP)
	state.Endpoint = types.StringValue(result.Endpoint)
	state.Status = types.StringValue(result.Status)
	state.PrivateKey = types.StringValue("")   // Not available on import
	state.ClientConfig = types.StringValue("") // Not available on import
	if r.data.CustomerID != "" {
		state.CustomerID = types.StringValue(r.data.CustomerID)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
