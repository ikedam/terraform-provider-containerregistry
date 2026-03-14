package providerconfig

// Config holds provider-level configuration passed to resources via ConfigureResponse.ResourceData.
type Config struct {
	// BuildxInstallIfMissing when true, installs the buildx plugin when not found.
	BuildxInstallIfMissing bool
	// BuildxVersion is the buildx version to install (e.g. "v0.12.0"). Empty means latest.
	BuildxVersion string
}
