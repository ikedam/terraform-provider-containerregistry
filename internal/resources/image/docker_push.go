package image

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
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

	// Push the image to the registry
	err = r.pushDockerImage(ctx, dockerClient, model)
	if err != nil {
		return fmt.Errorf("failed to push Docker image: %w", err)
	}

	return nil
}
