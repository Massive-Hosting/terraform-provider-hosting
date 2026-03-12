package resources

import (
	"context"
	"fmt"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &sshKeyResource{}
	_ resource.ResourceWithImportState = &sshKeyResource{}
)

type sshKeyResource struct {
	data *ProviderData
}

type sshKeyModel struct {
	ID                  types.String `tfsdk:"id"`
	CustomerID          types.String `tfsdk:"customer_id"`
	TenantID            types.String `tfsdk:"tenant_id"`
	Name                types.String `tfsdk:"name"`
	PublicKey           types.String `tfsdk:"public_key"`
	Generate            types.Bool   `tfsdk:"generate"`
	GeneratedPrivateKey types.String `tfsdk:"generated_private_key"`
	Fingerprint         types.String `tfsdk:"fingerprint"`
	ExpiresAt           types.String `tfsdk:"expires_at"`
	Status              types.String `tfsdk:"status"`
}

type sshKeyAPI struct {
	ID          string  `json:"id"`
	TenantID    string  `json:"tenant_id"`
	Name        string  `json:"name"`
	PublicKey   string  `json:"public_key"`
	Fingerprint string  `json:"fingerprint"`
	ExpiresAt   *string `json:"expires_at"`
	Status      string  `json:"status"`
}

type sshKeyCreateResultAPI struct {
	Key        sshKeyAPI `json:"key"`
	PrivateKey string    `json:"private_key"`
}

func NewSSHKey() resource.Resource {
	return &sshKeyResource{}
}

func (r *sshKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_key"
}

func (r *sshKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an SSH public key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "SSH key ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"customer_id": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Customer ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"tenant_id": schema.StringAttribute{
				Required: true, Description: "Tenant ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required: true, Description: "Key name.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"public_key": schema.StringAttribute{
				Optional: true, Computed: true, Description: "SSH public key content. Required when generate is false.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"generate": schema.BoolAttribute{
				Optional: true, Description: "When true, generate an Ed25519 keypair server-side instead of providing a public_key.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"generated_private_key": schema.StringAttribute{
				Computed: true, Sensitive: true,
				Description: "The generated private key PEM (only set when generate=true, only available after create).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"fingerprint": schema.StringAttribute{
				Computed: true, Description: "Key fingerprint.",
			},
			"expires_at": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Expiry timestamp (RFC 3339). Null means no expiry.",
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *sshKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *sshKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sshKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customerID := resolveCustomerID(plan.CustomerID, r.data)
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "Set customer_id on the resource or in the provider config.")
		return
	}

	generate := !plan.Generate.IsNull() && plan.Generate.ValueBool()

	if generate {
		body := map[string]any{
			"tenant_id": plan.TenantID.ValueString(),
			"name":      plan.Name.ValueString(),
		}
		if !plan.ExpiresAt.IsNull() && !plan.ExpiresAt.IsUnknown() {
			body["expires_at"] = plan.ExpiresAt.ValueString()
		}

		result, err := hosting.Post[sshKeyCreateResultAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/ssh-keypairs", customerID), body)
		if err != nil {
			resp.Diagnostics.AddError("Generate SSH Keypair Failed", err.Error())
			return
		}

		mapSSHKey(&result.Key, &plan, customerID)
		plan.GeneratedPrivateKey = types.StringValue(result.PrivateKey)
	} else {
		if plan.PublicKey.IsNull() || plan.PublicKey.IsUnknown() || plan.PublicKey.ValueString() == "" {
			resp.Diagnostics.AddError("Missing public_key", "public_key is required when generate is not true.")
			return
		}

		body := map[string]any{
			"tenant_id":  plan.TenantID.ValueString(),
			"name":       plan.Name.ValueString(),
			"public_key": plan.PublicKey.ValueString(),
		}
		if !plan.ExpiresAt.IsNull() && !plan.ExpiresAt.IsUnknown() {
			body["expires_at"] = plan.ExpiresAt.ValueString()
		}

		result, err := hosting.Post[sshKeyAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/ssh-keys", customerID), body)
		if err != nil {
			resp.Diagnostics.AddError("Create SSH Key Failed", err.Error())
			return
		}

		mapSSHKey(result, &plan, customerID)
		plan.GeneratedPrivateKey = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sshKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sshKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// No single GET endpoint at CP level — list and filter
	customerID := state.CustomerID.ValueString()
	if customerID == "" {
		customerID = r.data.CustomerID
	}
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "customer_id is required for reading SSH keys.")
		return
	}

	keys, err := hosting.List[sshKeyAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/ssh-keys", customerID))
	if err != nil {
		resp.Diagnostics.AddError("Read SSH Key Failed", err.Error())
		return
	}

	var found *sshKeyAPI
	for i := range keys {
		if keys[i].ID == state.ID.ValueString() {
			found = &keys[i]
			break
		}
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	mapSSHKey(found, &state, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *sshKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sshKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{}
	if !plan.ExpiresAt.IsNull() && !plan.ExpiresAt.IsUnknown() {
		body["expires_at"] = plan.ExpiresAt.ValueString()
	} else {
		body["expires_at"] = nil
	}

	result, err := hosting.Put[sshKeyAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/ssh-keys/%s", plan.ID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Update SSH Key Failed", err.Error())
		return
	}

	customerID := resolveCustomerID(plan.CustomerID, r.data)
	mapSSHKey(result, &plan, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *sshKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sshKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/ssh-keys/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete SSH Key Failed", err.Error())
	}
}

func (r *sshKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	customerID := r.data.CustomerID
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "customer_id must be set in provider config to import SSH keys.")
		return
	}

	keys, err := hosting.List[sshKeyAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/ssh-keys", customerID))
	if err != nil {
		resp.Diagnostics.AddError("Import SSH Key Failed", err.Error())
		return
	}

	var found *sshKeyAPI
	for i := range keys {
		if keys[i].ID == req.ID {
			found = &keys[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("Import SSH Key Failed", "Key not found")
		return
	}

	var state sshKeyModel
	mapSSHKey(found, &state, customerID)
	state.Generate = types.BoolNull()
	state.GeneratedPrivateKey = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapSSHKey(api *sshKeyAPI, state *sshKeyModel, customerID string) {
	state.ID = types.StringValue(api.ID)
	state.TenantID = types.StringValue(api.TenantID)
	state.Name = types.StringValue(api.Name)
	state.PublicKey = types.StringValue(api.PublicKey)
	state.Fingerprint = types.StringValue(api.Fingerprint)
	state.Status = types.StringValue(api.Status)
	if api.ExpiresAt != nil {
		state.ExpiresAt = types.StringValue(*api.ExpiresAt)
	} else {
		state.ExpiresAt = types.StringNull()
	}
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}
}
