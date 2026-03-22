package resources

import (
	"context"
	"fmt"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &websiteDaemonResource{}
	_ resource.ResourceWithImportState = &websiteDaemonResource{}
)

type websiteDaemonResource struct {
	data *ProviderData
}

type websiteDaemonModel struct {
	ID            types.String `tfsdk:"id"`
	CustomerID    types.String `tfsdk:"customer_id"`
	WebsiteID      types.String `tfsdk:"website_id"`
	Command       types.String `tfsdk:"command"`
	ProxyPath     types.String `tfsdk:"proxy_path"`
	ProxyPort     types.Int64  `tfsdk:"proxy_port"`
	StopSignal    types.String `tfsdk:"stop_signal"`
	StopWaitSecs  types.Int64  `tfsdk:"stop_wait_secs"`
	MaxMemoryMB      types.Int64  `tfsdk:"max_memory_mb"`
	RestartPolicy    types.String `tfsdk:"restart_policy"`
	RestartMaxRetries types.Int64 `tfsdk:"restart_max_retries"`
	Enabled          types.Bool   `tfsdk:"enabled"`
	Status        types.String `tfsdk:"status"`
	StatusMessage types.String `tfsdk:"status_message"`
}

type daemonAPI struct {
	ID            string  `json:"id"`
	WebsiteID      string  `json:"website_id"`
	Command       string  `json:"command"`
	ProxyPath     string  `json:"proxy_path"`
	ProxyPort     int64   `json:"proxy_port"`
	StopSignal    string  `json:"stop_signal"`
	StopWaitSecs  int64   `json:"stop_wait_secs"`
	MaxMemoryMB      int64  `json:"max_memory_mb"`
	RestartPolicy    string `json:"restart_policy"`
	RestartMaxRetries int64 `json:"restart_max_retries"`
	Enabled          bool   `json:"enabled"`
	Status        string  `json:"status"`
	StatusMessage *string `json:"status_message"`
}

func NewWebsiteDaemon() resource.Resource {
	return &websiteDaemonResource{}
}

func (r *websiteDaemonResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_website_daemon"
}

func (r *websiteDaemonResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a daemon process for a website.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Daemon ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"customer_id": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Customer ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"website_id": schema.StringAttribute{
				Required: true, Description: "Website ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"command": schema.StringAttribute{
				Required: true, Description: "Command to run.",
			},
			"proxy_path": schema.StringAttribute{
				Optional: true, Computed: true, Description: "URL path prefix to proxy to this daemon (e.g. /api).",
				Default: stringdefault.StaticString(""),
			},
			"proxy_port": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Port the daemon listens on for proxied requests.",
				Default: int64default.StaticInt64(0),
			},
			"stop_signal": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Signal to send when stopping (TERM, INT, QUIT, KILL).",
				Default: stringdefault.StaticString("TERM"),
			},
			"stop_wait_secs": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Seconds to wait after stop signal before killing.",
				Default: int64default.StaticInt64(10),
			},
			"max_memory_mb": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Memory limit in MB (0 = unlimited).",
				Default: int64default.StaticInt64(0),
			},
			"restart_policy": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Restart policy: no, on-failure, always.",
				Default: stringdefault.StaticString("on-failure"),
			},
			"restart_max_retries": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Max restart attempts before giving up (0 = unlimited).",
				Default: int64default.StaticInt64(0),
			},
			"enabled": schema.BoolAttribute{
				Computed: true, Description: "Whether the daemon is enabled.",
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
			"status_message": schema.StringAttribute{
				Computed: true, Description: "Status message (set on failure).",
			},
		},
	}
}

func (r *websiteDaemonResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *websiteDaemonResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan websiteDaemonModel
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
		"command":             plan.Command.ValueString(),
		"proxy_path":          plan.ProxyPath.ValueString(),
		"proxy_port":          plan.ProxyPort.ValueInt64(),
		"stop_signal":         plan.StopSignal.ValueString(),
		"stop_wait_secs":      plan.StopWaitSecs.ValueInt64(),
		"max_memory_mb":       plan.MaxMemoryMB.ValueInt64(),
		"restart_policy":      plan.RestartPolicy.ValueString(),
		"restart_max_retries": plan.RestartMaxRetries.ValueInt64(),
	}

	result, err := hosting.Post[daemonAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/websites/%s/daemons", plan.WebsiteID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Daemon Failed", err.Error())
		return
	}

	mapDaemon(result, &plan, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *websiteDaemonResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state websiteDaemonModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[daemonAPI](ctx, r.data.Client, "/api/v1/daemons/"+state.ID.ValueString())
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Daemon Failed", err.Error())
		return
	}

	customerID := state.CustomerID.ValueString()
	mapDaemon(result, &state, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *websiteDaemonResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan websiteDaemonModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state websiteDaemonModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"command":             plan.Command.ValueString(),
		"proxy_path":          plan.ProxyPath.ValueString(),
		"proxy_port":          plan.ProxyPort.ValueInt64(),
		"stop_signal":         plan.StopSignal.ValueString(),
		"stop_wait_secs":      plan.StopWaitSecs.ValueInt64(),
		"max_memory_mb":       plan.MaxMemoryMB.ValueInt64(),
		"restart_policy":      plan.RestartPolicy.ValueString(),
		"restart_max_retries": plan.RestartMaxRetries.ValueInt64(),
	}

	result, err := hosting.Put[daemonAPI](ctx, r.data.Client, "/api/v1/daemons/"+state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Update Daemon Failed", err.Error())
		return
	}

	customerID := state.CustomerID.ValueString()
	mapDaemon(result, &plan, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *websiteDaemonResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state websiteDaemonModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/daemons/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete Daemon Failed", err.Error())
	}
}

func (r *websiteDaemonResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[daemonAPI](ctx, r.data.Client, "/api/v1/daemons/"+req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import Daemon Failed", err.Error())
		return
	}

	var state websiteDaemonModel
	mapDaemon(result, &state, r.data.CustomerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapDaemon(api *daemonAPI, state *websiteDaemonModel, customerID string) {
	state.ID = types.StringValue(api.ID)
	state.WebsiteID = types.StringValue(api.WebsiteID)
	state.Command = types.StringValue(api.Command)
	state.ProxyPath = types.StringValue(api.ProxyPath)
	state.ProxyPort = types.Int64Value(api.ProxyPort)
	state.StopSignal = types.StringValue(api.StopSignal)
	state.StopWaitSecs = types.Int64Value(api.StopWaitSecs)
	state.MaxMemoryMB = types.Int64Value(api.MaxMemoryMB)
	state.RestartPolicy = types.StringValue(api.RestartPolicy)
	state.RestartMaxRetries = types.Int64Value(api.RestartMaxRetries)
	state.Enabled = types.BoolValue(api.Enabled)
	state.Status = types.StringValue(api.Status)
	if api.StatusMessage != nil {
		state.StatusMessage = types.StringValue(*api.StatusMessage)
	} else {
		state.StatusMessage = types.StringValue("")
	}
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}
}
