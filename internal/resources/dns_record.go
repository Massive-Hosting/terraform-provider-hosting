package resources

import (
	"context"
	"fmt"
	"strings"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &dnsRecordResource{}
	_ resource.ResourceWithImportState = &dnsRecordResource{}
)

type dnsRecordResource struct {
	data *ProviderData
}

type dnsRecordModel struct {
	ID       types.String `tfsdk:"id"`
	ZoneID   types.String `tfsdk:"zone_id"`
	Type     types.String `tfsdk:"type"`
	Name     types.String `tfsdk:"name"`
	Content  types.String `tfsdk:"content"`
	TTL      types.Int64  `tfsdk:"ttl"`
	Priority types.Int64  `tfsdk:"priority"`
	Status   types.String `tfsdk:"status"`
}

type dnsRecordAPI struct {
	ID       string `json:"id"`
	ZoneID   string `json:"zone_id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int64  `json:"ttl"`
	Priority *int64 `json:"priority"`
	Status   string `json:"status"`
}

func NewDNSRecord() resource.Resource {
	return &dnsRecordResource{}
}

func (r *dnsRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *dnsRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a DNS record within a zone.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Record ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"zone_id": schema.StringAttribute{
				Required: true, Description: "DNS zone ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"type": schema.StringAttribute{
				Required: true, Description: "Record type (A, AAAA, CNAME, MX, TXT, SRV, etc.).",
			},
			"name": schema.StringAttribute{
				Required: true, Description: "Record name (e.g. @, www, mail).",
			},
			"content": schema.StringAttribute{
				Required: true, Description: "Record content (e.g. IP address, hostname).",
			},
			"ttl": schema.Int64Attribute{
				Optional: true, Computed: true, Description: "TTL in seconds.",
				Default: int64default.StaticInt64(3600),
			},
			"priority": schema.Int64Attribute{
				Optional: true, Description: "Priority (for MX, SRV records).",
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status.",
			},
		},
	}
}

func (r *dnsRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *dnsRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"type":    plan.Type.ValueString(),
		"name":    plan.Name.ValueString(),
		"content": plan.Content.ValueString(),
		"ttl":     plan.TTL.ValueInt64(),
	}
	if !plan.Priority.IsNull() && !plan.Priority.IsUnknown() {
		body["priority"] = plan.Priority.ValueInt64()
	}

	result, err := hosting.Post[dnsRecordAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/dns-zones/%s/records", plan.ZoneID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Create DNS Record Failed", err.Error())
		return
	}

	mapDNSRecord(result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	records, err := hosting.List[dnsRecordAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/dns-zones/%s/records", state.ZoneID.ValueString()))
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read DNS Record Failed", err.Error())
		return
	}

	var found *dnsRecordAPI
	for i := range records {
		if records[i].ID == state.ID.ValueString() {
			found = &records[i]
			break
		}
	}
	if found == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	mapDNSRecord(found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dnsRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := map[string]any{
		"type":    plan.Type.ValueString(),
		"name":    plan.Name.ValueString(),
		"content": plan.Content.ValueString(),
		"ttl":     plan.TTL.ValueInt64(),
	}
	if !plan.Priority.IsNull() && !plan.Priority.IsUnknown() {
		body["priority"] = plan.Priority.ValueInt64()
	}

	result, err := hosting.Put[dnsRecordAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/dns-zones/%s/records/%s", state.ZoneID.ValueString(), state.ID.ValueString()), body)
	if err != nil {
		resp.Diagnostics.AddError("Update DNS Record Failed", err.Error())
		return
	}

	mapDNSRecord(result, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.data.Client.Delete(ctx, fmt.Sprintf("/api/v1/dns-zones/%s/records/%s", state.ZoneID.ValueString(), state.ID.ValueString())); err != nil {
		resp.Diagnostics.AddError("Delete DNS Record Failed", err.Error())
	}
}

func (r *dnsRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid Import ID", "Expected format: zone_id/record_id")
		return
	}

	records, err := hosting.List[dnsRecordAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/dns-zones/%s/records", parts[0]))
	if err != nil {
		resp.Diagnostics.AddError("Import DNS Record Failed", err.Error())
		return
	}

	var found *dnsRecordAPI
	for i := range records {
		if records[i].ID == parts[1] {
			found = &records[i]
			break
		}
	}
	if found == nil {
		resp.Diagnostics.AddError("Import DNS Record Failed", "Record not found")
		return
	}

	var state dnsRecordModel
	mapDNSRecord(found, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func mapDNSRecord(api *dnsRecordAPI, state *dnsRecordModel) {
	state.ID = types.StringValue(api.ID)
	state.ZoneID = types.StringValue(api.ZoneID)
	state.Type = types.StringValue(api.Type)
	state.Name = types.StringValue(api.Name)
	state.Content = types.StringValue(api.Content)
	state.TTL = types.Int64Value(api.TTL)
	state.Status = types.StringValue(api.Status)
	if api.Priority != nil {
		state.Priority = types.Int64Value(*api.Priority)
	} else {
		state.Priority = types.Int64Null()
	}
}
