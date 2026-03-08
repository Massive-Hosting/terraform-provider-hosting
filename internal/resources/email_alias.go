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
	_ resource.Resource                = &emailAliasResource{}
	_ resource.ResourceWithImportState = &emailAliasResource{}
)

type emailAliasResource struct {
	data *ProviderData
}

type emailAliasModel struct {
	ID             types.String `tfsdk:"id"`
	EmailAccountID types.String `tfsdk:"email_account_id"`
	Address        types.String `tfsdk:"address"`
	Status         types.String `tfsdk:"status"`
}

type emailAliasAPI struct {
	ID             string `json:"id"`
	EmailAccountID string `json:"email_account_id"`
	Address        string `json:"address"`
	Status         string `json:"status"`
}

func NewEmailAlias() resource.Resource {
	return &emailAliasResource{}
}

func (r *emailAliasResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_email_alias"
}

func (r *emailAliasResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an email alias.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Alias ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"email_account_id": schema.StringAttribute{
				Required: true, Description: "Parent email account ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"address": schema.StringAttribute{
				Required: true, Description: "Alias address.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *emailAliasResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *emailAliasResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan emailAliasModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Post[emailAliasAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/email/%s/aliases", plan.EmailAccountID.ValueString()), map[string]any{
		"address": plan.Address.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Create Email Alias Failed", err.Error())
		return
	}

	mapEmailAlias(result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailAliasResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state emailAliasModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	aliases, err := hosting.List[emailAliasAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/email/%s/aliases", state.EmailAccountID.ValueString()))
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Email Alias Failed", err.Error())
		return
	}

	var found *emailAliasAPI
	for i := range aliases {
		if aliases[i].ID == state.ID.ValueString() {
			found = &aliases[i]
			break
		}
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	mapEmailAlias(found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emailAliasResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "Email aliases cannot be updated. Delete and recreate instead.")
}

func (r *emailAliasResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state emailAliasModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, fmt.Sprintf("/api/v1/email/%s/aliases/%s", state.EmailAccountID.ValueString(), state.ID.ValueString())); err != nil {
		resp.Diagnostics.AddError("Delete Email Alias Failed", err.Error())
	}
}

func (r *emailAliasResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid Import ID", "Expected format: email_account_id/alias_id")
		return
	}

	aliases, err := hosting.List[emailAliasAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/email/%s/aliases", parts[0]))
	if err != nil {
		resp.Diagnostics.AddError("Import Email Alias Failed", err.Error())
		return
	}

	var found *emailAliasAPI
	for i := range aliases {
		if aliases[i].ID == parts[1] {
			found = &aliases[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("Import Email Alias Failed", "Alias not found")
		return
	}

	var state emailAliasModel
	mapEmailAlias(found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapEmailAlias(api *emailAliasAPI, state *emailAliasModel) {
	state.ID = types.StringValue(api.ID)
	state.EmailAccountID = types.StringValue(api.EmailAccountID)
	state.Address = types.StringValue(api.Address)
	state.Status = types.StringValue(api.Status)
}
