package providerconfig

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"
)

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
