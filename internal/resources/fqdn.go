package resources

import (
	"context"
	"fmt"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &fqdnResource{}
	_ resource.ResourceWithImportState = &fqdnResource{}
)

type fqdnResource struct {
	data *ProviderData
}

type fqdnModel struct {
	ID         types.String `tfsdk:"id"`
	CustomerID types.String `tfsdk:"customer_id"`
	FQDN       types.String `tfsdk:"fqdn"`
	WebappID   types.String `tfsdk:"webapp_id"`
	SSLEnabled types.Bool   `tfsdk:"ssl_enabled"`
	Status     types.String `tfsdk:"status"`
}

type fqdnAPI struct {
	ID         string  `json:"id"`
	AccountID  string  `json:"account_id"`
	FQDN       string  `json:"fqdn"`
	WebappID   *string `json:"webapp_id"`
	SSLEnabled bool    `json:"ssl_enabled"`
	Status     string  `json:"status"`
}

func NewFQDN() resource.Resource {
	return &fqdnResource{}
}

func (r *fqdnResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_fqdn"
}

func (r *fqdnResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a hostname/FQDN binding.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "FQDN ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"customer_id": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Customer ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"fqdn": schema.StringAttribute{
				Required: true, Description: "Fully qualified domain name.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"webapp_id": schema.StringAttribute{
				Optional: true, Description: "Webapp ID to attach this FQDN to.",
			},
			"ssl_enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Description: "Enable SSL certificate.",
				Default: booldefault.StaticBool(true),
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *fqdnResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *fqdnResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan fqdnModel
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
		"fqdn":        plan.FQDN.ValueString(),
		"ssl_enabled": plan.SSLEnabled.ValueBool(),
	}
	if !plan.WebappID.IsNull() && !plan.WebappID.IsUnknown() {
		body["webapp_id"] = plan.WebappID.ValueString()
	}

	result, err := hosting.Post[fqdnAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/hostnames", customerID), body)
	if err != nil {
		resp.Diagnostics.AddError("Create FQDN Failed", err.Error())
		return
	}

	final, err := waitForActive[fqdnAPI](ctx, r.data.Client, "/api/v1/hostnames/"+result.ID, func(f *fqdnAPI) string { return f.Status })
	if err != nil {
		resp.Diagnostics.AddWarning("FQDN Not Yet Active", err.Error())
		final = result
	}

	r.mapToState(final, &plan, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *fqdnResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state fqdnModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[fqdnAPI](ctx, r.data.Client, "/api/v1/hostnames/"+state.ID.ValueString())
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read FQDN Failed", err.Error())
		return
	}

	r.mapToState(result, &state, state.CustomerID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *fqdnResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan fqdnModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state fqdnModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"ssl_enabled": plan.SSLEnabled.ValueBool(),
	}
	if !plan.WebappID.IsNull() && !plan.WebappID.IsUnknown() {
		body["webapp_id"] = plan.WebappID.ValueString()
	} else {
		body["webapp_id"] = nil
	}

	result, err := hosting.Put[fqdnAPI](ctx, r.data.Client, "/api/v1/hostnames/"+state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Update FQDN Failed", err.Error())
		return
	}

	r.mapToState(result, &plan, state.CustomerID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *fqdnResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state fqdnModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/hostnames/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete FQDN Failed", err.Error())
	}
}

func (r *fqdnResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[fqdnAPI](ctx, r.data.Client, "/api/v1/hostnames/"+req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import FQDN Failed", err.Error())
		return
	}
	var state fqdnModel
	r.mapToState(result, &state, r.data.CustomerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *fqdnResource) mapToState(api *fqdnAPI, state *fqdnModel, customerID string) {
	state.ID = types.StringValue(api.ID)
	state.FQDN = types.StringValue(api.FQDN)
	state.SSLEnabled = types.BoolValue(api.SSLEnabled)
	state.Status = types.StringValue(api.Status)
	if api.WebappID != nil {
		state.WebappID = types.StringValue(*api.WebappID)
	} else {
		state.WebappID = types.StringNull()
	}
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}
}
