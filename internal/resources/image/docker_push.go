package image

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

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
