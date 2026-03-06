package compose

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ImageResourceModel describes the image resource data model.
// This is based on the schema defined in README.md
// AuthModel represents the authentication configurations
type AuthModel struct {
	AWSECR                 *AWSECRModel                 `tfsdk:"aws_ecr"`
	GoogleArtifactRegistry *GoogleArtifactRegistryModel `tfsdk:"google_artifact_registry"`
	UsernamePassword       *UsernamePasswordModel       `tfsdk:"username_password"`
}

// AWSECRModel represents AWS ECR authentication configuration
type AWSECRModel struct {
	Profile types.String `tfsdk:"profile"`
}

// GoogleArtifactRegistryModel represents Google Artifact Registry authentication configuration
type GoogleArtifactRegistryModel struct {
	// No additional fields required as it uses application default credentials
}

// UsernamePasswordModel represents username/password authentication configuration
type UsernamePasswordModel struct {
	Username            types.String `tfsdk:"username"`
	Password            types.String `tfsdk:"password"`
	AwsSecretsManager   types.String `tfsdk:"aws_secrets_manager"`
	GoogleSecretManager types.String `tfsdk:"google_secret_manager"`
}

type OptionModel struct {
	Pull     types.Bool   `tfsdk:"pull"`
	NoCache  types.Bool   `tfsdk:"no_cache"`
	Progress types.String `tfsdk:"progress"`
}

// BuildLogModel represents build log output configuration
type BuildLogModel struct {
	Timestamp types.Bool   `tfsdk:"timestamp"`
	Lines     types.Int64  `tfsdk:"lines"`
	Log       types.String `tfsdk:"log"`
}

type ComposeResourceModel struct {
	ID           types.String   `tfsdk:"id"`
	ImageURI     types.String   `tfsdk:"image_uri"`
	Build        types.String   `tfsdk:"build"`
	Labels       types.Map      `tfsdk:"labels"`
	Triggers     types.Map      `tfsdk:"triggers"`
	DeleteImage  types.Bool     `tfsdk:"delete_image"`
	Option       *OptionModel   `tfsdk:"option"`
	BuildLog     *BuildLogModel `tfsdk:"buildlog"`
	Auth         *AuthModel     `tfsdk:"auth"`
	SHA256Digest types.String   `tfsdk:"sha256_digest"`
}
