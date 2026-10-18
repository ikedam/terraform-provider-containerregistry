package buildx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const ephemeralBuilderNamePrefix = "tf-cr-ephemeral-"

// EphemeralBuilderConfig describes how to create a temporary buildx builder.
type EphemeralBuilderConfig struct {
	Driver     string
	DriverOpts []string
}

// NewEphemeralBuilderName returns a unique builder name for parallel-safe create/rm.
func NewEphemeralBuilderName() string {
	return ephemeralBuilderNamePrefix + uuid.NewString()
}

// CreateEphemeralBuilder runs `docker buildx create` with --bootstrap and without --use
// so parallel builds do not race on the global current builder.
func CreateEphemeralBuilder(ctx context.Context, name string, cfg EphemeralBuilderConfig) error {
	driver := cfg.Driver
	if driver == "" {
		return fmt.Errorf("builder driver is required for ephemeral builder %q", name)
	}

	args := []string{
		"buildx", "create",
		"--name", name,
		"--driver", driver,
		"--bootstrap",
	}
	for _, opt := range cfg.DriverOpts {
		if opt == "" {
			continue
		}
		args = append(args, "--driver-opt", opt)
	}

	tflog.Info(ctx, "Creating ephemeral buildx builder", map[string]interface{}{
		"name":   name,
		"driver": driver,
		"opts":   cfg.DriverOpts,
	})

	out, err := runDocker(ctx, args...)
	if err != nil {
		return fmt.Errorf("docker buildx create %q failed: %w%s", name, err, formatCmdOutput(out))
	}

	tflog.Info(ctx, "Created ephemeral buildx builder", map[string]interface{}{
		"name":   name,
		"output": strings.TrimSpace(out),
	})
	return nil
}

// RemoveEphemeralBuilder runs `docker buildx rm -f` to discard the builder and its cache volume.
func RemoveEphemeralBuilder(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}

	tflog.Info(ctx, "Removing ephemeral buildx builder", map[string]interface{}{
		"name": name,
	})

	out, err := runDocker(ctx, "buildx", "rm", "-f", name)
	if err != nil {
		return fmt.Errorf("docker buildx rm %q failed: %w%s", name, err, formatCmdOutput(out))
	}

	tflog.Info(ctx, "Removed ephemeral buildx builder", map[string]interface{}{
		"name": name,
	})
	return nil
}

func runDocker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func formatCmdOutput(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	return ": " + out
}
