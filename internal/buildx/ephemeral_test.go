package buildx

import (
	"strings"
	"testing"
)

func TestNewEphemeralBuilderName(t *testing.T) {
	a := NewEphemeralBuilderName()
	b := NewEphemeralBuilderName()
	if !strings.HasPrefix(a, ephemeralBuilderNamePrefix) {
		t.Fatalf("name %q missing prefix %q", a, ephemeralBuilderNamePrefix)
	}
	if a == b {
		t.Fatalf("expected unique names, got %q twice", a)
	}
}

func TestFormatCmdOutput(t *testing.T) {
	if got := formatCmdOutput(""); got != "" {
		t.Fatalf("empty output: got %q", got)
	}
	if got := formatCmdOutput("  boom\n"); got != ": boom" {
		t.Fatalf("formatCmdOutput = %q", got)
	}
}
