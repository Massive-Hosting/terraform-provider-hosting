package resources

import (
	"context"
	"fmt"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &containerResource{}
	_ resource.ResourceWithImportState = &containerResource{}
)

type containerResource struct {
	data *ProviderData
}

type containerModel struct {
	ID            types.String  `tfsdk:"id"`
	TenantID      types.String  `tfsdk:"tenant_id"`
	Name          types.String  `tfsdk:"name"`
	Image         types.String  `tfsdk:"image"`
	Command       types.String  `tfsdk:"command"`
	RestartPolicy types.String  `tfsdk:"restart_policy"`
	MaxMemoryMB   types.Int64   `tfsdk:"max_memory_mb"`
	MaxCPUCores   types.Float64 `tfsdk:"max_cpu_cores"`
	Enabled       types.Bool    `tfsdk:"enabled"`
	Status        types.String  `tfsdk:"status"`
}

type containerAPI struct {
	ID            string  `json:"id"`
	TenantID      string  `json:"tenant_id"`
	Name          string  `json:"name"`
	Image         string  `json:"image"`
	Command       *string `json:"command"`
	RestartPolicy string  `json:"restart_policy"`
	MaxMemoryMB   int64   `json:"max_memory_mb"`
	MaxCPUCores   float64 `json:"max_cpu_cores"`
	Enabled       bool    `json:"enabled"`
	Status        string  `json:"status"`
}

func NewContainer() resource.Resource {
	return &containerResource{}
}

func (r *containerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_container"
}

func (r *containerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an OCI container.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Container ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"tenant_id": schema.StringAttribute{
				Required: true, Description: "Tenant ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required: true, Description: "Container name.",
			},
			"image": schema.StringAttribute{
				Required: true, Description: "Container image (e.g. nginx:latest).",
			},
			"command": schema.StringAttribute{
				Optional: true, Description: "Override command.",
			},
			"restart_policy": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Restart policy (always, on-failure, no).",
				Default: stringdefault.StaticString("always"),
			},
			"max_memory_mb": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Memory limit in MB.",
				Default: int64default.StaticInt64(256),
			},
			"max_cpu_cores": schema.Float64Attribute{
				Optional: true, Computed: true, Description: "CPU cores limit.",
				Default: float64default.StaticFloat64(1.0),
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Description: "Whether the container is enabled.",
				Default: booldefault.StaticBool(true),
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *containerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *containerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan containerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"name":           plan.Name.ValueString(),
		"image":          plan.Image.ValueString(),
		"restart_policy": plan.RestartPolicy.ValueString(),
		"max_memory_mb":  plan.MaxMemoryMB.ValueInt64(),
		"max_cpu_cores":  plan.MaxCPUCores.ValueFloat64(),
		"enabled":        plan.Enabled.ValueBool(),
	}
	if !plan.Command.IsNull() && !plan.Command.IsUnknown() {
		body["command"] = plan.Command.ValueString()
	}

	result, err := hosting.Post[containerAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/tenants/%s/containers", plan.TenantID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Container Failed", err.Error())
		return
	}

	final, err := waitForActive[containerAPI](ctx, r.data.Client, "/api/v1/containers/"+result.ID, func(c *containerAPI) string { return c.Status })
	if err != nil {
		resp.Diagnostics.AddWarning("Container Not Yet Active", err.Error())
		final = result
	}

	mapContainer(final, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *containerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state containerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[containerAPI](ctx, r.data.Client, "/api/v1/containers/"+state.ID.ValueString())
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Container Failed", err.Error())
		return
	}

	mapContainer(result, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *containerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan containerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state containerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"name":           plan.Name.ValueString(),
		"image":          plan.Image.ValueString(),
		"restart_policy": plan.RestartPolicy.ValueString(),
		"max_memory_mb":  plan.MaxMemoryMB.ValueInt64(),
		"max_cpu_cores":  plan.MaxCPUCores.ValueFloat64(),
	}
	if !plan.Command.IsNull() && !plan.Command.IsUnknown() {
		body["command"] = plan.Command.ValueString()
	}

	result, err := hosting.Put[containerAPI](ctx, r.data.Client, "/api/v1/containers/"+state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Update Container Failed", err.Error())
		return
	}

	mapContainer(result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *containerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state containerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/containers/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete Container Failed", err.Error())
	}
}

func (r *containerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[containerAPI](ctx, r.data.Client, "/api/v1/containers/"+req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import Container Failed", err.Error())
		return
	}
	var state containerModel
	mapContainer(result, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapContainer(api *containerAPI, state *containerModel) {
	state.ID = types.StringValue(api.ID)
	state.TenantID = types.StringValue(api.TenantID)
	state.Name = types.StringValue(api.Name)
	state.Image = types.StringValue(api.Image)
	state.RestartPolicy = types.StringValue(api.RestartPolicy)
	state.MaxMemoryMB = types.Int64Value(api.MaxMemoryMB)
	state.MaxCPUCores = types.Float64Value(api.MaxCPUCores)
	state.Enabled = types.BoolValue(api.Enabled)
	state.Status = types.StringValue(api.Status)
	if api.Command != nil {
		state.Command = types.StringValue(*api.Command)
	} else {
		state.Command = types.StringNull()
	}
}
