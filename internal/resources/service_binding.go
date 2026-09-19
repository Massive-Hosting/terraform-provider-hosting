package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/massive-hosting/go-hosting"
)

/*
One website declaring that it uses another.

The binding is what is true; the environment variables are derived from it
every time the consumer is provisioned. That is why this resource has no
`value` and why changing the service it points at replaces it: there is
nothing here to update in place, and a binding that quietly re-pointed would
rewrite an environment without a plan saying so.
*/

var (
	_ resource.Resource                = &serviceBindingResource{}
	_ resource.ResourceWithImportState = &serviceBindingResource{}
)

type serviceBindingResource struct {
	data *ProviderData
}

type serviceBindingModel struct {
	ID         types.String `tfsdk:"id"`
	WebsiteID  types.String `tfsdk:"website_id"`
	TargetID   types.String `tfsdk:"target_id"`
	EnvPrefix  types.String `tfsdk:"env_prefix"`
	TargetName types.String `tfsdk:"target_name"`
}

type serviceBindingAPI struct {
	ID         string `json:"id"`
	WebsiteID  string `json:"website_id"`
	TargetKind string `json:"target_kind"`
	TargetID   string `json:"target_id"`
	EnvPrefix  string `json:"env_prefix"`
	TargetName string `json:"target_name"`
}

func NewServiceBinding() resource.Resource {
	return &serviceBindingResource{}
}

func (r *serviceBindingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_binding"
}

func (r *serviceBindingResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Connects a website to a container service in the same plan. The service's " +
			"address arrives in the website's environment as PREFIX_HOST, PREFIX_PORT and " +
			"PREFIX_URL, derived at provision time rather than stored — so a service that is " +
			"renamed or re-addressed stays reachable without a change here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Binding ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"website_id": schema.StringAttribute{
				Required:      true,
				Description:   "The consumer: the website whose environment gains the variables.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"target_id": schema.StringAttribute{
				Required: true,
				Description: "The service being used — a website with runtime \"container\", in the " +
					"same plan, since a binding produces an address on a per-tenant loopback.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"env_prefix": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "Prefix for the generated variables: MEILI gives MEILI_HOST, MEILI_PORT " +
					"and MEILI_URL. Defaults to the service's own name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"target_name": schema.StringAttribute{
				Computed:    true,
				Description: "The service's internal hostname — the value of PREFIX_HOST.",
			},
		},
	}
}

func (r *serviceBindingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *serviceBindingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceBindingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{"target_id": plan.TargetID.ValueString()}
	// Sent only when set: an empty prefix means "use the service's name", and
	// the platform is the one that knows it.
	if !plan.EnvPrefix.IsNull() && !plan.EnvPrefix.IsUnknown() && plan.EnvPrefix.ValueString() != "" {
		body["env_prefix"] = plan.EnvPrefix.ValueString()
	}

	created, err := hosting.Post[serviceBindingAPI](ctx, r.data.Client,
		fmt.Sprintf("/api/v1/websites/%s/bindings", plan.WebsiteID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Create Service Binding Failed", err.Error())
		return
	}

	// The create response does not resolve the target's name, so state takes
	// it from a read rather than carrying an unknown into the next plan.
	r.mapToState(&plan, created)
	if b := r.find(ctx, plan.WebsiteID.ValueString(), created.ID); b != nil {
		r.mapToState(&plan, b)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceBindingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bindings, err := hosting.List[serviceBindingAPI](ctx, r.data.Client,
		fmt.Sprintf("/api/v1/websites/%s/bindings", state.WebsiteID.ValueString()))
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read Service Binding Failed", err.Error())
		return
	}

	for _, b := range bindings {
		if b.ID == state.ID.ValueString() {
			r.mapToState(&state, &b)
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}
	// Deleting the service takes its bindings with it, so a binding that is
	// gone is an ordinary outcome rather than an error.
	resp.State.RemoveResource(ctx)
}

// Update cannot be reached: every settable attribute requires replacement.
func (r *serviceBindingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceBindingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *serviceBindingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serviceBindingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.data.Client.Delete(ctx, fmt.Sprintf("/api/v1/websites/%s/bindings/%s",
		state.WebsiteID.ValueString(), state.ID.ValueString()))
	if err != nil && !hosting.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete Service Binding Failed", err.Error())
	}
}

// ImportState takes "<website_id>/<binding_id>": a binding is only addressable
// through the website that holds it.
func (r *serviceBindingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	websiteID, bindingID, ok := strings.Cut(req.ID, "/")
	if !ok || websiteID == "" || bindingID == "" {
		resp.Diagnostics.AddError("Invalid Import ID", "Expected \"<website_id>/<binding_id>\".")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("website_id"), websiteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), bindingID)...)
}

// find returns one of a website's bindings, or nil if the lookup fails — the
// caller is filling in detail, not deciding whether the binding exists.
func (r *serviceBindingResource) find(ctx context.Context, websiteID, bindingID string) *serviceBindingAPI {
	bindings, err := hosting.List[serviceBindingAPI](ctx, r.data.Client,
		fmt.Sprintf("/api/v1/websites/%s/bindings", websiteID))
	if err != nil {
		return nil
	}
	for _, b := range bindings {
		if b.ID == bindingID {
			return &b
		}
	}
	return nil
}

func (r *serviceBindingResource) mapToState(m *serviceBindingModel, api *serviceBindingAPI) {
	m.ID = types.StringValue(api.ID)
	m.WebsiteID = types.StringValue(api.WebsiteID)
	m.TargetID = types.StringValue(api.TargetID)
	m.EnvPrefix = types.StringValue(api.EnvPrefix)
	m.TargetName = types.StringValue(api.TargetName)
}
