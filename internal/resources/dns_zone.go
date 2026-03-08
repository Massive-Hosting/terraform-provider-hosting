package resources

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/massive-hosting/go-hosting"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &dnsZoneResource{}
	_ resource.ResourceWithImportState = &dnsZoneResource{}
)

type dnsZoneResource struct {
	data *ProviderData
}

type dnsZoneModel struct {
	ID                   types.String `tfsdk:"id"`
	CustomerID           types.String `tfsdk:"customer_id"`
	Domain               types.String `tfsdk:"domain"`
	Managed              types.Bool   `tfsdk:"managed"`
	Status               types.String `tfsdk:"status"`
	VerificationTXTName  types.String `tfsdk:"verification_txt_name"`
	VerificationTXTValue types.String `tfsdk:"verification_txt_value"`
}

type dnsZoneAPI struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Managed bool   `json:"managed"`
	Status  string `json:"status"`
}

type domainVerificationAPI struct {
	ID        string `json:"id"`
	Domain    string `json:"domain"`
	Token     string `json:"token"`
	TXTRecord string `json:"txt_record"`
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
}

// domainListItem is returned by GET /customers/{cid}/domains.
type domainListItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Managed bool   `json:"managed"`
	Status  string `json:"status"`
	Type    string `json:"type"` // "zone" or "verification"
}

func NewDNSZone() resource.Resource {
	return &dnsZoneResource{}
}

func (r *dnsZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_zone"
}

func (r *dnsZoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a DNS zone via domain verification. On creation, starts a TXT-based verification. Once the TXT record is in place, the zone is auto-created. Works like AWS ACM certificate validation — output the verification records and create them via your DNS provider.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, Description: "Zone ID (available after verification completes).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"customer_id": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Customer ID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"domain": schema.StringAttribute{
				Required: true, Description: "Domain name (e.g. example.com).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"managed": schema.BoolAttribute{
				Computed: true, Description: "Whether the zone is managed by the platform's nameservers.",
			},
			"status": schema.StringAttribute{
				Computed: true, Description: "Current status (pending_verification or active).",
			},
			"verification_txt_name": schema.StringAttribute{
				Computed: true,
				Description: "TXT record name to create at your DNS provider for verification (e.g. _hosting-verification.example.com). Empty after verification completes.",
			},
			"verification_txt_value": schema.StringAttribute{
				Computed: true,
				Description: "TXT record value for verification. Empty after verification completes.",
			},
		},
	}
}

func (r *dnsZoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *dnsZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customerID := resolveCustomerID(plan.CustomerID, r.data)
	if customerID == "" {
		resp.Diagnostics.AddError("Missing customer_id", "Set customer_id on the resource or in the provider config.")
		return
	}

	// Check if zone already exists (idempotent).
	existing, err := r.findZoneByDomain(ctx, customerID, plan.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Check Existing Zone Failed", err.Error())
		return
	}
	if existing != nil {
		mapDNSZone2(existing, &plan, customerID)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	// Start domain verification.
	dv, err := hosting.Post[domainVerificationAPI](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/domains/verify", customerID), map[string]any{
		"domain": plan.Domain.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Create Domain Verification Failed", err.Error())
		return
	}

	// Set initial state with verification info so the user can create the TXT record.
	plan.CustomerID = types.StringValue(customerID)
	plan.Status = types.StringValue("pending_verification")
	plan.Managed = types.BoolValue(false)
	plan.VerificationTXTName = types.StringValue(dv.TXTRecord)
	plan.VerificationTXTValue = types.StringValue(dv.Token)
	// Use verification ID as temporary ID until zone is created.
	plan.ID = types.StringValue("verification:" + dv.ID)

	tflog.Info(ctx, "Domain verification started — add TXT record to complete", map[string]any{
		"domain":    plan.Domain.ValueString(),
		"txt_name":  dv.TXTRecord,
		"txt_value": dv.Token,
	})

	// Poll for verification. Signal check-now to speed things up.
	deadline := time.Now().Add(10 * time.Minute)
	interval := 10 * time.Second

	for {
		// Trigger immediate check.
		_, _ = r.data.Client.Do(ctx, "POST", fmt.Sprintf("/api/v1/domain-verifications/%s/check-now", dv.ID), nil)

		select {
		case <-ctx.Done():
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		case <-time.After(interval):
		}

		// Check if zone now exists (verification auto-creates zone then deletes itself).
		zone, err := r.findZoneByDomain(ctx, customerID, plan.Domain.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Check Zone Status Failed", err.Error())
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		}
		if zone != nil {
			mapDNSZone2(zone, &plan, customerID)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		}

		if time.Now().After(deadline) {
			resp.Diagnostics.AddWarning("Verification Pending",
				fmt.Sprintf("Domain verification is still pending. Add a TXT record:\n  Name:  %s\n  Value: %s\nRun 'terraform apply' again to resume polling.", dv.TXTRecord, dv.Token))
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		}
	}
}

func (r *dnsZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	// If still a pending verification, check if zone has appeared.
	if strings.HasPrefix(id, "verification:") {
		customerID := state.CustomerID.ValueString()
		if customerID == "" {
			customerID = r.data.CustomerID
		}
		zone, err := r.findZoneByDomain(ctx, customerID, state.Domain.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Read DNS Zone Failed", err.Error())
			return
		}
		if zone != nil {
			mapDNSZone2(zone, &state, customerID)
		}
		// Keep current state either way (zone found → updated, not found → still pending).
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	result, err := hosting.Get[dnsZoneAPI](ctx, r.data.Client, "/api/v1/dns-zones/"+id)
	if err != nil {
		if hosting.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read DNS Zone Failed", err.Error())
		return
	}

	state.Domain = types.StringValue(result.Name)
	state.Managed = types.BoolValue(result.Managed)
	state.Status = types.StringValue(result.Status)
	state.VerificationTXTName = types.StringValue("")
	state.VerificationTXTValue = types.StringValue("")
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dnsZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dnsZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state dnsZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	// If still a pending verification, check if zone appeared.
	if strings.HasPrefix(id, "verification:") {
		customerID := state.CustomerID.ValueString()
		zone, err := r.findZoneByDomain(ctx, customerID, state.Domain.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Update DNS Zone Failed", err.Error())
			return
		}
		if zone != nil {
			mapDNSZone2(zone, &plan, customerID)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
			return
		}
		resp.Diagnostics.AddError("Zone Not Ready", "Domain verification has not completed yet. Ensure the TXT record is in place and run apply again.")
		return
	}

	body := map[string]any{
		"managed": plan.Managed.ValueBool(),
	}

	result, err := hosting.Put[dnsZoneAPI](ctx, r.data.Client, "/api/v1/dns-zones/"+id, body)
	if err != nil {
		resp.Diagnostics.AddError("Update DNS Zone Failed", err.Error())
		return
	}

	plan.ID = state.ID
	plan.Domain = types.StringValue(result.Name)
	plan.Managed = types.BoolValue(result.Managed)
	plan.Status = types.StringValue(result.Status)
	plan.CustomerID = state.CustomerID
	plan.VerificationTXTName = types.StringValue("")
	plan.VerificationTXTValue = types.StringValue("")
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()

	// If still a pending verification, cancel it.
	if strings.HasPrefix(id, "verification:") {
		verificationID := id[len("verification:"):]
		if err := r.data.Client.Delete(ctx, "/api/v1/domain-verifications/"+verificationID); err != nil {
			if !hosting.IsNotFound(err) {
				resp.Diagnostics.AddError("Cancel Verification Failed", err.Error())
			}
		}
		return
	}

	if err := r.data.Client.Delete(ctx, "/api/v1/dns-zones/"+id); err != nil {
		resp.Diagnostics.AddError("Delete DNS Zone Failed", err.Error())
	}
}

func (r *dnsZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	result, err := hosting.Get[dnsZoneAPI](ctx, r.data.Client, "/api/v1/dns-zones/"+req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import DNS Zone Failed", err.Error())
		return
	}
	var state dnsZoneModel
	state.ID = types.StringValue(result.ID)
	state.Domain = types.StringValue(result.Name)
	state.Managed = types.BoolValue(result.Managed)
	state.Status = types.StringValue(result.Status)
	state.VerificationTXTName = types.StringValue("")
	state.VerificationTXTValue = types.StringValue("")
	if r.data.CustomerID != "" {
		state.CustomerID = types.StringValue(r.data.CustomerID)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// findZoneByDomain lists the customer's domains and looks for an active zone matching the domain.
func (r *dnsZoneResource) findZoneByDomain(ctx context.Context, customerID, domain string) (*dnsZoneAPI, error) {
	items, err := hosting.List[domainListItem](ctx, r.data.Client, fmt.Sprintf("/api/v1/customers/%s/domains", customerID))
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Type == "zone" && item.Name == domain {
			return &dnsZoneAPI{
				ID:      item.ID,
				Name:    item.Name,
				Managed: item.Managed,
				Status:  item.Status,
			}, nil
		}
	}
	return nil, nil
}

func mapDNSZone2(api *dnsZoneAPI, state *dnsZoneModel, customerID string) {
	state.ID = types.StringValue(api.ID)
	state.Domain = types.StringValue(api.Name)
	state.Managed = types.BoolValue(api.Managed)
	state.Status = types.StringValue(api.Status)
	state.VerificationTXTName = types.StringValue("")
	state.VerificationTXTValue = types.StringValue("")
	if customerID != "" {
		state.CustomerID = types.StringValue(customerID)
	}
}
