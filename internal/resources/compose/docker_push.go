package compose

import (
	"context"
	"errors"
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
func (r *ComposeResource) pushDockerImage(ctx context.Context, dockerClient *client.Client, model *ComposeResourceModel) error {
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
func (r *ComposeResource) buildDockerImageWithCompose(
	ctx context.Context,
	composeService api.Service,
	buildSpec *composetypes.BuildConfig,
	model *ComposeResourceModel,
	out io.Writer,
) error {
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
		Build: buildSpec,
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
		Out:      out,
		Services: []string{serviceName},
	}
	if model.Option != nil {
		buildOptions.Pull = model.Option.Pull.ValueBool()
		buildOptions.NoCache = model.Option.NoCache.ValueBool()
		buildOptions.Progress = model.Option.Progress.ValueString()
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

func withLoggingHTTPClient(c *client.Client) error {
	httpClient := c.HTTPClient()
	httpClient.Transport = injectLoggingToTransport(httpClient.Transport)
	return client.WithHTTPClient(httpClient)(c)
}

// buildLogConfig holds buildlog block configuration with schema defaults applied.
type buildLogConfig struct {
	Timestamp bool
	Lines     int
	Log       string
}

// getBuildLogConfig returns buildlog config. Uses schema defaults when buildlog block is absent.
func (r *ComposeResource) getBuildLogConfig(model *ComposeResourceModel) buildLogConfig {
	cfg := buildLogConfig{
		Timestamp: true,
		Lines:     10,
		Log:       "",
	}
	if model.BuildLog != nil {
		cfg.Timestamp = model.BuildLog.Timestamp.ValueBool()
		cfg.Lines = int(model.BuildLog.Lines.ValueInt64())
		if !model.BuildLog.Log.IsNull() {
			cfg.Log = model.BuildLog.Log.ValueString()
		}
	}
	return cfg
}

// buildAndPushImage builds and pushes an image based on the provided model.
// On build failure, it also returns the last N buffered build log lines
func (r *ComposeResource) buildAndPushImage(ctx context.Context, model *ComposeResourceModel) ([]string, error) {
	tflog.Debug(ctx, "Building and pushing image", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// Parse the build specification from JSON
	buildSpec, err := r.parseBuildSpec(ctx, model)
	if err != nil {
		return nil, fmt.Errorf("failed to parse build specification: %w", err)
	}

	buildLogCfg := r.getBuildLogConfig(model)
	capture := newBuildLogCapture(ctx, buildLogCfg.Timestamp, buildLogCfg.Lines, buildLogCfg.Log)
	defer func() {
		_ = capture.Close()
		capture.Wait()
	}()

	// Initialize Docker CLI with capture streams so output does not go to Terraform stdout
	dockerCli, err := command.NewDockerCli()
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker CLI: %w", err)
	}

	clientOpts := &flags.ClientOptions{}
	err = dockerCli.Initialize(clientOpts,
		command.WithOutputStream(capture.Writer()),
		command.WithErrorStream(capture.Writer()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Docker CLI: %w", err)
	}

	capture.Start(ctx)

	// Initialize Docker Compose service with the CLI
	composeService := compose.NewComposeService(dockerCli)

	// Build the Docker image using Docker Compose API
	err = r.buildDockerImageWithCompose(ctx, composeService, buildSpec, model, capture.Writer())
	if err != nil {
		_ = capture.Close()
		capture.Wait()
		return capture.GetLastLines(), fmt.Errorf("failed to build Docker image: %w", err)
	}

	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
		withLoggingHTTPClient,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerClient.Close()

	// Push the image to the registry
	err = r.pushDockerImage(ctx, dockerClient, model)
	if err != nil {
		return nil, fmt.Errorf("failed to push Docker image: %w", err)
	}

	// Get the image digest after pushing
	imageInfo, err := r.getImageInfoFromRegistry(ctx, model)
	if err != nil {
		return nil, fmt.Errorf("failed to get image digest after push: %w", err)
	}
	if imageInfo.ManifestDigest == "" {
		return nil, errors.New("manifest digest is empty")
	}

	// Update the model with the SHA256 digest - prioritize the manifest digest for docker pull
	model.SHA256Digest = tfplugintypes.StringValue(imageInfo.ManifestDigest)
	tflog.Debug(ctx, "Updated image manifest SHA256 digest", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
		"digest":    imageInfo.ManifestDigest,
	})

	return nil, nil
}
