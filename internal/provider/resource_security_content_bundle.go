// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.

package provider

import (
	"context"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/vmware/terraform-provider-ssp/internal/provider/client"
)

var _ resource.Resource = &SecurityContentBundleResource{}

func NewSecurityContentBundleResource() resource.Resource {
	return &SecurityContentBundleResource{}
}

// SecurityContentBundleResource manages security-content "mega bundle" uploads.
type SecurityContentBundleResource struct {
	client *client.Client
}

// SecurityContentBundleResourceModel is the Terraform state model.
type SecurityContentBundleResourceModel struct {
	ID              types.String `tfsdk:"id"`
	FilePath        types.String `tfsdk:"file_path"`
	BundleType      types.String `tfsdk:"bundle_type"`
	BundleID        types.String `tfsdk:"bundle_id"`
	Source          types.String `tfsdk:"source"`
	UploadStatus    types.String `tfsdk:"upload_status"`
	StatusMessage   types.String `tfsdk:"status_message"`
	ManifestVersion types.String `tfsdk:"manifest_version"`
	SspVersion      types.String `tfsdk:"ssp_version"`
}

// asyncActivityResponse decodes the minimal subset of the AsyncActivity
// schema this resource needs (the generated upload ID) from the 202 response
// to POST /ssp/security-content/bundles.
type asyncActivityResponse struct {
	ID string `json:"id"`
}

func (r *SecurityContentBundleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_content_bundle"
}

func (r *SecurityContentBundleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Uploads a security-content \"mega bundle\" archive file to the SSP security " +
			"content store.\n\n" +
			"The file is streamed directly from disk in a multipart request rather than buffered fully in " +
			"memory, so upload memory usage stays roughly constant regardless of file size (bundles can be " +
			"several GB). After the upload is accepted, the resource polls the bundle's processing status " +
			"until it reaches a terminal state (`SUCCESS` or `FAILED`), which can take significant time for " +
			"large bundles.\n\n" +
			"Corresponds to `POST /ssp/security-content/bundles`, `GET /ssp/security-content/bundles/{bundle-id}`, " +
			"and `DELETE /ssp/security-content/bundles/{bundle-id}`.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Unique ID of the mega bundle upload.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"file_path": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Path on local disk to the mega bundle archive file to upload.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"bundle_type": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Type of the mega bundle being uploaded. One of `FIREWALL_ATP` " +
					"(Advanced Threat Protection firewall bundle), `AI_ASSISTANT_FIREWALL` (AI Assistant " +
					"Firewall bundle), or `AI_ASSISTANT_AVI` (AI Assistant AVI bundle).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.OneOf("FIREWALL_ATP", "AI_ASSISTANT_FIREWALL", "AI_ASSISTANT_AVI"),
				},
			},
			"bundle_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Bundle identifier reported by the platform once processed.",
			},
			"source": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Source of the bundle (e.g. `MANUAL_UPLOAD`).",
			},
			"upload_status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Processing status of the upload (`SUCCESS`, `FAILED`, `RUNNING`, `ACCEPTED`).",
			},
			"status_message": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Human-readable status message accompanying `upload_status`.",
			},
			"manifest_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Manifest version of the uploaded bundle.",
			},
			"ssp_version": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SSP product version the bundle was built for.",
			},
		},
	}
}

func (r *SecurityContentBundleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, err := GetClientFromProviderData(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data type", err.Error())
		return
	}
	r.client = c
}

func (r *SecurityContentBundleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SecurityContentBundleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	bundleType := data.BundleType.ValueString()
	filePath := data.FilePath.ValueString()

	var asyncResp asyncActivityResponse
	_, err := r.client.PostMultipartFile(ctx, "/ssp/security-content/bundles", "bundle_type="+url.QueryEscape(bundleType), filePath, &asyncResp)
	if err != nil {
		resp.Diagnostics.AddError("Error uploading mega bundle", err.Error())
		return
	}

	if asyncResp.ID == "" {
		resp.Diagnostics.AddError("No bundle ID returned", "API accepted the upload but returned an empty ID.")
		return
	}

	// The upload was genuinely accepted server-side; record the ID before
	// polling so a subsequent poll failure doesn't orphan the upload from
	// Terraform's perspective (the next apply would otherwise re-upload).
	data.ID = types.StringValue(asyncResp.ID)
	data.BundleID = types.StringValue(asyncResp.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)

	bundle, waitErr := r.client.WaitForMegaBundleReady(ctx, asyncResp.ID)
	if bundle != nil {
		mapMegaBundleToState(bundle, &data)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	if waitErr != nil {
		resp.Diagnostics.AddError("Mega bundle upload failed", waitErr.Error())
	}
}

func (r *SecurityContentBundleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SecurityContentBundleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var bundle client.MegaBundle
	httpStatus, err := r.client.Get(ctx, "/ssp/security-content/bundles/"+data.ID.ValueString(), &bundle)
	if httpStatus == 404 {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading mega bundle", err.Error())
		return
	}

	mapMegaBundleToState(&bundle, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SecurityContentBundleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Update not supported", "Mega bundle uploads are immutable; change file_path or bundle_type to force a new upload.")
}

func (r *SecurityContentBundleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SecurityContentBundleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpStatus, err := r.client.Delete(ctx, "/ssp/security-content/bundles/"+data.ID.ValueString())
	if httpStatus == 404 {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error deleting mega bundle", err.Error())
	}
}

func mapMegaBundleToState(bundle *client.MegaBundle, data *SecurityContentBundleResourceModel) {
	if bundle.BundleID != "" {
		data.BundleID = types.StringValue(bundle.BundleID)
	}
	data.Source = types.StringValue(bundle.Source)
	data.UploadStatus = types.StringValue(bundle.UploadStatus)
	data.StatusMessage = types.StringValue(bundle.StatusMessage)
	data.ManifestVersion = types.StringValue(bundle.ManifestVersion)
	data.SspVersion = types.StringValue(bundle.SspVersion)
}
