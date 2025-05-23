package image

import (
	"context"
	"fmt"
	"io"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v2/pkg/api"
	"github.com/docker/compose/v2/pkg/compose"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	tfplugintypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// pushDockerImage pushes a Docker image to the registry
func (r *ImageResource) pushDockerImage(ctx context.Context, dockerClient *client.Client, model *ImageResourceModel) error {
	tflog.Info(ctx, "Pushing Docker image to registry", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// Get authentication configuration
	authConfig, err := r.getAuthConfig(ctx, model)
	if err != nil {
		return fmt.Errorf("failed to get authentication configuration: %w", err)
	}

	// Create encoded authentication string for Docker API
	var encodedAuth string
	if authConfig != nil {
		encodedAuth, err = r.GetEncodedAuthConfig(ctx, authConfig)
		if err != nil {
			return fmt.Errorf("failed to encode auth config: %w", err)
		}
		tflog.Debug(ctx, "Using authentication for pushing image")
	} else {
		tflog.Debug(ctx, "No authentication used for pushing image")
	}

	// Push the image
	pushOptions := image.PushOptions{
		RegistryAuth: encodedAuth,
	}

	pushResponse, err := dockerClient.ImagePush(ctx, model.ImageURI.ValueString(), pushOptions)
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}
	defer pushResponse.Close()

	// Read the response to ensure the push completes
	// Docker API sends progress as a JSON stream
	if _, err := io.ReadAll(pushResponse); err != nil {
		return fmt.Errorf("error reading push response: %w", err)
	}

	tflog.Info(ctx, "Successfully pushed Docker image to registry", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	return nil
}

// buildDockerImageWithCompose builds a Docker image using Docker Compose API
func (r *ImageResource) buildDockerImageWithCompose(ctx context.Context, composeService api.Service, buildSpec map[string]interface{}, model *ImageResourceModel) error {
	tflog.Info(ctx, "Building Docker image using Docker Compose API", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// Create a minimal Docker Compose project structure
	project := &composetypes.Project{
		Name:        "dummy",                             // Using a dummy project name
		WorkingDir:  ".",                                 // Current directory
		Environment: composetypes.NewMapping([]string{}), // Empty environment
	}

	// Create a service for the Docker image to build
	serviceName := "build-service"
	service := composetypes.ServiceConfig{
		Name:  serviceName,
		Image: model.ImageURI.ValueString(),
		Build: &composetypes.BuildConfig{},
	}

	// Configure the build settings from Terraform build spec
	if contextDir, ok := buildSpec["context"].(string); ok && contextDir != "" {
		service.Build.Context = contextDir
		tflog.Debug(ctx, "Using build context", map[string]interface{}{
			"context": contextDir,
		})
	} else {
		service.Build.Context = "." // Default to current directory
	}

	// Set Dockerfile if specified
	if dockerfile, ok := buildSpec["dockerfile"].(string); ok && dockerfile != "" {
		service.Build.Dockerfile = dockerfile
		tflog.Debug(ctx, "Using dockerfile", map[string]interface{}{
			"dockerfile": dockerfile,
		})
	}

	// Add build arguments if specified
	if args, ok := buildSpec["args"].(map[string]interface{}); ok {
		service.Build.Args = composetypes.MappingWithEquals{}
		for key, value := range args {
			if strValue, ok := value.(string); ok {
				service.Build.Args[key] = &strValue
			}
		}
		tflog.Debug(ctx, "Using build args", map[string]interface{}{
			"args": args,
		})
	}

	// Add additional build contexts if specified
	if additionalContexts, ok := buildSpec["additional_contexts"].(map[string]interface{}); ok {
		service.Build.AdditionalContexts = composetypes.Mapping{}
		for name, path := range additionalContexts {
			if strPath, ok := path.(string); ok {
				service.Build.AdditionalContexts[name] = strPath
			}
		}
		tflog.Debug(ctx, "Using additional build contexts", map[string]interface{}{
			"additional_contexts": additionalContexts,
		})
	}

	// Set labels from the model
	labels := r.extractLabels(model)
	if len(labels) > 0 {
		service.Build.Labels = composetypes.Labels{}
		for key, value := range labels {
			service.Build.Labels[key] = value
		}
	}

	// Add the service to the project
	project.Services = composetypes.Services{serviceName: service}

	// Set tags for the image
	imageTarget := model.ImageURI.ValueString()
	service.Build.Tags = []string{imageTarget}

	// Configure build options
	buildOptions := api.BuildOptions{
		Pull:     true,                  // Always pull newest version of base images
		NoCache:  false,                 // Use cache by default
		Services: []string{serviceName}, // Build just our service
	}

	// Execute the build
	err := composeService.Build(ctx, project, buildOptions)
	if err != nil {
		return fmt.Errorf("docker compose build failed: %w", err)
	}

	tflog.Info(ctx, "Successfully built Docker image using Docker Compose API", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	return nil
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
	// Initialize Docker CLI
	dockerCli, err := command.NewDockerCli()
	if err != nil {
		return fmt.Errorf("failed to create Docker CLI: %w", err)
	}

	// Setup Docker CLI with standard streams
	clientOpts := &flags.ClientOptions{}
	err = dockerCli.Initialize(clientOpts, command.WithStandardStreams())
	if err != nil {
		return fmt.Errorf("failed to initialize Docker CLI: %w", err)
	}

	// Initialize Docker Compose service with the CLI
	composeService := compose.NewComposeService(dockerCli)

	// Build the Docker image using Docker Compose API
	err = r.buildDockerImageWithCompose(ctx, composeService, buildSpec, model)
	if err != nil {
		return fmt.Errorf("failed to build Docker image: %w", err)
	}

	// Initialize a Docker client for pushing
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerClient.Close()

	// Push the image to the registry
	err = r.pushDockerImage(ctx, dockerClient, model)
	if err != nil {
		return fmt.Errorf("failed to push Docker image: %w", err)
	}

	// Get the image digest after pushing
	imageInfo, err := r.getImageInfoFromRegistry(ctx, model)
	if err != nil {
		tflog.Warn(ctx, "Failed to get image digest after push", map[string]interface{}{
			"image_uri": model.ImageURI.ValueString(),
			"error":     err.Error(),
		})
		// Don't return error - we can still continue without the digest
	} else {
		// Update the model with the SHA256 digest - prioritize the manifest digest for docker pull
		if manifestDigest, ok := imageInfo["manifest_digest"].(string); ok && manifestDigest != "" {
			model.SHA256Digest = tfplugintypes.StringValue(manifestDigest)
			tflog.Debug(ctx, "Updated image manifest SHA256 digest", map[string]interface{}{
				"image_uri": model.ImageURI.ValueString(),
				"digest":    manifestDigest,
			})
		} else if configDigest, ok := imageInfo["digest"].(string); ok && configDigest != "" {
			// Fall back to config digest if manifest digest is not available
			model.SHA256Digest = tfplugintypes.StringValue(configDigest)
			tflog.Debug(ctx, "Updated image config SHA256 digest (fallback)", map[string]interface{}{
				"image_uri": model.ImageURI.ValueString(),
				"digest":    configDigest,
			})
		}
	}

	return nil
}
