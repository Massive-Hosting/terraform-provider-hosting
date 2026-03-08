package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &databaseUserResource{}
	_ resource.ResourceWithImportState = &databaseUserResource{}
)

type databaseUserResource struct {
	data *ProviderData
}

type databaseUserModel struct {
	ID         types.String `tfsdk:"id"`
	DatabaseID types.String `tfsdk:"database_id"`
	Username   types.String `tfsdk:"username"`
	Password   types.String `tfsdk:"password"`
	Privileges types.List   `tfsdk:"privileges"`
	Status     types.String `tfsdk:"status"`
}

type databaseUserAPI struct {
	ID         string   `json:"id"`
	DatabaseID string   `json:"database_id"`
	Username   string   `json:"username"`
	Password   string   `json:"password,omitempty"`
	Privileges []string `json:"privileges"`
	Status     string   `json:"status"`
}

func NewDatabaseUser() resource.Resource {
	return &databaseUserResource{}
}

func (r *databaseUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database_user"
}

func (r *databaseUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a MySQL database user.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Database user ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"database_id": schema.StringAttribute{
				Required: true, Description: "Parent database ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"username": schema.StringAttribute{
				Computed: true, Description: "Generated username.",
			},
			"password": schema.StringAttribute{
				Optional: true, Computed: true, Sensitive: true,
				Description: "User password. Generated if not provided.",
			},
			"privileges": schema.ListAttribute{
				Optional: true, Computed: true,
				Description: "List of MySQL privileges (e.g. SELECT, INSERT, UPDATE, DELETE, ALL).",
				ElementType: types.StringType,
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *databaseUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *databaseUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan databaseUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{}
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() {
		body["password"] = plan.Password.ValueString()
	}
	if !plan.Privileges.IsNull() && !plan.Privileges.IsUnknown() {
		var privs []string
		resp.Diagnostics.Append(plan.Privileges.ElementsAs(ctx, &privs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body["privileges"] = privs
	}

	result, err := hosting.Post[databaseUserAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/databases/%s/users", plan.DatabaseID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Database User Failed", err.Error())
		return
	}

	r.mapToState(ctx, result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *databaseUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state databaseUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// List users and find by ID (no single-user GET endpoint)
	users, err := hosting.List[databaseUserAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/databases/%s/users", state.DatabaseID.ValueString()))
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Database User Failed", err.Error())
		return
	}

	var found *databaseUserAPI
	for i := range users {
		if users[i].ID == state.ID.ValueString() {
			found = &users[i]
			break
		}
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	r.mapToState(ctx, found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *databaseUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan databaseUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state databaseUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{}
	if !plan.Password.IsNull() && !plan.Password.IsUnknown() {
		body["password"] = plan.Password.ValueString()
	}
	if !plan.Privileges.IsNull() && !plan.Privileges.IsUnknown() {
		var privs []string
		resp.Diagnostics.Append(plan.Privileges.ElementsAs(ctx, &privs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body["privileges"] = privs
	}

	result, err := hosting.Put[databaseUserAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/databases/%s/users/%s", state.DatabaseID.ValueString(), state.ID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Update Database User Failed", err.Error())
		return
	}

	r.mapToState(ctx, result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *databaseUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state databaseUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, fmt.Sprintf("/api/v1/databases/%s/users/%s", state.DatabaseID.ValueString(), state.ID.ValueString())); err != nil {
		resp.Diagnostics.AddError("Delete Database User Failed", err.Error())
	}
}

func (r *databaseUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID format: "database_id/user_id"
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid Import ID", "Expected format: database_id/user_id")
		return
	}

	users, err := hosting.List[databaseUserAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/databases/%s/users", parts[0]))
	if err != nil {
		resp.Diagnostics.AddError("Import Database User Failed", err.Error())
		return
	}

	var found *databaseUserAPI
	for i := range users {
		if users[i].ID == parts[1] {
			found = &users[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("Import Database User Failed", "User not found")
		return
	}

	var state databaseUserModel
	r.mapToState(ctx, found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *databaseUserResource) mapToState(ctx context.Context, api *databaseUserAPI, state *databaseUserModel) {
	state.ID = types.StringValue(api.ID)
	state.DatabaseID = types.StringValue(api.DatabaseID)
	state.Username = types.StringValue(api.Username)
	state.Status = types.StringValue(api.Status)
	if api.Password != "" {
		state.Password = types.StringValue(api.Password)
	}
	privs, _ := types.ListValueFrom(ctx, types.StringType, api.Privileges)
	state.Privileges = privs
}
