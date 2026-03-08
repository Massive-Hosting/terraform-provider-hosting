package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &emailForwardResource{}
	_ resource.ResourceWithImportState = &emailForwardResource{}
)

type emailForwardResource struct {
	data *ProviderData
}

type emailForwardModel struct {
	ID             types.String `tfsdk:"id"`
	EmailAccountID types.String `tfsdk:"email_account_id"`
	Destination    types.String `tfsdk:"destination"`
	KeepCopy       types.Bool   `tfsdk:"keep_copy"`
	Status         types.String `tfsdk:"status"`
}

type emailForwardAPI struct {
	ID             string `json:"id"`
	EmailAccountID string `json:"email_account_id"`
	Destination    string `json:"destination"`
	KeepCopy       bool   `json:"keep_copy"`
	Status         string `json:"status"`
}

func NewEmailForward() resource.Resource {
	return &emailForwardResource{}
}

func (r *emailForwardResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_email_forward"
}

func (r *emailForwardResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an email forward rule.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Forward ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"email_account_id": schema.StringAttribute{
				Required: true, Description: "Parent email account ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"destination": schema.StringAttribute{
				Required: true, Description: "Destination email address.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"keep_copy": schema.BoolAttribute{
				Optional: true, Computed: true, Description: "Keep a copy in the original mailbox.",
				Default: booldefault.StaticBool(true),
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *emailForwardResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *emailForwardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan emailForwardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Post[emailForwardAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/email/%s/forwards", plan.EmailAccountID.ValueString()), map[string]any{
		"destination": plan.Destination.ValueString(),
		"keep_copy":   plan.KeepCopy.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Create Email Forward Failed", err.Error())
		return
	}

	mapEmailForward(result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailForwardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state emailForwardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	forwards, err := hosting.List[emailForwardAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/email/%s/forwards", state.EmailAccountID.ValueString()))
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Email Forward Failed", err.Error())
		return
	}

	var found *emailForwardAPI
	for i := range forwards {
		if forwards[i].ID == state.ID.ValueString() {
			found = &forwards[i]
			break
		}
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	mapEmailForward(found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emailForwardResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "Email forwards cannot be updated. Delete and recreate instead.")
}

func (r *emailForwardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state emailForwardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, fmt.Sprintf("/api/v1/email/%s/forwards/%s", state.EmailAccountID.ValueString(), state.ID.ValueString())); err != nil {
		resp.Diagnostics.AddError("Delete Email Forward Failed", err.Error())
	}
}

func (r *emailForwardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid Import ID", "Expected format: email_account_id/forward_id")
		return
	}

	forwards, err := hosting.List[emailForwardAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/email/%s/forwards", parts[0]))
	if err != nil {
		resp.Diagnostics.AddError("Import Email Forward Failed", err.Error())
		return
	}

	var found *emailForwardAPI
	for i := range forwards {
		if forwards[i].ID == parts[1] {
			found = &forwards[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("Import Email Forward Failed", "Forward not found")
		return
	}

	var state emailForwardModel
	mapEmailForward(found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapEmailForward(api *emailForwardAPI, state *emailForwardModel) {
	state.ID = types.StringValue(api.ID)
	state.EmailAccountID = types.StringValue(api.EmailAccountID)
	state.Destination = types.StringValue(api.Destination)
	state.KeepCopy = types.BoolValue(api.KeepCopy)
	state.Status = types.StringValue(api.Status)
}
