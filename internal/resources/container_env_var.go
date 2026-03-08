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

var _ resource.Resource = &containerEnvVarsResource{}

type containerEnvVarsResource struct {
	data *ProviderData
}

type containerEnvVarsModel struct {
	ID          types.String `tfsdk:"id"`
	CustomerID  types.String `tfsdk:"customer_id"`
	ContainerID types.String `tfsdk:"container_id"`
	Vars        types.Map    `tfsdk:"vars"`
	SecretVars  types.Map    `tfsdk:"secret_vars"`
}

func NewContainerEnvVars() resource.Resource {
	return &containerEnvVarsResource{}
}

func (r *containerEnvVarsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_container_env_vars"
}

func (r *containerEnvVarsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages environment variables for a container. This resource manages ALL env vars for the container — any vars not included will be removed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Resource ID (same as container_id).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"customer_id": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Customer ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"container_id": schema.StringAttribute{
				Required: true, Description: "Container ID.",
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

func (r *containerEnvVarsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *containerEnvVarsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan containerEnvVarsModel
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

	if err := r.setEnvVars(ctx, plan.ContainerID.ValueString(), entries); err != nil {
		resp.Diagnostics.AddError("Set Container Env Vars Failed", err.Error())
		return
	}

	plan.ID = plan.ContainerID
	plan.CustomerID = types.StringValue(customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *containerEnvVarsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state containerEnvVarsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	items, err := hosting.List[envVarAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/containers/%s/env-vars", state.ContainerID.ValueString()))
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Container Env Vars Failed", err.Error())
		return
	}

	plainVars := map[string]string{}
	apiSecretNames := map[string]bool{}
	for _, item := range items {
		if item.IsSecret {
			apiSecretNames[item.Name] = true
		} else {
			plainVars[item.Name] = item.Value
		}
	}

	if len(plainVars) > 0 {
		varsMap, diags := types.MapValueFrom(ctx, types.StringType, plainVars)
		resp.Diagnostics.Append(diags...)
		state.Vars = varsMap
	} else {
		state.Vars = types.MapNull(types.StringType)
	}

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

func (r *containerEnvVarsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan containerEnvVarsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state containerEnvVarsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	entries, diags := r.buildEntries(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setEnvVars(ctx, plan.ContainerID.ValueString(), entries); err != nil {
		resp.Diagnostics.AddError("Set Container Env Vars Failed", err.Error())
		return
	}

	plan.ID = state.ID
	plan.CustomerID = state.CustomerID
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *containerEnvVarsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state containerEnvVarsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.setEnvVars(ctx, state.ContainerID.ValueString(), []setEnvVarEntry{}); err != nil {
		resp.Diagnostics.AddError("Delete Container Env Vars Failed", err.Error())
	}
}

func (r *containerEnvVarsResource) buildEntries(ctx context.Context, plan containerEnvVarsModel) ([]setEnvVarEntry, diag.Diagnostics) {
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

func (r *containerEnvVarsResource) setEnvVars(ctx context.Context, containerID string, entries []setEnvVarEntry) error {
	_, err := r.data.Client.Do(ctx, "PUT", fmt.Sprintf("/api/v1/containers/%s/env-vars", containerID), map[string]any{
		"vars": entries,
	})
	return err
}
