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

	composetypes "github.com/compose-spec/compose-go/types"
	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// parseBuildSpec parses the build specification from the model
func (r *ImageResource) parseBuildSpec(ctx context.Context, model *ImageResourceModel) (*composetypes.BuildConfig, error) {
	// The build attribute contains a Docker Compose compatible build specification in JSON format
	buildJSON := model.Build.ValueString()
	if buildJSON == "" {
		return nil, errors.New("build specification is empty")
	}

	// Parse the JSON into a map
	var buildConfig composetypes.BuildConfig
	err := json.Unmarshal([]byte(buildJSON), &buildConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON in build specification: %w", err)
	}

	return &buildConfig, nil
}

// buildDockerImage builds a Docker image using Docker Compose
func (r *ImageResource) buildDockerImage(ctx context.Context, dockerClient *client.Client, buildConfig *composetypes.BuildConfig, model *ImageResourceModel) error {
	tflog.Info(ctx, "Building Docker image", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// Get build context directory
	buildContextDir := buildConfig.Context
	if buildContextDir == "" {
		return fmt.Errorf("build context not specified in build configuration")
	}

	tflog.Debug(ctx, "Building with context", map[string]interface{}{
		"context":    buildContextDir,
		"dockerfile": buildConfig.Dockerfile,
	})

	// Prepare tarball with build context
	buildContextTar, err := r.prepareBuildContext(ctx, buildContextDir)
	if err != nil {
		return fmt.Errorf("failed to prepare build context: %w", err)
	}
	defer buildContextTar.Close()

	// Create build arguments
	buildArgs := make(map[string]*string)
	for k, v := range buildConfig.Args {
		value := v
		buildArgs[k] = value
	}

	// Create build options
	buildOptions := dockertypes.ImageBuildOptions{
		Tags:        []string{model.ImageURI.ValueString()},
		Dockerfile:  buildConfig.Dockerfile,
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
