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
	_ resource.Resource                = &emailAccountResource{}
	_ resource.ResourceWithImportState = &emailAccountResource{}
)

type emailAccountResource struct {
	data *ProviderData
}

type emailAccountModel struct {
	ID          types.String `tfsdk:"id"`
	FQDNID      types.String `tfsdk:"fqdn_id"`
	Address     types.String `tfsdk:"address"`
	DisplayName types.String `tfsdk:"display_name"`
	Password    types.String `tfsdk:"password"`
	QuotaBytes  types.Int64  `tfsdk:"quota_bytes"`
	Status      types.String `tfsdk:"status"`
}

type emailAccountAPI struct {
	ID          string `json:"id"`
	FQDNId      string `json:"fqdn_id"`
	Address     string `json:"address"`
	DisplayName string `json:"display_name"`
	QuotaBytes  int64  `json:"quota_bytes"`
	Status      string `json:"status"`
}

func NewEmailAccount() resource.Resource {
	return &emailAccountResource{}
}

func (r *emailAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_email_account"
}

func (r *emailAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an email account.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Email account ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"fqdn_id": schema.StringAttribute{
				Required: true, Description: "Hostname/FQDN ID this email account belongs to.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"address": schema.StringAttribute{
				Required: true, Description: "Email address (local part only, domain from FQDN).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"display_name": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Display name.",
			},
			"password": schema.StringAttribute{
				Required: true, Sensitive: true, Description: "Account password.",
			},
			"quota_bytes": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Mailbox quota in bytes.",
				Default: int64default.StaticInt64(1073741824), // 1GB
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *emailAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *emailAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan emailAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"address":      plan.Address.ValueString(),
		"password":     plan.Password.ValueString(),
		"quota_bytes":  plan.QuotaBytes.ValueInt64(),
	}
	if !plan.DisplayName.IsNull() && !plan.DisplayName.IsUnknown() {
		body["display_name"] = plan.DisplayName.ValueString()
	}

	result, err := hosting.Post[emailAccountAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/hostnames/%s/email-accounts", plan.FQDNID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Email Account Failed", err.Error())
		return
	}

	final, err := waitForActive[emailAccountAPI](ctx, r.data.Client, "/api/v1/email/"+result.ID, func(e *emailAccountAPI) string { return e.Status })
	if err != nil {
		resp.Diagnostics.AddWarning("Email Account Not Yet Active", err.Error())
		final = result
	}

	mapEmailAccount(final, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state emailAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[emailAccountAPI](ctx, r.data.Client, "/api/v1/email/"+state.ID.ValueString())
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Email Account Failed", err.Error())
		return
	}

	pw := state.Password
	mapEmailAccount(result, &state)
	state.Password = pw // Preserve password from state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emailAccountResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "Email accounts cannot be updated. Delete and recreate instead.")
}

func (r *emailAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state emailAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/email/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete Email Account Failed", err.Error())
	}
}

func (r *emailAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[emailAccountAPI](ctx, r.data.Client, "/api/v1/email/"+req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import Email Account Failed", err.Error())
		return
	}
	var state emailAccountModel
	mapEmailAccount(result, &state)
	state.Password = types.StringValue("") // Not available on import
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapEmailAccount(api *emailAccountAPI, state *emailAccountModel) {
	state.ID = types.StringValue(api.ID)
	state.FQDNID = types.StringValue(api.FQDNId)
	state.Address = types.StringValue(api.Address)
	state.DisplayName = types.StringValue(api.DisplayName)
	state.QuotaBytes = types.Int64Value(api.QuotaBytes)
	state.Status = types.StringValue(api.Status)
}
