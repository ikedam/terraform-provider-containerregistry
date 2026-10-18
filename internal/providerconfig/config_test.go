package providerconfig

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"
)

func boolPtr(v bool) *bool       { return &v }
func stringPtr(v string) *string { return &v }

func TestResolveEphemeralBuilder_Defaults(t *testing.T) {
	c := &Config{}
	got := c.ResolveEphemeralBuilder(nil)
	if got.Enabled {
		t.Fatal("expected ephemeral disabled by default")
	}
	if got.Driver != "" {
		t.Fatalf("driver = %q, want empty when disabled", got.Driver)
	}
}

func TestResolveEphemeralBuilder_ProviderEnabledDefaultsDriver(t *testing.T) {
	c := &Config{EphemeralBuilder: true}
	got := c.ResolveEphemeralBuilder(nil)
	if !got.Enabled {
		t.Fatal("expected ephemeral enabled")
	}
	if got.Driver != DefaultEphemeralBuilderDriver {
		t.Fatalf("driver = %q, want %q", got.Driver, DefaultEphemeralBuilderDriver)
	}
}

func TestResolveEphemeralBuilder_ResourceOverride(t *testing.T) {
	c := &Config{
		EphemeralBuilder:  false,
		BuilderDriver:     "docker",
		BuilderDriverOpts: []string{"network=host"},
	}
	opts := []string{"image=moby/buildkit:latest"}
	got := c.ResolveEphemeralBuilder(&EphemeralBuilderOverride{
		Enabled:    boolPtr(true),
		Driver:     stringPtr("docker-container"),
		DriverOpts: &opts,
	})
	if !got.Enabled {
		t.Fatal("expected resource to enable ephemeral")
	}
	if got.Driver != "docker-container" {
		t.Fatalf("driver = %q", got.Driver)
	}
	if len(got.DriverOpts) != 1 || got.DriverOpts[0] != "image=moby/buildkit:latest" {
		t.Fatalf("driver opts = %#v", got.DriverOpts)
	}
}

func TestResolveEphemeralBuilder_NilConfig(t *testing.T) {
	var c *Config
	got := c.ResolveEphemeralBuilder(&EphemeralBuilderOverride{Enabled: boolPtr(true)})
	if !got.Enabled {
		t.Fatal("expected enabled from override")
	}
	if got.Driver != DefaultEphemeralBuilderDriver {
		t.Fatalf("driver = %q, want default", got.Driver)
	}
}

func TestAcquireBuildSlot_NilConfig(t *testing.T) {
	var c *Config
	if err := c.AcquireBuildSlot(context.Background()); err != nil {
		t.Fatalf("nil config AcquireBuildSlot: %v", err)
	}
	c.ReleaseBuildSlot()
}

func TestAcquireBuildSlot_Unlimited(t *testing.T) {
	c := &Config{}
	if err := c.AcquireBuildSlot(context.Background()); err != nil {
		t.Fatalf("unlimited AcquireBuildSlot: %v", err)
	}
	c.ReleaseBuildSlot()
}

func TestAcquireBuildSlot_LimitsConcurrency(t *testing.T) {
	c := &Config{BuildSemaphore: semaphore.NewWeighted(1)}

	if err := c.AcquireBuildSlot(context.Background()); err != nil {
		t.Fatalf("first AcquireBuildSlot: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.AcquireBuildSlot(ctx); err == nil {
		t.Fatal("expected second AcquireBuildSlot to block and fail on timeout")
	}

	c.ReleaseBuildSlot()

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := c.AcquireBuildSlot(ctx2); err != nil {
		t.Fatalf("AcquireBuildSlot after release: %v", err)
	}
	c.ReleaseBuildSlot()
}
