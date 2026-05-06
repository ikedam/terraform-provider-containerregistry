package providerconfig

// Config holds provider-level configuration passed to resources via ConfigureResponse.ResourceData.
type Config struct {
	// BuildxInstallIfMissing when true, installs the buildx plugin when not found.
	BuildxInstallIfMissing bool
	// BuildxVersion is the buildx version to install (e.g. "v0.12.0"). Empty means latest.
	BuildxVersion string
	// RegistryAuth maps registry hostname (e.g. asia-northeast1-docker.pkg.dev) to credentials.
	// Used by resources when pushing/pulling or calling the Registry HTTP API for that host.
	RegistryAuth map[string]RegistryAuthCredentials
}

// RegistryAuthCredentials is username/password for a single registry host.
type RegistryAuthCredentials struct {
	Username string
	Password string
}
