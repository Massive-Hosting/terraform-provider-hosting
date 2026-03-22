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
	_ resource.Resource                = &websiteCronJobResource{}
	_ resource.ResourceWithImportState = &websiteCronJobResource{}
)

type websiteCronJobResource struct {
	data *ProviderData
}

type websiteCronJobModel struct {
	ID               types.String `tfsdk:"id"`
	CustomerID       types.String `tfsdk:"customer_id"`
	WebsiteID         types.String `tfsdk:"website_id"`
	Schedule         types.String `tfsdk:"schedule"`
	Command          types.String `tfsdk:"command"`
	WorkingDirectory types.String `tfsdk:"working_directory"`
	TimeoutSeconds   types.Int64  `tfsdk:"timeout_seconds"`
	MaxMemoryMB      types.Int64  `tfsdk:"max_memory_mb"`
	Enabled          types.Bool   `tfsdk:"enabled"`
	Status           types.String `tfsdk:"status"`
	StatusMessage    types.String `tfsdk:"status_message"`
}

type cronJobAPI struct {
	ID               string  `json:"id"`
	WebsiteID         string  `json:"website_id"`
	Schedule         string  `json:"schedule"`
	Command          string  `json:"command"`
	WorkingDirectory string  `json:"working_directory"`
	TimeoutSeconds   int64   `json:"timeout_seconds"`
	MaxMemoryMB      int64   `json:"max_memory_mb"`
	Enabled          bool    `json:"enabled"`
	Status           string  `json:"status"`
	StatusMessage    *string `json:"status_message"`
}

func NewWebsiteCronJob() resource.Resource {
	return &websiteCronJobResource{}
}

func (r *websiteCronJobResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_website_cron_job"
}

func (r *websiteCronJobResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a cron job for a website.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Cron job ID.",
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
			"schedule": schema.StringAttribute{
				Required: true, Description: "Cron schedule expression (e.g. '*/5 * * * *').",
			},
			"command": schema.StringAttribute{
				Required: true, Description: "Command to run.",
			},
			"working_directory": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Working directory for the command.",
			},
			"timeout_seconds": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Maximum execution time in seconds (0 = no limit).",
				Default: int64default.StaticInt64(0),
			},
			"max_memory_mb": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Memory limit in MB (0 = unlimited).",
				Default: int64default.StaticInt64(0),
			},
			"enabled": schema.BoolAttribute{
				Computed: true, Description: "Whether the cron job is enabled.",
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

func (r *websiteCronJobResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *websiteCronJobResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan websiteCronJobModel
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
		"schedule":        plan.Schedule.ValueString(),
		"command":         plan.Command.ValueString(),
		"timeout_seconds": plan.TimeoutSeconds.ValueInt64(),
		"max_memory_mb":   plan.MaxMemoryMB.ValueInt64(),
	}
	if !plan.WorkingDirectory.IsNull() && !plan.WorkingDirectory.IsUnknown() {
		body["working_directory"] = plan.WorkingDirectory.ValueString()
	}

	result, err := hosting.Post[cronJobAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/websites/%s/cron-jobs", plan.WebsiteID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Cron Job Failed", err.Error())
		return
	}

	mapCronJob(result, &plan, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *websiteCronJobResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state websiteCronJobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[cronJobAPI](ctx, r.data.Client, "/api/v1/cron-jobs/"+state.ID.ValueString())
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Cron Job Failed", err.Error())
		return
	}

	customerID := state.CustomerID.ValueString()
	mapCronJob(result, &state, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *websiteCronJobResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan websiteCronJobModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state websiteCronJobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"schedule":        plan.Schedule.ValueString(),
		"command":         plan.Command.ValueString(),
		"timeout_seconds": plan.TimeoutSeconds.ValueInt64(),
		"max_memory_mb":   plan.MaxMemoryMB.ValueInt64(),
	}
	if !plan.WorkingDirectory.IsNull() && !plan.WorkingDirectory.IsUnknown() {
		body["working_directory"] = plan.WorkingDirectory.ValueString()
	}

	result, err := hosting.Put[cronJobAPI](ctx, r.data.Client, "/api/v1/cron-jobs/"+state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Update Cron Job Failed", err.Error())
		return
	}

	customerID := state.CustomerID.ValueString()
	mapCronJob(result, &plan, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *websiteCronJobResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state websiteCronJobModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/cron-jobs/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete Cron Job Failed", err.Error())
	}
}

func (r *websiteCronJobResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[cronJobAPI](ctx, r.data.Client, "/api/v1/cron-jobs/"+req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import Cron Job Failed", err.Error())
		return
	}

	var state websiteCronJobModel
	mapCronJob(result, &state, r.data.CustomerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapCronJob(api *cronJobAPI, state *websiteCronJobModel, customerID string) {
	state.ID = types.StringValue(api.ID)
	state.WebsiteID = types.StringValue(api.WebsiteID)
	state.Schedule = types.StringValue(api.Schedule)
	state.Command = types.StringValue(api.Command)
	state.WorkingDirectory = types.StringValue(api.WorkingDirectory)
	state.TimeoutSeconds = types.Int64Value(api.TimeoutSeconds)
	state.MaxMemoryMB = types.Int64Value(api.MaxMemoryMB)
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
