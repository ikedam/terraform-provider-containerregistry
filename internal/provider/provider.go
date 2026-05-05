package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ikedam/terraform-provider-containerregistry/internal/providerconfig"
	"github.com/ikedam/terraform-provider-containerregistry/internal/resources/compose"
)

// Ensure the implementation satisfies the provider.Provider interface.
var _ provider.Provider = &ContainerRegistryProvider{}

// ContainerRegistryProvider defines the provider implementation.
type ContainerRegistryProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// ContainerRegistryProviderModel describes the provider data model.
type ContainerRegistryProviderModel struct {
	BuildxInstallIfMissing types.Bool   `tfsdk:"buildx_install_if_missing"`
	BuildxVersion          types.String `tfsdk:"buildx_version"`
	RegistryAuth           types.Map    `tfsdk:"registry_auth"`
}

type RegistryAuthEntryModel struct {
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

// New returns a function that initializes a provider.Provider.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ContainerRegistryProvider{
			version: version,
		}
	}
}

// Metadata returns the provider type name.
func (p *ContainerRegistryProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "containerregistry"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *ContainerRegistryProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"buildx_install_if_missing": schema.BoolAttribute{
				MarkdownDescription: "When true, the buildx plugin is installed automatically when not found. " +
					"This allows Compose v5 builds to use BuildKit without requiring a pre-installed buildx. Default is false.",
				Optional: true,
			},
			"buildx_version": schema.StringAttribute{
				MarkdownDescription: "Buildx version to install when buildx_install_if_missing is true (e.g. `v0.12.0`). " +
					"Empty or omit for latest. Ignored when buildx is already present.",
				Optional: true,
			},
			"registry_auth": schema.MapNestedAttribute{
				MarkdownDescription: "Per-registry Docker Registry HTTP Basic credentials. " +
					"Keys must be the registry hostname from `image_uri` (e.g. `asia-northeast1-docker.pkg.dev`, `123456789012.dkr.ecr.ap-northeast-1.amazonaws.com`). " +
					"Resources match this key to the hostname part of `image_uri`.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"username": schema.StringAttribute{
							MarkdownDescription: "Registry username (e.g. AWS ECR user from aws_ecr_authorization_token, or `oauth2accesstoken` for Google Artifact Registry with access token).",
							Required:            true,
						},
						"password": schema.StringAttribute{
							MarkdownDescription: "Registry password or token.",
							Required:            true,
							Sensitive:           true,
						},
					},
				},
			},
		},
	}
}

// Configure prepares a containerregistry API client for resources and data sources.
func (p *ContainerRegistryProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data ContainerRegistryProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Apply defaults for provider-level options (framework does not support Default on provider attributes)
	installIfMissing := false
	if !data.BuildxInstallIfMissing.IsNull() {
		installIfMissing = data.BuildxInstallIfMissing.ValueBool()
	}
	version := ""
	if !data.BuildxVersion.IsNull() {
		version = data.BuildxVersion.ValueString()
	}

	registryAuth := map[string]providerconfig.RegistryAuthCredentials{}
	if !data.RegistryAuth.IsNull() && !data.RegistryAuth.IsUnknown() {
		var entries map[string]RegistryAuthEntryModel
		resp.Diagnostics.Append(data.RegistryAuth.ElementsAs(ctx, &entries, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for host, e := range entries {
			if e.Username.IsNull() || e.Username.IsUnknown() || e.Password.IsNull() || e.Password.IsUnknown() {
				resp.Diagnostics.AddError(
					"Invalid registry_auth entry",
					"Each registry_auth value must include username and password.",
				)
				return
			}
			registryAuth[host] = providerconfig.RegistryAuthCredentials{
				Username: e.Username.ValueString(),
				Password: e.Password.ValueString(),
			}
		}
	}

	resp.ResourceData = &providerconfig.Config{
		BuildxInstallIfMissing: installIfMissing,
		BuildxVersion:          version,
		RegistryAuth:           registryAuth,
	}
}

// Resources defines the resources implemented in the provider.
func (p *ContainerRegistryProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		compose.NewComposeResource,
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *ContainerRegistryProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		// もしデータソースがあれば登録します
	}
}
