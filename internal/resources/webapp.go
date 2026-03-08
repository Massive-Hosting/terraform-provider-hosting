package resources

import (
	"context"
	"fmt"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &webappResource{}
	_ resource.ResourceWithImportState = &webappResource{}
)

type webappResource struct {
	data *ProviderData
}

type webappModel struct {
	ID                     types.String `tfsdk:"id"`
	CustomerID             types.String `tfsdk:"customer_id"`
	TenantID               types.String `tfsdk:"tenant_id"`
	Runtime                types.String `tfsdk:"runtime"`
	RuntimeVersion         types.String `tfsdk:"runtime_version"`
	PublicFolder           types.String `tfsdk:"public_folder"`
	EnvFileName            types.String `tfsdk:"env_file_name"`
	ServiceHostnameEnabled types.Bool   `tfsdk:"service_hostname_enabled"`
	Status                 types.String `tfsdk:"status"`
}

type webappAPI struct {
	ID                     string `json:"id"`
	TenantID               string `json:"tenant_id"`
	Runtime                string `json:"runtime"`
	RuntimeVersion         string `json:"runtime_version"`
	PublicFolder           string `json:"public_folder"`
	EnvFileName            string `json:"env_file_name"`
	ServiceHostnameEnabled bool   `json:"service_hostname_enabled"`
	Status                 string `json:"status"`
}

func NewWebapp() resource.Resource {
	return &webappResource{}
}

func (r *webappResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webapp"
}

func (r *webappResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a web application.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Webapp ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"customer_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Customer ID. Defaults to provider customer_id.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant_id": schema.StringAttribute{
				Required:    true,
				Description: "Tenant ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"runtime": schema.StringAttribute{
				Required:    true,
				Description: "Runtime type (e.g. php, nodejs, python, ruby, static).",
			},
			"runtime_version": schema.StringAttribute{
				Required:    true,
				Description: "Runtime version (e.g. 8.4, 22, 3.13).",
			},
			"public_folder": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Public folder relative to app root (e.g. public).",
				Default:     stringdefault.StaticString(""),
			},
			"env_file_name": schema.StringAttribute{
				Computed:    true,
				Description: "Environment file name.",
			},
			"service_hostname_enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the built-in service hostname is enabled.",
				Default:     booldefault.StaticBool(true),
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current status.",
			},
		},
	}
}

func (r *webappResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *webappResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webappModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customerID := r.resolveCustomerID(plan.CustomerID)
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "Set customer_id on the resource or in the provider config.")
		return
	}

	body := map[string]any{
		"tenant_id":       plan.TenantID.ValueString(),
		"runtime":         plan.Runtime.ValueString(),
		"runtime_version": plan.RuntimeVersion.ValueString(),
	}
	if !plan.PublicFolder.IsNull() && !plan.PublicFolder.IsUnknown() {
		body["public_folder"] = plan.PublicFolder.ValueString()
	}

	result, err := hosting.Post[webappAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/webapps", customerID), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Webapp Failed", err.Error())
		return
	}

	// Wait for active status
	final, err := waitForActive[webappAPI](ctx, r.data.Client, "/api/v1/webapps/"+result.ID, func(w *webappAPI) string { return w.Status })
	if err != nil {
		resp.Diagnostics.AddWarning("Webapp Not Yet Active", err.Error())
		final = result
	}

	r.mapToState(final, &plan, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webappResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webappModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[webappAPI](ctx, r.data.Client, "/api/v1/webapps/"+state.ID.ValueString())
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Webapp Failed", err.Error())
		return
	}

	r.mapToState(result, &state, state.CustomerID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webappResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webappModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state webappModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"runtime":                  plan.Runtime.ValueString(),
		"runtime_version":          plan.RuntimeVersion.ValueString(),
		"public_folder":            plan.PublicFolder.ValueString(),
		"service_hostname_enabled": plan.ServiceHostnameEnabled.ValueBool(),
	}

	result, err := hosting.Put[webappAPI](ctx, r.data.Client, "/api/v1/webapps/"+state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Update Webapp Failed", err.Error())
		return
	}

	r.mapToState(result, &plan, state.CustomerID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webappResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webappModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Webapps don't have a delete endpoint — they're removed by deleting the tenant.
	// For now, we just remove from state.
	resp.Diagnostics.AddWarning("Webapp Removal", "Webapps cannot be individually deleted. Removed from Terraform state only.")
}

func (r *webappResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[webappAPI](ctx, r.data.Client, "/api/v1/webapps/"+req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import Webapp Failed", err.Error())
		return
	}

	var state webappModel
	r.mapToState(result, &state, r.data.CustomerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webappResource) mapToState(api *webappAPI, state *webappModel, customerID string) {
	state.ID = types.StringValue(api.ID)
	state.TenantID = types.StringValue(api.TenantID)
	state.Runtime = types.StringValue(api.Runtime)
	state.RuntimeVersion = types.StringValue(api.RuntimeVersion)
	state.PublicFolder = types.StringValue(api.PublicFolder)
	state.EnvFileName = types.StringValue(api.EnvFileName)
	state.ServiceHostnameEnabled = types.BoolValue(api.ServiceHostnameEnabled)
	state.Status = types.StringValue(api.Status)
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}
}

func (r *webappResource) resolveCustomerID(v types.String) string {
	if !v.IsNull() && !v.IsUnknown() {
		return v.ValueString()
	}
	return r.data.CustomerID
}
