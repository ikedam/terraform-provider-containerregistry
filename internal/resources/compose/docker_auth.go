package compose

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/registry"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// AuthConfig represents the authentication configuration for a Docker registry
type AuthConfig struct {
	Username string
	Password string
	Auth     string
}

// registryHostFromImageURI returns the registry hostname portion of image_uri (reference.Domain).
func registryHostFromImageURI(imageURI string) (string, error) {
	ref, err := reference.ParseAnyReference(imageURI)
	if err != nil {
		return "", fmt.Errorf("invalid image URI: %w", err)
	}
	named, ok := ref.(reference.Named)
	if !ok {
		return "", fmt.Errorf("image URI must be a named reference")
	}
	return reference.Domain(named), nil
}

// getAuthConfig returns credentials for the registry host in imageURI using provider registry_auth.
func (r *ComposeResource) getAuthConfig(ctx context.Context, imageURI string) (*AuthConfig, error) {
	if r.providerConfig == nil || len(r.providerConfig.RegistryAuth) == 0 {
		tflog.Debug(ctx, "No provider registry_auth configured")
		return nil, nil
	}

	host, err := registryHostFromImageURI(imageURI)
	if err != nil {
		return nil, err
	}

	creds, ok := r.providerConfig.RegistryAuth[host]
	if !ok {
		tflog.Debug(ctx, "No registry_auth entry for registry host", map[string]any{
			"registry_host": host,
		})
		return nil, nil
	}
	if creds.Username == "" || creds.Password == "" {
		return nil, fmt.Errorf("registry_auth for %q has empty username or password", host)
	}

	tflog.Debug(ctx, "Using provider registry_auth for registry host", map[string]any{
		"registry_host": host,
	})
	return &AuthConfig{Username: creds.Username, Password: creds.Password}, nil
}

// GetEncodedAuthConfig converts the AuthConfig to a base64 encoded string for Docker API
func (r *ComposeResource) GetEncodedAuthConfig(_ context.Context, authConfig *AuthConfig) (string, error) {
	if authConfig == nil {
		return "", nil
	}

	dockerAuthConfig := registry.AuthConfig{
		Username: authConfig.Username,
		Password: authConfig.Password,
	}

	encodedJSON, err := json.Marshal(dockerAuthConfig)
	if err != nil {
		return "", fmt.Errorf("unable to encode auth config: %w", err)
	}

	return base64.URLEncoding.EncodeToString(encodedJSON), nil
}

// GetHTTPAuthHeader returns an HTTP Authorization header value for registry API requests
func (r *ComposeResource) GetHTTPAuthHeader(_ context.Context, authConfig *AuthConfig) string {
	if authConfig == nil {
		return ""
	}

	auth := fmt.Sprintf("%s:%s", authConfig.Username, authConfig.Password)
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}
