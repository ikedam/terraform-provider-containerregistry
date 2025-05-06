package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/ikedam/terraform-provider-containerregistry/internal/resources/image"
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
	// プロバイダーの設定項目があればここに定義します
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
			// プロバイダー設定項目があればここで定義します
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

	// ここでクライアントの初期化など設定が必要な場合は行います
	// 何もない場合は空のままでOK
}

// Resources defines the resources implemented in the provider.
func (p *ContainerRegistryProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		image.NewImageResource,
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *ContainerRegistryProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		// もしデータソースがあれば登録します
	}
}
