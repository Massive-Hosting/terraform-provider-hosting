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
	_ resource.Resource                = &s3AccessKeyResource{}
	_ resource.ResourceWithImportState = &s3AccessKeyResource{}
)

type s3AccessKeyResource struct {
	data *ProviderData
}

type s3AccessKeyModel struct {
	ID              types.String `tfsdk:"id"`
	S3BucketID      types.String `tfsdk:"s3_bucket_id"`
	AccessKeyID     types.String `tfsdk:"access_key_id"`
	SecretAccessKey  types.String `tfsdk:"secret_access_key"`
	Permissions     types.List   `tfsdk:"permissions"`
	Status          types.String `tfsdk:"status"`
}

type s3AccessKeyAPI struct {
	ID              string   `json:"id"`
	S3BucketID      string   `json:"s3_bucket_id"`
	AccessKeyID     string   `json:"access_key_id"`
	SecretAccessKey  string   `json:"secret_access_key,omitempty"`
	Permissions     []string `json:"permissions"`
	Status          string   `json:"status"`
}

func NewS3AccessKey() resource.Resource {
	return &s3AccessKeyResource{}
}

func (r *s3AccessKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_s3_access_key"
}

func (r *s3AccessKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an S3 access key for a bucket.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Access key resource ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"s3_bucket_id": schema.StringAttribute{
				Required: true, Description: "Parent S3 bucket ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"access_key_id": schema.StringAttribute{
				Computed: true, Description: "AWS-style access key ID.",
			},
			"secret_access_key": schema.StringAttribute{
				Computed: true, Sensitive: true,
				Description: "Secret access key. Only available on creation.",
			},
			"permissions": schema.ListAttribute{
				Optional: true, Computed: true,
				Description: "List of permissions (e.g. read, write).",
				ElementType: types.StringType,
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *s3AccessKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *s3AccessKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan s3AccessKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{}
	if !plan.Permissions.IsNull() && !plan.Permissions.IsUnknown() {
		var perms []string
		resp.Diagnostics.Append(plan.Permissions.ElementsAs(ctx, &perms, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body["permissions"] = perms
	}

	result, err := hosting.Post[s3AccessKeyAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/s3-buckets/%s/access-keys", plan.S3BucketID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Create S3 Access Key Failed", err.Error())
		return
	}

	r.mapToState(ctx, result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *s3AccessKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state s3AccessKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	keys, err := hosting.List[s3AccessKeyAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/s3-buckets/%s/access-keys", state.S3BucketID.ValueString()))
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read S3 Access Key Failed", err.Error())
		return
	}

	var found *s3AccessKeyAPI
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

	// Preserve secret from state (not returned on read)
	secretKey := state.SecretAccessKey
	r.mapToState(ctx, found, &state)
	state.SecretAccessKey = secretKey
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *s3AccessKeyResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update Not Supported", "S3 access keys cannot be updated. Delete and recreate instead.")
}

func (r *s3AccessKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state s3AccessKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, fmt.Sprintf("/api/v1/s3-buckets/%s/access-keys/%s", state.S3BucketID.ValueString(), state.ID.ValueString())); err != nil {
		resp.Diagnostics.AddError("Delete S3 Access Key Failed", err.Error())
	}
}

func (r *s3AccessKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid Import ID", "Expected format: s3_bucket_id/access_key_id")
		return
	}

	keys, err := hosting.List[s3AccessKeyAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/s3-buckets/%s/access-keys", parts[0]))
	if err != nil {
		resp.Diagnostics.AddError("Import S3 Access Key Failed", err.Error())
		return
	}

	var found *s3AccessKeyAPI
	for i := range keys {
		if keys[i].ID == parts[1] {
			found = &keys[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("Import S3 Access Key Failed", "Access key not found")
		return
	}

	var state s3AccessKeyModel
	r.mapToState(ctx, found, &state)
	// Secret not available on import
	state.SecretAccessKey = types.StringValue("")
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *s3AccessKeyResource) mapToState(ctx context.Context, api *s3AccessKeyAPI, state *s3AccessKeyModel) {
	state.ID = types.StringValue(api.ID)
	state.S3BucketID = types.StringValue(api.S3BucketID)
	state.AccessKeyID = types.StringValue(api.AccessKeyID)
	state.Status = types.StringValue(api.Status)
	if api.SecretAccessKey != "" {
		state.SecretAccessKey = types.StringValue(api.SecretAccessKey)
	}
	perms, _ := types.ListValueFrom(ctx, types.StringType, api.Permissions)
	state.Permissions = perms
}
