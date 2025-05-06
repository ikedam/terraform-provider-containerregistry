package image

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ resource.Resource = &ImageResource{}
var _ resource.ResourceWithImportState = &ImageResource{}

// NewImageResource returns a new resource implementing the containerregistry_image resource type.
func NewImageResource() resource.Resource {
	return &ImageResource{}
}

// ImageResource defines the resource implementation.
type ImageResource struct {
	// client would be defined here if we had a client to communicate with the container registry
}

// Metadata returns the resource type name.
func (r *ImageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_image"
}

// Schema defines the schema for the resource.
func (r *ImageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Container registry image resource",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier of the image",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"image_uri": schema.StringAttribute{
				MarkdownDescription: "URI of the image to build and push",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"build": schema.StringAttribute{
				MarkdownDescription: "Docker compose v2 compatible build specification in JSON format",
				Required:            true,
			},
			"labels": schema.MapAttribute{
				MarkdownDescription: "Labels for the image",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"triggers": schema.MapAttribute{
				MarkdownDescription: "Map of arbitrary strings that, when changed, will force the image to be rebuilt",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"delete_image": schema.BoolAttribute{
				MarkdownDescription: "Whether to delete the image when the resource is deleted",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"auth": schema.ObjectAttribute{
				MarkdownDescription: "Authentication configuration for the container registry",
				Optional:            true,
				AttributeTypes: map[string]attr.Type{
					"aws_ecr": types.ObjectType{
						AttrTypes: map[string]attr.Type{
							"profile": types.StringType,
						},
					},
					"google_artifact_registry": types.ObjectType{
						AttrTypes: map[string]attr.Type{},
					},
					"username_password": types.ObjectType{
						AttrTypes: map[string]attr.Type{
							"username":              types.StringType,
							"password":              types.StringType,
							"aws_secrets_manager":   types.StringType,
							"google_secret_manager": types.StringType,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ImageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	// Here we would get the client from the provider if we had one
	// client, ok := req.ProviderData.(*SomeClient)
}

// Create creates the resource and sets the initial Terraform state.
func (r *ImageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Get the plan and model
	var plan ImageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Log the creation operation
	tflog.Info(ctx, "Creating container registry image", map[string]interface{}{
		"image_uri": plan.ImageURI.ValueString(),
	})

	// Build and push the image
	err := r.buildAndPushImage(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error building and pushing image",
			fmt.Sprintf("Could not build and push image %s: %s", plan.ImageURI.ValueString(), err),
		)
		return
	}

	// Set the ID to the image URI
	plan.ID = plan.ImageURI

	// Save the plan to the state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *ImageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get the current state
	var state ImageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Log the read operation
	tflog.Info(ctx, "Reading container registry image", map[string]interface{}{
		"image_uri": state.ImageURI.ValueString(),
		"id":        state.ID.ValueString(),
	})

	// Try to fetch image information from the container registry using the Registry API
	// We use the image URI stored in the state file, even when the tag might have changed
	imageInfo, err := r.getImageInfoFromRegistry(ctx, &state)
	if err != nil {
		tflog.Warn(ctx, "Failed to get image info from registry", map[string]interface{}{
			"image_uri": state.ImageURI.ValueString(),
			"error":     err.Error(),
		})

		// If the image doesn't exist in the registry, mark it as deleted from state
		resp.State.RemoveResource(ctx)
		return
	}

	// If image exists, update label information from the registry
	if labels, ok := imageInfo["labels"].(map[string]string); ok && len(labels) > 0 {
		tflog.Debug(ctx, "Updating labels from registry", map[string]interface{}{
			"image_uri": state.ImageURI.ValueString(),
			"labels":    labels,
		})

		// Convert the map[string]string to map[string]attr.Value for Terraform
		labelValues := make(map[string]attr.Value, len(labels))
		for k, v := range labels {
			labelValues[k] = types.StringValue(v)
		}

		// Create a new labels map
		labelsMap, diags := types.MapValue(types.StringType, labelValues)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		// Update the state model with the new labels
		state.Labels = labelsMap

		tflog.Info(ctx, "Updated image labels from registry", map[string]interface{}{
			"image_uri": state.ImageURI.ValueString(),
		})
	}

	// Save the updated state
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *ImageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Get plan and current state
	var plan, state ImageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Log the update operation
	tflog.Info(ctx, "Updating container registry image", map[string]interface{}{
		"image_uri": plan.ImageURI.ValueString(),
	})

	// Build and push the image
	err := r.buildAndPushImage(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error building and pushing image",
			fmt.Sprintf("Could not build and push image %s: %s", plan.ImageURI.ValueString(), err),
		)
		return
	}

	// Save the updated plan to the state
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *ImageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Get current state
	var state ImageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Log the delete operation
	tflog.Info(ctx, "Deleting container registry image", map[string]interface{}{
		"image_uri": state.ImageURI.ValueString(),
	})

	// Check if we should actually delete the image
	if state.DeleteImage.ValueBool() {
		// Delete the image from the registry
		tflog.Info(ctx, "Deleting the image from registry", map[string]interface{}{
			"image_uri": state.ImageURI.ValueString(),
		})

		err := r.deleteImageFromRegistry(ctx, &state)
		if err != nil {
			resp.Diagnostics.AddWarning(
				"Error deleting image from registry",
				fmt.Sprintf("Could not delete image %s: %s", state.ImageURI.ValueString(), err),
			)
			// Continue with resource deletion even if image deletion fails
		} else {
			tflog.Info(ctx, "Successfully deleted image from registry", map[string]interface{}{
				"image_uri": state.ImageURI.ValueString(),
			})
		}
	}

	// No need to update the state as it will be removed by Terraform after this function returns
}

// ImportState imports an existing resource into Terraform.
func (r *ImageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Log the import operation
	tflog.Info(ctx, "Importing container registry image", map[string]interface{}{
		"image_uri": req.ID,
	})

	// Set the image_uri attribute from the provided ID (which is expected to be the image URI)
	resp.Diagnostics.Append(resp.State.SetAttribute(
		ctx,
		path.Root("image_uri"),
		req.ID,
	)...)

	// Generate a new UUID for the resource ID
	// This is needed because we don't use image URI as the resource ID since the tag might change
	id := generateUUID()
	resp.Diagnostics.Append(resp.State.SetAttribute(
		ctx,
		path.Root("id"),
		id,
	)...)

	// Set default values for optional attributes
	resp.Diagnostics.Append(resp.State.SetAttribute(
		ctx,
		path.Root("delete_image"),
		false,
	)...)

	// The remaining attributes like build, labels, triggers, and auth
	// will need to be set by the user after import
	tflog.Info(ctx, "Successfully imported image, additional configuration required", map[string]interface{}{
		"image_uri": req.ID,
		"id":        id,
	})
}

// generateUUID generates a new UUID for resource identification
func generateUUID() string {
	// Import package in the top of the file: "github.com/google/uuid"
	return uuid.New().String()
}
