package resources

import (
	"context"
	"fmt"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &valkeyResource{}
	_ resource.ResourceWithImportState = &valkeyResource{}
)

type valkeyResource struct {
	data *ProviderData
}

type valkeyModel struct {
	ID          types.String `tfsdk:"id"`
	CustomerID  types.String `tfsdk:"customer_id"`
	TenantID    types.String `tfsdk:"tenant_id"`
	Port        types.Int64  `tfsdk:"port"`
	MaxMemoryMB types.Int64  `tfsdk:"max_memory_mb"`
	Status      types.String `tfsdk:"status"`
}

type valkeyAPI struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Port        int64  `json:"port"`
	MaxMemoryMB int64  `json:"max_memory_mb"`
	Status      string `json:"status"`
}

func NewValkey() resource.Resource {
	return &valkeyResource{}
}

func (r *valkeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_valkey"
}

func (r *valkeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Valkey (Redis) instance.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Valkey instance ID.",
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
			"port": schema.Int64Attribute{
				Computed: true, Description: "Assigned port number.",
			},
			"max_memory_mb": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Maximum memory in MB.",
				Default: int64default.StaticInt64(64),
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *valkeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *valkeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan valkeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customerID := resolveCustomerID(plan.CustomerID, r.data)
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "Set customer_id on the resource or in the provider config.")
		return
	}

	body := map[string]any{
		"tenant_id":     plan.TenantID.ValueString(),
		"max_memory_mb": plan.MaxMemoryMB.ValueInt64(),
	}

	result, err := hosting.Post[valkeyAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/valkey", customerID), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Valkey Failed", err.Error())
		return
	}

	final, err := waitForActive[valkeyAPI](ctx, r.data.Client, "/api/v1/valkey/"+result.ID, func(v *valkeyAPI) string { return v.Status })
	if err != nil {
		resp.Diagnostics.AddWarning("Valkey Not Yet Active", err.Error())
		final = result
	}

	mapValkey(final, &plan, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *valkeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state valkeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[valkeyAPI](ctx, r.data.Client, "/api/v1/valkey/"+state.ID.ValueString())
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Valkey Failed", err.Error())
		return
	}

	mapValkey(result, &state, state.CustomerID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *valkeyResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "Valkey instances cannot be updated. Delete and recreate instead.")
}

func (r *valkeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state valkeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/valkey/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete Valkey Failed", err.Error())
	}
}

func (r *valkeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[valkeyAPI](ctx, r.data.Client, "/api/v1/valkey/"+req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import Valkey Failed", err.Error())
		return
	}
	var state valkeyModel
	mapValkey(result, &state, r.data.CustomerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapValkey(api *valkeyAPI, state *valkeyModel, customerID string) {
	state.ID = types.StringValue(api.ID)
	state.TenantID = types.StringValue(api.TenantID)
	state.Port = types.Int64Value(api.Port)
	state.MaxMemoryMB = types.Int64Value(api.MaxMemoryMB)
	state.Status = types.StringValue(api.Status)
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}
}
