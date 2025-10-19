package compose

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
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/docker/docker/api/types/registry"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/oauth2/google"
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
func (r *ComposeResource) getAuthConfig(ctx context.Context, model *ComposeResourceModel) (*AuthConfig, error) {
	// If no authentication is provided, return nil
	if model.Auth == nil {
		tflog.Debug(ctx, "No authentication configuration provided")
		return nil, nil
	}

	// Check for username/password authentication
	if model.Auth.UsernamePassword != nil {
		authMap := make(map[string]interface{})

		// Extract username if it exists
		if !model.Auth.UsernamePassword.Username.IsNull() && !model.Auth.UsernamePassword.Username.IsUnknown() {
			authMap["username"] = model.Auth.UsernamePassword.Username.ValueString()
		}

		// Extract password if it exists
		if !model.Auth.UsernamePassword.Password.IsNull() && !model.Auth.UsernamePassword.Password.IsUnknown() {
			authMap["password"] = model.Auth.UsernamePassword.Password.ValueString()
		}

		// Extract AWS Secrets Manager ARN if it exists
		if !model.Auth.UsernamePassword.AwsSecretsManager.IsNull() && !model.Auth.UsernamePassword.AwsSecretsManager.IsUnknown() {
			authMap["aws_secrets_manager"] = model.Auth.UsernamePassword.AwsSecretsManager.ValueString()
		}

		// Extract Google Secret Manager resource if it exists
		if !model.Auth.UsernamePassword.GoogleSecretManager.IsNull() && !model.Auth.UsernamePassword.GoogleSecretManager.IsUnknown() {
			authMap["google_secret_manager"] = model.Auth.UsernamePassword.GoogleSecretManager.ValueString()
		}

		return r.getUsernamePasswordAuth(ctx, authMap)
	}

	// Check for AWS ECR authentication
	if model.Auth.AWSECR != nil {
		authMap := make(map[string]interface{})

		// Extract profile if it exists
		if !model.Auth.AWSECR.Profile.IsNull() && !model.Auth.AWSECR.Profile.IsUnknown() {
			authMap["profile"] = model.Auth.AWSECR.Profile.ValueString()
		}

		return r.getAWSECRAuth(ctx, authMap, model.ImageURI.ValueString())
	}

	// Check for Google Cloud Artifact Registry authentication
	if model.Auth.GoogleArtifactRegistry != nil {
		// No additional fields needed for Google Artifact Registry
		authMap := make(map[string]interface{})
		return r.getGoogleArtifactRegistryAuth(ctx, authMap, model.ImageURI.ValueString())
	}

	// No authentication method found
	tflog.Debug(ctx, "No supported authentication method found")
	return nil, nil
}

// getUsernamePasswordAuth extracts username and password from the auth configuration
func (r *ComposeResource) getUsernamePasswordAuth(ctx context.Context, authMap map[string]interface{}) (*AuthConfig, error) {
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
func (r *ComposeResource) getAWSSecretsManagerAuth(ctx context.Context, secretArn string) (*AuthConfig, error) {
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
func (r *ComposeResource) getGoogleSecretManagerAuth(ctx context.Context, secretResource string) (*AuthConfig, error) {
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
func (r *ComposeResource) parseCredentialsString(ctx context.Context, credentialsString string) (*AuthConfig, error) {
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
func (r *ComposeResource) GetEncodedAuthConfig(ctx context.Context, authConfig *AuthConfig) (string, error) {
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
func (r *ComposeResource) GetHTTPAuthHeader(ctx context.Context, authConfig *AuthConfig) string {
	if authConfig == nil {
		return ""
	}

	// For basic auth, create a Basic auth header
	auth := fmt.Sprintf("%s:%s", authConfig.Username, authConfig.Password)
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

// getAWSECRAuth retrieves an authentication token from AWS ECR
func (r *ComposeResource) getAWSECRAuth(ctx context.Context, authMap map[string]interface{}, imageURI string) (*AuthConfig, error) {
	tflog.Debug(ctx, "Getting AWS ECR authentication token", map[string]interface{}{
		"image_uri": imageURI,
	})

	// Get the profile name if specified
	var profile string
	if profileVal, ok := authMap["profile"].(string); ok && profileVal != "" {
		profile = profileVal
	}

	// Extract registry domain from image URI
	// Format: registry-domain/repository:tag
	registryDomain := strings.Split(imageURI, "/")[0]
	tflog.Debug(ctx, "Extracted registry domain", map[string]interface{}{
		"registry_domain": registryDomain,
	})

	// Load AWS SDK configuration
	var cfg aws.Config
	var err error

	if profile != "" {
		// Use specified profile
		tflog.Debug(ctx, "Loading AWS config with profile", map[string]interface{}{
			"profile": profile,
		})
		cfg, err = config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	} else {
		// Use default profile
		tflog.Debug(ctx, "Loading AWS config with default profile")
		cfg, err = config.LoadDefaultConfig(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	// Create an ECR client
	client := ecr.NewFromConfig(cfg)

	// Call the ECR API to get an authorization token
	output, err := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to get ECR authorization token: %w", err)
	}

	// Check if we got any auth data
	if len(output.AuthorizationData) == 0 {
		return nil, fmt.Errorf("no authorization data received from ECR")
	}

	// Get the first auth data (we only need one)
	authData := output.AuthorizationData[0]

	// Decode the authorization token (which is in base64 format)
	decodedToken, err := base64.StdEncoding.DecodeString(*authData.AuthorizationToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ECR authorization token: %w", err)
	}

	// The token is in the format "username:password"
	authConfig, err := r.parseCredentialsString(ctx, string(decodedToken))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ECR credentials: %w", err)
	}

	tflog.Debug(ctx, "Successfully retrieved ECR authentication token")
	return authConfig, nil
}

// getGoogleArtifactRegistryAuth retrieves an authentication token for Google Cloud Artifact Registry
func (r *ComposeResource) getGoogleArtifactRegistryAuth(ctx context.Context, authMap map[string]interface{}, imageURI string) (*AuthConfig, error) {
	tflog.Debug(ctx, "Getting Google Cloud Artifact Registry authentication token", map[string]interface{}{
		"image_uri": imageURI,
	})

	tflog.Debug(ctx, "Using application default credentials")

	// Create the token source from application default credentials
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to find default credentials: %w", err)
	}
	tokenSource := creds.TokenSource

	// Get the token
	token, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Use the access token for authentication
	// For Artifact Registry, we use "oauth2accesstoken" as username and the access token as password
	// as per https://cloud.google.com/artifact-registry/docs/docker/authentication#token
	authConfig := &AuthConfig{
		Username: "oauth2accesstoken",
		Password: token.AccessToken,
	}

	tflog.Debug(ctx, "Successfully retrieved Google Cloud Artifact Registry authentication token")
	return authConfig, nil
}
