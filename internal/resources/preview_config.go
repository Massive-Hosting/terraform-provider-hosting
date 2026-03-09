package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/massive-hosting/go-hosting"
)

var (
	_ resource.Resource                = &previewConfigResource{}
	_ resource.ResourceWithImportState = &previewConfigResource{}
)

type previewConfigResource struct {
	data *ProviderData
}

type previewConfigModel struct {
	ID                   types.String `tfsdk:"id"`
	WebappID             types.String `tfsdk:"webapp_id"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	GitHubRepoOwner      types.String `tfsdk:"github_repo_owner"`
	GitHubRepoName       types.String `tfsdk:"github_repo_name"`
	GitHubInstallationID types.Int64  `tfsdk:"github_installation_id"`
	WorkflowFilename     types.String `tfsdk:"workflow_filename"`
	DatabaseMode         types.String `tfsdk:"database_mode"`
	SourceDatabaseID     types.String `tfsdk:"source_database_id"`
	ValkeyMode           types.String `tfsdk:"valkey_mode"`
	SourceValkeyID       types.String `tfsdk:"source_valkey_id"`
	S3Mode               types.String `tfsdk:"s3_mode"`
	EnvVarOverrides      types.List   `tfsdk:"env_var_overrides"`
	AutoDestroyHours     types.Int64  `tfsdk:"auto_destroy_hours"`
	Status               types.String `tfsdk:"status"`
}

type previewConfigAPI struct {
	ID                   string              `json:"id"`
	WebappID             string              `json:"webapp_id"`
	Enabled              bool                `json:"enabled"`
	GitHubRepoOwner      string              `json:"github_repo_owner"`
	GitHubRepoName       string              `json:"github_repo_name"`
	GitHubInstallationID int64               `json:"github_installation_id"`
	WorkflowFilename     string              `json:"workflow_filename"`
	DatabaseMode         string              `json:"database_mode"`
	SourceDatabaseID     *string             `json:"source_database_id"`
	ValkeyMode           string              `json:"valkey_mode"`
	SourceValkeyID       *string             `json:"source_valkey_id"`
	S3Mode               string              `json:"s3_mode"`
	EnvVarOverrides      []envVarOverrideAPI `json:"env_var_overrides"`
	AutoDestroyHours     int64               `json:"auto_destroy_hours"`
}

type envVarOverrideAPI struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

var envVarOverrideAttrTypes = map[string]attr.Type{
	"name":  types.StringType,
	"value": types.StringType,
}

func NewPreviewConfig() resource.Resource {
	return &previewConfigResource{}
}

func (r *previewConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_preview_config"
}

func (r *previewConfigResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a preview environment configuration for a webapp.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Preview config ID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"webapp_id": schema.StringAttribute{
				Required:    true,
				Description: "Webapp ID to attach preview config to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether preview environments are enabled.",
				Default:     booldefault.StaticBool(true),
			},
			"github_repo_owner": schema.StringAttribute{
				Required:    true,
				Description: "GitHub repository owner (organization or user).",
			},
			"github_repo_name": schema.StringAttribute{
				Required:    true,
				Description: "GitHub repository name.",
			},
			"github_installation_id": schema.Int64Attribute{
				Required:    true,
				Description: "GitHub App installation ID.",
			},
			"workflow_filename": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Deploy workflow filename.",
				Default:     stringdefault.StaticString("deploy.yml"),
			},
			"database_mode": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Database provisioning mode: none, empty, or clone.",
				Default:     stringdefault.StaticString("none"),
			},
			"source_database_id": schema.StringAttribute{
				Optional:    true,
				Description: "Source database ID for clone mode.",
			},
			"valkey_mode": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Valkey provisioning mode: none or empty.",
				Default:     stringdefault.StaticString("none"),
			},
			"source_valkey_id": schema.StringAttribute{
				Optional:    true,
				Description: "Source Valkey instance ID for config reference.",
			},
			"s3_mode": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "S3 provisioning mode: none or empty.",
				Default:     stringdefault.StaticString("none"),
			},
			"env_var_overrides": schema.ListNestedAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Environment variable overrides with template variable support.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Environment variable name.",
						},
						"value": schema.StringAttribute{
							Required:    true,
							Description: "Value, supports template variables like {{PREVIEW_URL}}, {{DB_HOST}}, etc.",
						},
					},
				},
			},
			"auto_destroy_hours": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Hours before preview environments are auto-destroyed.",
				Default:     int64default.StaticInt64(72),
			},
			"status": schema.StringAttribute{
				Computed:    true,
				Description: "Current status.",
			},
		},
	}
}

func (r *previewConfigResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *previewConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan previewConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Put[previewConfigAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/webapps/%s/preview-config", plan.WebappID.ValueString()), r.buildBody(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Create Preview Config Failed", err.Error())
		return
	}

	r.mapToState(result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *previewConfigResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state previewConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[previewConfigAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/webapps/%s/preview-config", state.WebappID.ValueString()))
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Preview Config Failed", err.Error())
		return
	}

	r.mapToState(result, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *previewConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan previewConfigModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Put[previewConfigAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/webapps/%s/preview-config", plan.WebappID.ValueString()), r.buildBody(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Update Preview Config Failed", err.Error())
		return
	}

	r.mapToState(result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *previewConfigResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state previewConfigModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, fmt.Sprintf("/api/v1/webapps/%s/preview-config", state.WebappID.ValueString())); err != nil {
		resp.Diagnostics.AddError("Delete Preview Config Failed", err.Error())
	}
}

func (r *previewConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	webappID := req.ID

	result, err := hosting.Get[previewConfigAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/webapps/%s/preview-config", webappID))
	if err != nil {
		resp.Diagnostics.AddError("Import Preview Config Failed", err.Error())
		return
	}

	var state previewConfigModel
	state.WebappID = types.StringValue(webappID)
	r.mapToState(result, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *previewConfigResource) buildBody(plan *previewConfigModel) map[string]any {
	body := map[string]any{
		"enabled":                  plan.Enabled.ValueBool(),
		"github_repo_owner":       plan.GitHubRepoOwner.ValueString(),
		"github_repo_name":        plan.GitHubRepoName.ValueString(),
		"github_installation_id":  plan.GitHubInstallationID.ValueInt64(),
		"workflow_filename":       plan.WorkflowFilename.ValueString(),
		"database_mode":           plan.DatabaseMode.ValueString(),
		"valkey_mode":             plan.ValkeyMode.ValueString(),
		"s3_mode":                 plan.S3Mode.ValueString(),
		"auto_destroy_hours":      plan.AutoDestroyHours.ValueInt64(),
	}

	if !plan.SourceDatabaseID.IsNull() && !plan.SourceDatabaseID.IsUnknown() {
		body["source_database_id"] = plan.SourceDatabaseID.ValueString()
	}
	if !plan.SourceValkeyID.IsNull() && !plan.SourceValkeyID.IsUnknown() {
		body["source_valkey_id"] = plan.SourceValkeyID.ValueString()
	}

	var overrides []map[string]string
	if !plan.EnvVarOverrides.IsNull() && !plan.EnvVarOverrides.IsUnknown() {
		for _, elem := range plan.EnvVarOverrides.Elements() {
			obj := elem.(types.Object)
			attrs := obj.Attributes()
			overrides = append(overrides, map[string]string{
				"name":  attrs["name"].(types.String).ValueString(),
				"value": attrs["value"].(types.String).ValueString(),
			})
		}
	}
	if overrides == nil {
		overrides = []map[string]string{}
	}
	body["env_var_overrides"] = overrides

	return body
}

func (r *previewConfigResource) mapToState(api *previewConfigAPI, state *previewConfigModel) {
	state.ID = types.StringValue(api.ID)
	state.WebappID = types.StringValue(api.WebappID)
	state.Enabled = types.BoolValue(api.Enabled)
	state.GitHubRepoOwner = types.StringValue(api.GitHubRepoOwner)
	state.GitHubRepoName = types.StringValue(api.GitHubRepoName)
	state.GitHubInstallationID = types.Int64Value(api.GitHubInstallationID)
	state.WorkflowFilename = types.StringValue(api.WorkflowFilename)
	state.DatabaseMode = types.StringValue(api.DatabaseMode)
	state.ValkeyMode = types.StringValue(api.ValkeyMode)
	state.S3Mode = types.StringValue(api.S3Mode)
	state.AutoDestroyHours = types.Int64Value(api.AutoDestroyHours)
	state.Status = types.StringValue("active")

	if api.SourceDatabaseID != nil {
		state.SourceDatabaseID = types.StringValue(*api.SourceDatabaseID)
	} else {
		state.SourceDatabaseID = types.StringNull()
	}
	if api.SourceValkeyID != nil {
		state.SourceValkeyID = types.StringValue(*api.SourceValkeyID)
	} else {
		state.SourceValkeyID = types.StringNull()
	}

	overrideObjs := make([]attr.Value, len(api.EnvVarOverrides))
	for i, o := range api.EnvVarOverrides {
		overrideObjs[i], _ = types.ObjectValue(envVarOverrideAttrTypes, map[string]attr.Value{
			"name":  types.StringValue(o.Name),
			"value": types.StringValue(o.Value),
		})
	}
	state.EnvVarOverrides, _ = types.ListValue(types.ObjectType{AttrTypes: envVarOverrideAttrTypes}, overrideObjs)
}
