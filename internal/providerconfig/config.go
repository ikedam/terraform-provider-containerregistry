package providerconfig

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// Config holds provider-level configuration passed to resources via ConfigureResponse.ResourceData.
type Config struct {
	// BuildxInstallIfMissing when true, installs the buildx plugin when not found.
	BuildxInstallIfMissing bool
	// BuildxVersion is the buildx version to install (e.g. "v0.12.0"). Empty means latest.
	BuildxVersion string
	// RegistryAuth maps registry hostname (e.g. asia-northeast1-docker.pkg.dev) to credentials.
	// Used by resources when pushing/pulling or calling the Registry HTTP API for that host.
	RegistryAuth map[string]RegistryAuthCredentials
	// BuildSemaphore limits concurrent image builds across resources. Nil means unlimited.
	BuildSemaphore *semaphore.Weighted
}

// RegistryAuthCredentials is username/password for a single registry host.
type RegistryAuthCredentials struct {
	Username string
	Password string
}

// AcquireBuildSlot waits for a build concurrency slot. No-op when BuildSemaphore is nil.
func (c *Config) AcquireBuildSlot(ctx context.Context) error {
	if c == nil || c.BuildSemaphore == nil {
		return nil
	}
	return c.BuildSemaphore.Acquire(ctx, 1)
}

// ReleaseBuildSlot releases a build concurrency slot. No-op when BuildSemaphore is nil.
func (c *Config) ReleaseBuildSlot() {
	if c == nil || c.BuildSemaphore == nil {
		return
	}
	c.BuildSemaphore.Release(1)
}
