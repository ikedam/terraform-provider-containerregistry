package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/distribution/reference"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// ImageInfo represents the minimal information retrieved from the container registry
type ImageInfo struct {
	ManifestDigest string            `json:"manifest_digest"`
	Labels         map[string]string `json:"labels"`
}

// getImageInfoFromRegistry retrieves minimal image information from the container registry
func (r *ComposeResource) getImageInfoFromRegistry(ctx context.Context, model *ComposeResourceModel) (*ImageInfo, error) {
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

	// Create HTTP client to interact with the Registry API, using Terraform logging transport.
	client := newHTTPLoggingClient()

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
	// Support for OCI Image Index (multi-platform image)
	req.Header.Add("Accept", "application/vnd.oci.image.index.v1+json")

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
		// This will be set when the image is a multi-platform image.
		Manifests []struct {
			MediaType string `json:"mediaType"`
			Size      int    `json:"size"`
			Digest    string `json:"digest"`
			Platform  struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
			Annotations map[string]string `json:"annotations"`
		} `json:"manifests"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	// Handle OCI Image Index (multi-platform image)
	if manifest.MediaType == "application/vnd.oci.image.index.v1+json" {
		// Find the first non-attestation manifest
		var selectedDigest string
		for _, m := range manifest.Manifests {
			// Skip attestation manifests
			if m.Annotations != nil {
				if refType, exists := m.Annotations["vnd.docker.reference.type"]; exists && refType == "attestation-manifest" {
					continue
				}
			}
			selectedDigest = m.Digest
			break
		}

		if selectedDigest == "" {
			return nil, fmt.Errorf("no suitable manifest found in OCI Image Index")
		}

		tflog.Info(ctx, "Selected manifest from OCI Image Index", map[string]interface{}{
			"digest": selectedDigest,
		})

		// For OCI Index, we need to fetch the actual manifest to get the config digest
		actualManifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, selectedDigest)
		actualReq, err := http.NewRequestWithContext(ctx, "GET", actualManifestURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create actual manifest request: %w", err)
		}

		// Add accept headers for the actual manifest
		actualReq.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
		actualReq.Header.Add("Accept", "application/vnd.oci.image.manifest.v1+json")

		// Add authorization headers if we have authentication config
		if authConfig != nil {
			authHeader := r.GetHTTPAuthHeader(ctx, authConfig)
			if authHeader != "" {
				actualReq.Header.Add("Authorization", authHeader)
			}
		}

		// Execute the actual manifest request
		actualResp, err := client.Do(actualReq)
		if err != nil {
			return nil, fmt.Errorf("failed to get actual manifest: %w", err)
		}
		defer actualResp.Body.Close()

		// Check for errors
		if actualResp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to get actual manifest, status: %d", actualResp.StatusCode)
		}

		// Parse the actual manifest
		if err := json.NewDecoder(actualResp.Body).Decode(&manifest); err != nil {
			return nil, fmt.Errorf("failed to decode actual manifest: %w", err)
		}
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

	// Create the result struct with minimal information
	imageInfo := &ImageInfo{
		ManifestDigest: manifestDigest,
		Labels:         labels,
	}

	tflog.Debug(ctx, "Retrieved image info from registry", map[string]interface{}{
		"image_uri":         model.ImageURI.ValueString(),
		"labels":            labels,
		"digest_for_labels": manifest.Config.Digest,
		"manifest_digest":   manifestDigest,
	})

	return imageInfo, nil
}
