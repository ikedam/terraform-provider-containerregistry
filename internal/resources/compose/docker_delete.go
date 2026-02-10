package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/distribution/reference"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// deleteImageFromRegistry deletes an image from a remote registry
func (r *ComposeResource) deleteImageFromRegistry(ctx context.Context, model *ComposeResourceModel) error {
	tflog.Info(ctx, "Deleting image from registry", map[string]interface{}{
		"image_uri": model.ImageURI.ValueString(),
	})

	// Parse the image reference to extract registry, repository, and tag/digest information
	imageURI := model.ImageURI.ValueString()
	ref, err := reference.ParseAnyReference(imageURI)
	if err != nil {
		return fmt.Errorf("invalid image URI format: %w", err)
	}

	// Get authentication configuration
	authConfig, err := r.getAuthConfig(ctx, model)
	if err != nil {
		return fmt.Errorf("failed to get authentication configuration: %w", err)
	}

	return r.deleteFromDockerRegistry(ctx, ref, authConfig)
}

// deleteFromDockerRegistry deletes an image from a generic Docker Registry using the Registry API v2
func (r *ComposeResource) deleteFromDockerRegistry(ctx context.Context, ref reference.Reference, authConfig *AuthConfig) error {
	// Extract registry, repository, and reference components
	var registry, repository, tag, digest string

	// Extract repository and registry
	namedRef, ok := ref.(reference.Named)
	if !ok {
		return fmt.Errorf("invalid image reference format")
	}

	registry = reference.Domain(namedRef)
	repository = reference.Path(namedRef)

	// Extract tag or digest
	if taggedRef, isTagged := ref.(reference.NamedTagged); isTagged {
		tag = taggedRef.Tag()
	} else if digestRef, hasDigest := ref.(reference.Canonical); hasDigest {
		digest = digestRef.Digest().String()
	} else {
		return fmt.Errorf("image reference must have a tag or digest")
	}

	tflog.Debug(ctx, "Parsed image reference", map[string]interface{}{
		"registry":   registry,
		"repository": repository,
		"tag":        tag,
		"digest":     digest,
	})

	// Create HTTP client with Terraform logging transport
	client := newHTTPLoggingClient()
	var url string

	if digest != "" {
		// If we have a digest, delete by digest
		url = fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, digest)
	} else if tag != "" {
		// If we have a tag, we need to get the digest first
		// Get the manifest for the tag
		manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)

		req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create manifest request: %w", err)
		}

		// Add accept header for manifest v2
		req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")

		// Add authorization headers if we have authentication config
		if authConfig != nil {
			authHeader := r.GetHTTPAuthHeader(ctx, authConfig)
			if authHeader != "" {
				req.Header.Add("Authorization", authHeader)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to get manifest: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("authentication failed for registry: %s", registry)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to get manifest, status: %d", resp.StatusCode)
		}

		// Extract the digest from the Docker-Content-Digest header
		digest = resp.Header.Get("Docker-Content-Digest")
		if digest == "" {
			// If not in header, parse from body
			var manifest struct {
				Config struct {
					Digest string `json:"digest"`
				} `json:"config"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
				return fmt.Errorf("failed to decode manifest: %w", err)
			}

			digest = manifest.Config.Digest
		}

		if digest == "" {
			return fmt.Errorf("could not determine digest for tag %s", tag)
		}

		// Now we can delete using the digest
		url = fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, digest)
	}

	// Create DELETE request
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create DELETE request: %w", err)
	}

	// Add authorization headers if we have authentication config
	if authConfig != nil {
		authHeader := r.GetHTTPAuthHeader(ctx, authConfig)
		if authHeader != "" {
			req.Header.Add("Authorization", authHeader)
		}
	}

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute DELETE request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed for registry: %s", registry)
	}

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete image, status: %d", resp.StatusCode)
	}

	tflog.Info(ctx, "Successfully deleted image from registry", map[string]interface{}{
		"repository": repository,
		"tag":        tag,
		"digest":     digest,
	})

	return nil
}
