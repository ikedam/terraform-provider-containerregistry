package image

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/docker/docker/api/types/registry"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"google.golang.org/api/option"
)

// AuthConfig represents the authentication configuration for a Docker registry
type AuthConfig struct {
	Username string
	Password string
	Auth     string
}

// getAuthConfig returns the authentication configuration for the container registry
// based on the authentication options provided in the model
func (r *ImageResource) getAuthConfig(ctx context.Context, model *ImageResourceModel) (*AuthConfig, error) {
	// If no authentication is provided, return nil
	if model.Auth.IsNull() || model.Auth.IsUnknown() {
		tflog.Debug(ctx, "No authentication configuration provided")
		return nil, nil
	}

	// Get the auth object from the model
	authMap := make(map[string]interface{})
	diags := model.Auth.As(ctx, &authMap, basetypes.ObjectAsOptions{})
	if diags.HasError() {
		return nil, fmt.Errorf("failed to parse auth configuration: %v", diags)
	}

	// Check for username/password authentication
	if usernamePassMap, ok := authMap["username_password"].(map[string]interface{}); ok {
		return r.getUsernamePasswordAuth(ctx, usernamePassMap)
	}

	// No authentication method found
	tflog.Debug(ctx, "No supported authentication method found")
	return nil, nil
}

// getUsernamePasswordAuth extracts username and password from the auth configuration
func (r *ImageResource) getUsernamePasswordAuth(ctx context.Context, authMap map[string]interface{}) (*AuthConfig, error) {
	var username, password string

	// Extract username if it exists
	if usernameVal, ok := authMap["username"].(string); ok && usernameVal != "" {
		username = usernameVal
	}

	// Extract password if it exists
	if passwordVal, ok := authMap["password"].(string); ok && passwordVal != "" {
		password = passwordVal
	}

	// If both username and password are provided, create the auth configuration
	if username != "" && password != "" {
		tflog.Debug(ctx, "Using username/password authentication")

		// Create and return the authentication configuration
		return &AuthConfig{
			Username: username,
			Password: password,
		}, nil
	}

	// Check for AWS Secrets Manager authentication
	if awsSecretsArn, ok := authMap["aws_secrets_manager"].(string); ok && awsSecretsArn != "" {
		tflog.Debug(ctx, "Using AWS Secrets Manager authentication")

		// Get credentials from AWS Secrets Manager
		awsAuth, err := r.getAWSSecretsManagerAuth(ctx, awsSecretsArn)
		if err != nil {
			return nil, fmt.Errorf("failed to get authentication from AWS Secrets Manager: %w", err)
		}

		return awsAuth, nil
	}

	// Check for Google Secret Manager authentication
	if googleSecretResource, ok := authMap["google_secret_manager"].(string); ok && googleSecretResource != "" {
		tflog.Debug(ctx, "Using Google Secret Manager authentication")

		// Get credentials from Google Secret Manager
		googleAuth, err := r.getGoogleSecretManagerAuth(ctx, googleSecretResource)
		if err != nil {
			return nil, fmt.Errorf("failed to get authentication from Google Secret Manager: %w", err)
		}

		return googleAuth, nil
	}

	return nil, fmt.Errorf("insufficient authentication information provided")
}

// getAWSSecretsManagerAuth retrieves authentication information from AWS Secrets Manager
func (r *ImageResource) getAWSSecretsManagerAuth(ctx context.Context, secretArn string) (*AuthConfig, error) {
	// Load AWS SDK configuration
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	// Create a Secrets Manager client
	client := secretsmanager.NewFromConfig(cfg)

	// Get the secret value
	result, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretArn),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to get secret value: %w", err)
	}

	// Get the secret string
	var secretString string
	if result.SecretString != nil {
		secretString = *result.SecretString
	} else if result.SecretBinary != nil {
		decodedBinarySecretBytes := make([]byte, base64.StdEncoding.DecodedLen(len(result.SecretBinary)))
		len, err := base64.StdEncoding.Decode(decodedBinarySecretBytes, result.SecretBinary)
		if err != nil {
			return nil, fmt.Errorf("unable to decode binary secret data: %w", err)
		}
		secretString = string(decodedBinarySecretBytes[:len])
	}

	// Parse the secret string as username:password
	return r.parseCredentialsString(ctx, secretString)
}

// getGoogleSecretManagerAuth retrieves authentication information from Google Secret Manager
func (r *ImageResource) getGoogleSecretManagerAuth(ctx context.Context, secretResource string) (*AuthConfig, error) {
	// Create the Secret Manager client
	client, err := secretmanager.NewClient(ctx, option.WithUserAgent("terraform-provider-containerregistry"))
	if err != nil {
		return nil, fmt.Errorf("failed to create Secret Manager client: %w", err)
	}
	defer client.Close()

	// Build the request
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: secretResource,
	}

	// Call the API
	result, err := client.AccessSecretVersion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to access secret version: %w", err)
	}

	// Get the secret string
	secretData := result.Payload.Data
	secretString := string(secretData)

	// Parse the secret string as username:password
	return r.parseCredentialsString(ctx, secretString)
}

// parseCredentialsString parses a string in the format "username:password"
func (r *ImageResource) parseCredentialsString(ctx context.Context, credentialsString string) (*AuthConfig, error) {
	// Handle JSON format
	var jsonCreds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	err := json.Unmarshal([]byte(credentialsString), &jsonCreds)
	if err == nil && jsonCreds.Username != "" && jsonCreds.Password != "" {
		return &AuthConfig{
			Username: jsonCreds.Username,
			Password: jsonCreds.Password,
		}, nil
	}

	// Handle simple username:password format
	parts := strings.SplitN(credentialsString, ":", 2)
	if len(parts) == 2 {
		username := parts[0]
		password := parts[1]

		if username != "" && password != "" {
			return &AuthConfig{
				Username: username,
				Password: password,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid credentials format: expected JSON with username/password or string with format 'username:password'")
}

// GetEncodedAuthConfig converts the AuthConfig to a base64 encoded string for Docker API
func (r *ImageResource) GetEncodedAuthConfig(ctx context.Context, authConfig *AuthConfig) (string, error) {
	if authConfig == nil {
		return "", nil
	}

	// Create Docker registry auth config
	dockerAuthConfig := registry.AuthConfig{
		Username: authConfig.Username,
		Password: authConfig.Password,
	}

	// Convert to JSON
	encodedJSON, err := json.Marshal(dockerAuthConfig)
	if err != nil {
		return "", fmt.Errorf("unable to encode auth config: %w", err)
	}

	// Base64 encode the JSON
	return base64.URLEncoding.EncodeToString(encodedJSON), nil
}

// GetHTTPAuthHeader returns an HTTP Authorization header value for registry API requests
func (r *ImageResource) GetHTTPAuthHeader(ctx context.Context, authConfig *AuthConfig) string {
	if authConfig == nil {
		return ""
	}

	// For basic auth, create a Basic auth header
	auth := fmt.Sprintf("%s:%s", authConfig.Username, authConfig.Password)
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}
