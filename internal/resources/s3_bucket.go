package resources

import (
	"context"
	"fmt"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &s3BucketResource{}
	_ resource.ResourceWithImportState = &s3BucketResource{}
)

type s3BucketResource struct {
	data *ProviderData
}

type s3BucketModel struct {
	ID         types.String `tfsdk:"id"`
	CustomerID types.String `tfsdk:"customer_id"`
	TenantID   types.String `tfsdk:"tenant_id"`
	Public     types.Bool   `tfsdk:"public"`
	QuotaBytes types.Int64  `tfsdk:"quota_bytes"`
	Status     types.String `tfsdk:"status"`
}

type s3BucketAPI struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	Public     bool   `json:"public"`
	QuotaBytes int64  `json:"quota_bytes"`
	Status     string `json:"status"`
}

func NewS3Bucket() resource.Resource {
	return &s3BucketResource{}
}

func (r *s3BucketResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_bucket"
}

func (r *s3BucketResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an S3 storage bucket.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Bucket ID.",
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
			"public": schema.BoolAttribute{
				Optional: true, Computed: true, Description: "Whether the bucket is publicly accessible.",
				Default: booldefault.StaticBool(false),
			},
			"quota_bytes": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "Storage quota in bytes (0 = unlimited).",
				Default: int64default.StaticInt64(0),
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *s3BucketResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *s3BucketResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan s3BucketModel
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
		"tenant_id":   plan.TenantID.ValueString(),
		"public":      plan.Public.ValueBool(),
		"quota_bytes": plan.QuotaBytes.ValueInt64(),
	}

	result, err := hosting.Post[s3BucketAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/s3-buckets", customerID), body)
	if err != nil {
		resp.Diagnostics.AddError("Create S3 Bucket Failed", err.Error())
		return
	}

	final, err := waitForActive[s3BucketAPI](ctx, r.data.Client, "/api/v1/s3-buckets/"+result.ID, func(b *s3BucketAPI) string { return b.Status })
	if err != nil {
		resp.Diagnostics.AddWarning("S3 Bucket Not Yet Active", err.Error())
		final = result
	}

	mapS3Bucket(final, &plan, customerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *s3BucketResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state s3BucketModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := hosting.Get[s3BucketAPI](ctx, r.data.Client, "/api/v1/s3-buckets/"+state.ID.ValueString())
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read S3 Bucket Failed", err.Error())
		return
	}

	mapS3Bucket(result, &state, state.CustomerID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *s3BucketResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan s3BucketModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state s3BucketModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"public":      plan.Public.ValueBool(),
		"quota_bytes": plan.QuotaBytes.ValueInt64(),
	}

	result, err := hosting.Put[s3BucketAPI](ctx, r.data.Client, "/api/v1/s3-buckets/"+state.ID.ValueString(), body)
	if err != nil {
		resp.Diagnostics.AddError("Update S3 Bucket Failed", err.Error())
		return
	}

	mapS3Bucket(result, &plan, state.CustomerID.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *s3BucketResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state s3BucketModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/s3-buckets/"+state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete S3 Bucket Failed", err.Error())
	}
}

func (r *s3BucketResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[s3BucketAPI](ctx, r.data.Client, "/api/v1/s3-buckets/"+req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import S3 Bucket Failed", err.Error())
		return
	}
	var state s3BucketModel
	mapS3Bucket(result, &state, r.data.CustomerID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapS3Bucket(api *s3BucketAPI, state *s3BucketModel, customerID string) {
	state.ID = types.StringValue(api.ID)
	state.TenantID = types.StringValue(api.TenantID)
	state.Public = types.BoolValue(api.Public)
	state.QuotaBytes = types.Int64Value(api.QuotaBytes)
	state.Status = types.StringValue(api.Status)
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}
}
