// filepath: /workspaces/terraform-provider-containerregistry/internal/resources/image/docker_manifest.go
package image

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/distribution/reference"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// getImageInfoFromRegistry retrieves image information from the container registry using the Registry API
// It updates the provided model with information fetched from the registry
func (r *ImageResource) getImageInfoFromRegistry(ctx context.Context, model *ImageResourceModel) (map[string]interface{}, error) {
	// Log the operation
	tflog.Debug(ctx, "Getting image info from registry", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// Parse the image reference to extract registry, repository, and tag/digest information
	imageURI := model.ImageURI.ValueString()
	ref, err := reference.ParseAnyReference(imageURI)
	if err != nil {
		return nil, fmt.Errorf("invalid image URI format: %w", err)
	}

	// Extract registry, repository, and reference components
	var registry, repository, tag, digest string

	// Extract repository and registry
	namedRef, ok := ref.(reference.Named)
	if !ok {
		return nil, fmt.Errorf("invalid image reference format")
	}

	registry = reference.Domain(namedRef)
	repository = reference.Path(namedRef)

	// Extract tag or digest
	if taggedRef, isTagged := ref.(reference.NamedTagged); isTagged {
		tag = taggedRef.Tag()
	} else if digestRef, hasDigest := ref.(reference.Canonical); hasDigest {
		digest = digestRef.Digest().String()
	} else {
		return nil, fmt.Errorf("image reference must have a tag or digest")
	}

	tflog.Debug(ctx, "Parsed image reference", map[string]interface{}{
		"registry":   registry,
		"repository": repository,
		"tag":        tag,
		"digest":     digest,
	})

	// Authenticate with the registry based on the authentication configuration in the model
	authConfig, err := r.getAuthConfig(ctx, model)
	if err != nil {
		return nil, fmt.Errorf("failed to get authentication configuration: %w", err)
	}

	// Create HTTP client to interact with the Registry API
	client := &http.Client{}

	// First, we need to get the manifest for the image
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)
	if digest != "" {
		manifestURL = fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, digest)
	}

	// Create request to get the manifest
	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest request: %w", err)
	}

	// Add accept headers to get the manifest in the v2 format
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Add("Accept", "application/vnd.oci.image.manifest.v1+json")

	// Add authorization headers if we have authentication config
	if authConfig != nil {
		// Add Basic authentication header
		authHeader := r.GetHTTPAuthHeader(ctx, authConfig)
		if authHeader != "" {
			req.Header.Add("Authorization", authHeader)
		}
	}

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get manifest: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("image not found: %s", imageURI)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed for registry: %s", registry)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get manifest, status: %d", resp.StatusCode)
	}

	// Parse the manifest to extract the config digest
	var manifest struct {
		SchemaVersion int    `json:"schemaVersion"`
		MediaType     string `json:"mediaType"`
		Config        struct {
			MediaType string `json:"mediaType"`
			Size      int    `json:"size"`
			Digest    string `json:"digest"`
		} `json:"config"`
		Layers []struct {
			MediaType string `json:"mediaType"`
			Size      int    `json:"size"`
			Digest    string `json:"digest"`
		} `json:"layers"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	// Now we need to get the image configuration blob which contains the labels
	configURL := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repository, manifest.Config.Digest)

	configReq, err := http.NewRequestWithContext(ctx, "GET", configURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create config request: %w", err)
	}

	// Add authorization headers if we have authentication config
	if authConfig != nil {
		// Add Basic authentication header
		authHeader := r.GetHTTPAuthHeader(ctx, authConfig)
		if authHeader != "" {
			configReq.Header.Add("Authorization", authHeader)
		}
	}

	// Execute the config request
	configResp, err := client.Do(configReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	defer configResp.Body.Close()

	// Check for errors
	if configResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get config, status: %d", configResp.StatusCode)
	}

	// Parse the config blob to get the labels
	var configBlob struct {
		Architecture string `json:"architecture"`
		Created      string `json:"created"`
		OS           string `json:"os"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
		History []struct {
			Created    string `json:"created"`
			CreatedBy  string `json:"created_by"`
			EmptyLayer bool   `json:"empty_layer,omitempty"`
		} `json:"history"`
	}

	if err := json.NewDecoder(configResp.Body).Decode(&configBlob); err != nil {
		return nil, fmt.Errorf("failed to decode config blob: %w", err)
	}

	// Extract labels from the config
	labels := make(map[string]string)
	if configBlob.Config.Labels != nil {
		labels = configBlob.Config.Labels
	}

	// Get the manifest digest from the Docker-Content-Digest header
	// This is the digest that should be used with docker pull image@sha256:digest
	manifestDigest := resp.Header.Get("Docker-Content-Digest")

	// Create the result map with the image information
	imageInfo := map[string]interface{}{
		"exists":          true,
		"labels":          labels,
		"created":         configBlob.Created,
		"digest":          manifest.Config.Digest,
		"manifest_digest": manifestDigest,
		"architecture":    configBlob.Architecture,
		"os":              configBlob.OS,
	}

	tflog.Debug(ctx, "Retrieved image info from registry", map[string]interface{}{
		"image_uri":       model.ImageURI.ValueString(),
		"labels":          labels,
		"config_digest":   manifest.Config.Digest,
		"manifest_digest": manifestDigest,
	})

	return imageInfo, nil
}
