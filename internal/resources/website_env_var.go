package resources

import (
	"context"
	"fmt"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &websiteEnvVarsResource{}

type websiteEnvVarsResource struct {
	data *ProviderData
}

type websiteEnvVarsModel struct {
	ID         types.String `tfsdk:"id"`
	CustomerID types.String `tfsdk:"customer_id"`
	WebsiteID   types.String `tfsdk:"website_id"`
	Vars       types.Map    `tfsdk:"vars"`
	SecretVars types.Map    `tfsdk:"secret_vars"`
}

type envVarAPI struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	IsSecret bool   `json:"is_secret"`
}

type setEnvVarEntry struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

func NewWebsiteEnvVars() resource.Resource {
	return &websiteEnvVarsResource{}
}

func (r *websiteEnvVarsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_website_env_vars"
}

func (r *websiteEnvVarsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages environment variables for a website. This resource manages ALL env vars for the website — any vars not included will be removed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Resource ID (same as website_id).",
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
			"vars": schema.MapAttribute{
				Optional: true, Description: "Non-secret environment variables (name → value).",
				ElementType: types.StringType,
			},
			"secret_vars": schema.MapAttribute{
				Optional: true, Sensitive: true,
				Description: "Secret environment variables (name → value). Values are encrypted server-side and cannot be read back.",
				ElementType: types.StringType,
			},
		},
	}
}

func (r *websiteEnvVarsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *websiteEnvVarsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan websiteEnvVarsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customerID := resolveCustomerID(plan.CustomerID, r.data)
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "Set customer_id on the resource or in the provider config.")
		return
	}

	entries, diags := r.buildEntries(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setEnvVars(ctx, plan.WebsiteID.ValueString(), entries); err != nil {
		resp.Diagnostics.AddError("Set Env Vars Failed", err.Error())
		return
	}

	plan.ID = plan.WebsiteID
	plan.CustomerID = types.StringValue(customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *websiteEnvVarsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state websiteEnvVarsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := hosting.List[envVarAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/websites/%s/env-vars", state.WebsiteID.ValueString()))
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Env Vars Failed", err.Error())
		return
	}

	// Split into vars and secret_vars based on is_secret flag.
	// Secret values come back as "***" so we keep state values.
	plainVars := map[string]string{}
	for _, item := range items {
		if !item.IsSecret {
			plainVars[item.Name] = item.Value
		}
	}

	// Build set of secret var names from API.
	apiSecretNames := map[string]bool{}
	for _, item := range items {
		if item.IsSecret {
			apiSecretNames[item.Name] = true
		}
	}

	// Update plain vars from API.
	if len(plainVars) > 0 {
		varsMap, diags := types.MapValueFrom(ctx, types.StringType, plainVars)
		resp.Diagnostics.Append(diags...)
		state.Vars = varsMap
	} else {
		state.Vars = types.MapNull(types.StringType)
	}

	// For secret vars, keep state values but remove any that no longer exist in the API.
	if !state.SecretVars.IsNull() && !state.SecretVars.IsUnknown() {
		existingSecrets := map[string]string{}
		resp.Diagnostics.Append(state.SecretVars.ElementsAs(ctx, &existingSecrets, false)...)
		filtered := map[string]string{}
		for k, v := range existingSecrets {
			if apiSecretNames[k] {
				filtered[k] = v
			}
		}
		if len(filtered) > 0 {
			secretMap, diags := types.MapValueFrom(ctx, types.StringType, filtered)
			resp.Diagnostics.Append(diags...)
			state.SecretVars = secretMap
		} else {
			state.SecretVars = types.MapNull(types.StringType)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *websiteEnvVarsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan websiteEnvVarsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state websiteEnvVarsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	entries, diags := r.buildEntries(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setEnvVars(ctx, plan.WebsiteID.ValueString(), entries); err != nil {
		resp.Diagnostics.AddError("Set Env Vars Failed", err.Error())
		return
	}

	plan.ID = state.ID
	plan.CustomerID = state.CustomerID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *websiteEnvVarsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state websiteEnvVarsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set empty vars to remove all.
	if err := r.setEnvVars(ctx, state.WebsiteID.ValueString(), []setEnvVarEntry{}); err != nil {
		resp.Diagnostics.AddError("Delete Env Vars Failed", err.Error())
	}
}

func (r *websiteEnvVarsResource) buildEntries(ctx context.Context, plan websiteEnvVarsModel) ([]setEnvVarEntry, diag.Diagnostics) {
	var entries []setEnvVarEntry
	var diags diag.Diagnostics

	if !plan.Vars.IsNull() && !plan.Vars.IsUnknown() {
		plainVars := map[string]string{}
		diags.Append(plan.Vars.ElementsAs(ctx, &plainVars, false)...)
		for k, v := range plainVars {
			entries = append(entries, setEnvVarEntry{Name: k, Value: v, Secret: false})
		}
	}

	if !plan.SecretVars.IsNull() && !plan.SecretVars.IsUnknown() {
		secretVars := map[string]string{}
		diags.Append(plan.SecretVars.ElementsAs(ctx, &secretVars, false)...)
		for k, v := range secretVars {
			entries = append(entries, setEnvVarEntry{Name: k, Value: v, Secret: true})
		}
	}

	return entries, diags
}

func (r *websiteEnvVarsResource) setEnvVars(ctx context.Context, websiteID string, entries []setEnvVarEntry) error {
	_, err := r.data.Client.Do(ctx, "PUT", fmt.Sprintf("/api/v1/websites/%s/env-vars", websiteID), map[string]any{
		"vars": entries,
	})
	if err != nil {
		return err
	}
	return nil
}
