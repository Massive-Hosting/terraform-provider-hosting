package resources

import (
	"context"
	"fmt"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &egressRuleResource{}
	_ resource.ResourceWithImportState = &egressRuleResource{}
)

type egressRuleResource struct {
	data *ProviderData
}

type egressRuleModel struct {
	ID          types.String `tfsdk:"id"`
	CustomerID  types.String `tfsdk:"customer_id"`
	TenantID    types.String `tfsdk:"tenant_id"`
	CIDR        types.String `tfsdk:"cidr"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
}

type egressRuleAPI struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	CIDR        string `json:"cidr"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func NewEgressRule() resource.Resource {
	return &egressRuleResource{}
}

func (r *egressRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_egress_rule"
}

func (r *egressRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a tenant egress firewall rule.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Egress rule ID.",
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
			"cidr": schema.StringAttribute{
				Required: true, Description: "CIDR block to allow egress to.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Description of the rule.",
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *egressRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *egressRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan egressRuleModel
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
		"tenant_id": plan.TenantID.ValueString(),
		"cidr":      plan.CIDR.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body["description"] = plan.Description.ValueString()
	}

	result, err := hosting.Post[egressRuleAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/egress-rules", customerID), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Egress Rule Failed", err.Error())
		return
	}

	mapEgressRule(result, &plan, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *egressRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state egressRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customerID := state.CustomerID.ValueString()
	if customerID == "" {
		customerID = r.data.CustomerID
	}
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "customer_id is required for reading egress rules.")
		return
	}

	rules, err := hosting.List[egressRuleAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/egress-rules", customerID))
	if err != nil {
		resp.Diagnostics.AddError("Read Egress Rule Failed", err.Error())
		return
	}

	var found *egressRuleAPI
	for i := range rules {
		if rules[i].ID == state.ID.ValueString() {
			found = &rules[i]
			break
		}
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	mapEgressRule(found, &state, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *egressRuleResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "Egress rules cannot be updated. Delete and recreate instead.")
}

func (r *egressRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state egressRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/egress-rules/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete Egress Rule Failed", err.Error())
	}
}

func (r *egressRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	customerID := r.data.CustomerID
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "customer_id must be set in provider config to import egress rules.")
		return
	}

	rules, err := hosting.List[egressRuleAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/egress-rules", customerID))
	if err != nil {
		resp.Diagnostics.AddError("Import Egress Rule Failed", err.Error())
		return
	}

	var found *egressRuleAPI
	for i := range rules {
		if rules[i].ID == req.ID {
			found = &rules[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("Import Egress Rule Failed", "Rule not found")
		return
	}

	var state egressRuleModel
	mapEgressRule(found, &state, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapEgressRule(api *egressRuleAPI, state *egressRuleModel, customerID string) {
	state.ID = types.StringValue(api.ID)
	state.TenantID = types.StringValue(api.TenantID)
	state.CIDR = types.StringValue(api.CIDR)
	state.Description = types.StringValue(api.Description)
	state.Status = types.StringValue(api.Status)
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}
}
