package image

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/loader"
	composetypes "github.com/compose-spec/compose-go/types"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
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
						AttrTypes: map[string]attr.Type{
							"service_account": types.StringType,
						},
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
	})

	// Here we would typically read the image info from the container registry
	// and update the state with it, but for the skeleton we'll just keep
	// the state as is.

	// Save state
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
		// Here we would typically delete the image from the registry
		tflog.Info(ctx, "Would delete the image from registry (if implemented)")
	}

	// No need to update the state as it will be removed by Terraform after this function returns
}

// ImportState imports an existing resource into Terraform.
func (r *ImageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildAndPushImage builds and pushes an image based on the provided model
func (r *ImageResource) buildAndPushImage(ctx context.Context, model *ImageResourceModel) error {
	tflog.Debug(ctx, "Building and pushing image", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// Parse the build specification from JSON
	buildSpec, err := r.parseBuildSpec(ctx, model)
	if err != nil {
		return fmt.Errorf("failed to parse build specification: %w", err)
	}

	// Initialize a Docker client
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerClient.Close()

	// Build the Docker image using Docker Compose
	err = r.buildDockerImage(ctx, dockerClient, buildSpec, model)
	if err != nil {
		return fmt.Errorf("failed to build Docker image: %w", err)
	}

	// Perform authentication with the container registry
	err = r.authenticateRegistry(ctx, model)
	if err != nil {
		return fmt.Errorf("failed to authenticate with registry: %w", err)
	}

	// Push the image to the registry
	err = r.pushDockerImage(ctx, dockerClient, model)
	if err != nil {
		return fmt.Errorf("failed to push Docker image: %w", err)
	}

	return nil
}

// parseBuildSpec parses the build specification from the model
func (r *ImageResource) parseBuildSpec(ctx context.Context, model *ImageResourceModel) (*composetypes.Project, error) {
	// The build attribute contains a Docker Compose compatible build specification in JSON format
	buildJSON := model.Build.ValueString()
	if buildJSON == "" {
		return nil, errors.New("build specification is empty")
	}

	// Parse the JSON into a map
	var buildConfig map[string]interface{}
	err := json.Unmarshal([]byte(buildJSON), &buildConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON in build specification: %w", err)
	}

	// Create a temporary directory for the Docker Compose file
	tempDir, err := os.MkdirTemp("", "tf-docker-build-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a simple Docker Compose project with the build specification
	composeConfig := map[string]interface{}{
		"version": "3",
		"services": map[string]interface{}{
			"app": map[string]interface{}{
				"build": buildConfig,
				"image": model.ImageURI.ValueString(),
			},
		},
	}

	// Convert to JSON
	composeJSON, err := json.Marshal(composeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal compose config: %w", err)
	}

	tflog.Debug(ctx, "Created Docker Compose configuration", map[string]interface{}{
		"compose_json": string(composeJSON),
	})

	// Create a temporary file for the Docker Compose configuration
	composePath := fmt.Sprintf("%s/docker-compose.json", tempDir)
	err = os.WriteFile(composePath, composeJSON, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write Docker Compose file: %w", err)
	}

	// Load the Docker Compose project
	project, err := loader.Load(composetypes.ConfigDetails{
		ConfigFiles: []composetypes.ConfigFile{
			{
				Filename: composePath,
				Content:  composeJSON,
			},
		},
		WorkingDir: tempDir,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load Docker Compose project: %w", err)
	}

	return project, nil
}

// buildDockerImage builds a Docker image using Docker Compose
func (r *ImageResource) buildDockerImage(ctx context.Context, dockerClient *client.Client, project *composetypes.Project, model *ImageResourceModel) error {
	tflog.Info(ctx, "Building Docker image", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// Create a service to build
	service, err := project.GetService("app")
	if err != nil {
		return fmt.Errorf("failed to get service from Docker Compose project: %w", err)
	}

	// Get build context directory
	buildContextDir := service.Build.Context
	if buildContextDir == "" {
		return fmt.Errorf("build context not specified in build configuration")
	}

	tflog.Debug(ctx, "Building with context", map[string]interface{}{
		"context":    buildContextDir,
		"dockerfile": service.Build.Dockerfile,
	})

	// Prepare tarball with build context
	buildContextTar, err := r.prepareBuildContext(ctx, buildContextDir)
	if err != nil {
		return fmt.Errorf("failed to prepare build context: %w", err)
	}
	defer buildContextTar.Close()

	// Create build arguments
	buildArgs := make(map[string]*string)
	for k, v := range service.Build.Args {
		value := v
		buildArgs[k] = value
	}

	// Create build options
	buildOptions := dockertypes.ImageBuildOptions{
		Tags:        []string{model.ImageURI.ValueString()},
		Dockerfile:  service.Build.Dockerfile,
		BuildArgs:   buildArgs,
		Remove:      true,
		ForceRemove: true,
		PullParent:  true,
		Labels:      r.extractLabels(model),
	}

	// Build the image
	buildResponse, err := dockerClient.ImageBuild(ctx, buildContextTar, buildOptions)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}
	defer buildResponse.Body.Close()

	// Process the build output
	err = r.processBuildOutput(ctx, buildResponse.Body)
	if err != nil {
		return fmt.Errorf("build process failed: %w", err)
	}

	tflog.Info(ctx, "Docker image built successfully", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	return nil
}

// prepareBuildContext creates a tar archive of the build context directory
func (r *ImageResource) prepareBuildContext(ctx context.Context, contextDir string) (*os.File, error) {
	tflog.Debug(ctx, "Preparing build context", map[string]interface{}{
		"context_dir": contextDir,
	})

	// Create a temporary file for the build context tarball
	buildContextTarFile, err := os.CreateTemp("", "docker-build-context-*.tar")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file for build context: %w", err)
	}

	// Create a tar writer for the temporary file
	tarWriter := tar.NewWriter(buildContextTarFile)

	// Walk through the build context directory to add all files to the tarball
	err = filepath.Walk(contextDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories as they are created implicitly
		if info.IsDir() {
			return nil
		}

		// Get the relative path for the file inside the tarball
		relPath, err := filepath.Rel(contextDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Create header for the file
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header: %w", err)
		}
		header.Name = relPath

		// Write the header to the tar archive
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header: %w", err)
		}

		// Open and read the file
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()

		// Copy the file content to the tar archive
		if _, err := io.Copy(tarWriter, file); err != nil {
			return fmt.Errorf("failed to copy file content: %w", err)
		}

		return nil
	})

	// Close the tar writer
	if err := tarWriter.Close(); err != nil {
		buildContextTarFile.Close()
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Seek to the beginning of the file for reading
	if _, err := buildContextTarFile.Seek(0, 0); err != nil {
		buildContextTarFile.Close()
		return nil, fmt.Errorf("failed to seek to the beginning of tar file: %w", err)
	}

	return buildContextTarFile, err
}

// processBuildOutput processes the output stream from the Docker build process
func (r *ImageResource) processBuildOutput(ctx context.Context, buildOutput io.ReadCloser) error {
	decoder := json.NewDecoder(buildOutput)

	type BuildOutput struct {
		Stream string `json:"stream"`
		Error  string `json:"error"`
	}

	// Process each line of output
	for {
		var output BuildOutput
		if err := decoder.Decode(&output); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error decoding build output: %w", err)
		}

		// Log any output stream content
		if output.Stream != "" {
			tflog.Debug(ctx, output.Stream)
		}

		// If there's an error, return it
		if output.Error != "" {
			return fmt.Errorf("build error: %s", output.Error)
		}
	}

	return nil
}

// extractLabels extracts labels from the model
func (r *ImageResource) extractLabels(model *ImageResourceModel) map[string]string {
	labels := make(map[string]string)

	// Extract labels from the model if they exist
	if !model.Labels.IsNull() && !model.Labels.IsUnknown() {
		elements := model.Labels.Elements()
		for k, v := range elements {
			if strVal, ok := v.(types.String); ok {
				labels[k] = strVal.ValueString()
			}
		}
	}

	return labels
}

// authenticateRegistry authenticates with the container registry
func (r *ImageResource) authenticateRegistry(ctx context.Context, model *ImageResourceModel) error {
	tflog.Info(ctx, "Authenticating with container registry", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// TODO: Implement authentication with container registry
	// This is a placeholder where authentication would be implemented
	// Authentication would depend on the registry type (Docker Hub, ECR, GAR, etc.)
	// and the authentication method provided in the model.Auth field

	// For now, we'll assume authentication is successful
	return nil
}

// pushDockerImage pushes a Docker image to the registry
func (r *ImageResource) pushDockerImage(ctx context.Context, dockerClient *client.Client, model *ImageResourceModel) error {
	tflog.Info(ctx, "Pushing Docker image to registry", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// In reality, this would involve:
	// 1. Creating authentication information
	// 2. Pushing the image using the Docker API

	// For demonstration, we'll log that the push would happen here
	tflog.Info(ctx, "Docker image push would happen here", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// In reality, you would execute the push using the Docker API
	// For example:
	/*
		// Create authentication configuration
		authConfig := registry.AuthConfig{
			Username: username,
			Password: password,
		}
		encodedAuth, err := registry.EncodeAuthConfig(authConfig)
		if err != nil {
			return err
		}
	*/

	// Push the image
	pushOptions := image.PushOptions{
		// RegistryAuth: encodedAuth,
	}
	pushResponse, err := dockerClient.ImagePush(ctx, model.ImageURI.ValueString(), pushOptions)
	if err != nil {
		return err
	}
	defer pushResponse.Close()

	return nil
}
