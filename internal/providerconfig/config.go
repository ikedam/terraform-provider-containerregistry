package providerconfig

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// DefaultEphemeralBuilderDriver is used when ephemeral_builder is enabled and
// builder_driver is unset. docker-container is required for registry cache export.
const DefaultEphemeralBuilderDriver = "docker-container"

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
	// EphemeralBuilder when true, creates a temporary buildx builder per build and removes it after.
	EphemeralBuilder bool
	// BuilderDriver is the buildx driver for ephemeral builders (e.g. "docker-container").
	// Empty means DefaultEphemeralBuilderDriver when EphemeralBuilder is enabled.
	BuilderDriver string
	// BuilderDriverOpts are passed to `docker buildx create --driver-opt`.
	BuilderDriverOpts []string
}

// RegistryAuthCredentials is username/password for a single registry host.
type RegistryAuthCredentials struct {
	Username string
	Password string
}

// EphemeralBuilderOverride is an optional per-resource override of provider ephemeral builder settings.
// Pointer fields mean "use provider value".
type EphemeralBuilderOverride struct {
	// Enabled overrides EphemeralBuilder when non-nil.
	Enabled *bool
	// Driver overrides BuilderDriver when non-nil (including empty string).
	Driver *string
	// DriverOpts overrides BuilderDriverOpts when non-nil (including empty slice).
	DriverOpts *[]string
}

// ResolvedEphemeralBuilder is the effective ephemeral builder setting for one build.
type ResolvedEphemeralBuilder struct {
	Enabled    bool
	Driver     string
	DriverOpts []string
}

// ResolveEphemeralBuilder merges provider defaults with an optional resource override.
// When enabled and driver is empty, Driver defaults to DefaultEphemeralBuilderDriver.
func (c *Config) ResolveEphemeralBuilder(override *EphemeralBuilderOverride) ResolvedEphemeralBuilder {
	resolved := ResolvedEphemeralBuilder{}
	if c != nil {
		resolved.Enabled = c.EphemeralBuilder
		resolved.Driver = c.BuilderDriver
		resolved.DriverOpts = append([]string(nil), c.BuilderDriverOpts...)
	}
	if override != nil {
		if override.Enabled != nil {
			resolved.Enabled = *override.Enabled
		}
		if override.Driver != nil {
			resolved.Driver = *override.Driver
		}
		if override.DriverOpts != nil {
			resolved.DriverOpts = append([]string(nil), (*override.DriverOpts)...)
		}
	}
	if resolved.Enabled && resolved.Driver == "" {
		resolved.Driver = DefaultEphemeralBuilderDriver
	}
	return resolved
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
