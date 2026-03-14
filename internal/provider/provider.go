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

	resp.ResourceData = &providerconfig.Config{
		BuildxInstallIfMissing: installIfMissing,
		BuildxVersion:          version,
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
