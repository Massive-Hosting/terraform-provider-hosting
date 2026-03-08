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
	_ resource.Resource                = &valkeyUserResource{}
	_ resource.ResourceWithImportState = &valkeyUserResource{}
)

type valkeyUserResource struct {
	data *ProviderData
}

type valkeyUserModel struct {
	ID               types.String `tfsdk:"id"`
	ValkeyInstanceID types.String `tfsdk:"valkey_instance_id"`
	Username         types.String `tfsdk:"username"`
	Password         types.String `tfsdk:"password"`
	Privileges       types.List   `tfsdk:"privileges"`
	KeyPattern       types.String `tfsdk:"key_pattern"`
	Status           types.String `tfsdk:"status"`
}

type valkeyUserAPI struct {
	ID               string   `json:"id"`
	ValkeyInstanceID string   `json:"valkey_instance_id"`
	Username         string   `json:"username"`
	Password         string   `json:"password,omitempty"`
	Privileges       []string `json:"privileges"`
	KeyPattern       string   `json:"key_pattern"`
	Status           string   `json:"status"`
}

func NewValkeyUser() resource.Resource {
	return &valkeyUserResource{}
}

func (r *valkeyUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_valkey_user"
}

func (r *valkeyUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Valkey (Redis) user.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Valkey user ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"valkey_instance_id": schema.StringAttribute{
				Required: true, Description: "Parent Valkey instance ID.",
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
				Description: "List of Valkey ACL privileges.",
				ElementType: types.StringType,
			},
			"key_pattern": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Key pattern for ACL (e.g. *).",
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *valkeyUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *valkeyUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan valkeyUserModel
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
	if !plan.KeyPattern.IsNull() && !plan.KeyPattern.IsUnknown() {
		body["key_pattern"] = plan.KeyPattern.ValueString()
	}

	result, err := hosting.Post[valkeyUserAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/valkey/%s/users", plan.ValkeyInstanceID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Valkey User Failed", err.Error())
		return
	}

	r.mapToState(ctx, result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *valkeyUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state valkeyUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	users, err := hosting.List[valkeyUserAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/valkey/%s/users", state.ValkeyInstanceID.ValueString()))
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Valkey User Failed", err.Error())
		return
	}

	var found *valkeyUserAPI
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

func (r *valkeyUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan valkeyUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state valkeyUserModel
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
	if !plan.KeyPattern.IsNull() && !plan.KeyPattern.IsUnknown() {
		body["key_pattern"] = plan.KeyPattern.ValueString()
	}

	result, err := hosting.Put[valkeyUserAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/valkey/%s/users/%s", state.ValkeyInstanceID.ValueString(), state.ID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Update Valkey User Failed", err.Error())
		return
	}

	r.mapToState(ctx, result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *valkeyUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state valkeyUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, fmt.Sprintf("/api/v1/valkey/%s/users/%s", state.ValkeyInstanceID.ValueString(), state.ID.ValueString())); err != nil {
		resp.Diagnostics.AddError("Delete Valkey User Failed", err.Error())
	}
}

func (r *valkeyUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid Import ID", "Expected format: valkey_instance_id/user_id")
		return
	}

	users, err := hosting.List[valkeyUserAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/valkey/%s/users", parts[0]))
	if err != nil {
		resp.Diagnostics.AddError("Import Valkey User Failed", err.Error())
		return
	}

	var found *valkeyUserAPI
	for i := range users {
		if users[i].ID == parts[1] {
			found = &users[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("Import Valkey User Failed", "User not found")
		return
	}

	var state valkeyUserModel
	r.mapToState(ctx, found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *valkeyUserResource) mapToState(ctx context.Context, api *valkeyUserAPI, state *valkeyUserModel) {
	state.ID = types.StringValue(api.ID)
	state.ValkeyInstanceID = types.StringValue(api.ValkeyInstanceID)
	state.Username = types.StringValue(api.Username)
	state.KeyPattern = types.StringValue(api.KeyPattern)
	state.Status = types.StringValue(api.Status)
	if api.Password != "" {
		state.Password = types.StringValue(api.Password)
	}
	privs, _ := types.ListValueFrom(ctx, types.StringType, api.Privileges)
	state.Privileges = privs
}
