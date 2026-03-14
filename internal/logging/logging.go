package logging

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
)

// HTTPLoggingSubsystemName is the tflog subsystem name used for HTTP logging.
const HTTPLoggingSubsystemName = "containerregistry"

// WithHTTPLoggingSubsystem initializes the tflog subsystem used for HTTP
// logging and configures masking of sensitive HTTP headers for all
// downstream HTTP calls that use this context.
func WithHTTPLoggingSubsystem(ctx context.Context) context.Context {
	ctx = tflog.NewSubsystem(ctx, HTTPLoggingSubsystemName)
	ctx = tflog.SubsystemMaskFieldValuesWithFieldKeys(ctx, HTTPLoggingSubsystemName, "Authorization")
	ctx = tflog.SubsystemMaskFieldValuesWithFieldKeys(ctx, HTTPLoggingSubsystemName, "Proxy-Authorization")
	return ctx
}

// NewHTTPLoggingClient returns an *http.Client whose Transport is wrapped by
// NewSubsystemLoggingHTTPTransport so that HTTP requests/responses are logged
// via tflog for the containerregistry subsystem. Use this when making HTTP
// requests that should be traceable (e.g. buildx plugin download).
func NewHTTPLoggingClient() *http.Client {
	transport := InjectLoggingToTransport(http.DefaultTransport)
	return &http.Client{Transport: transport}
}

// InjectLoggingToTransport wraps the given RoundTripper with subsystem HTTP logging.
func InjectLoggingToTransport(transport http.RoundTripper) http.RoundTripper {
	return logging.NewSubsystemLoggingHTTPTransport(HTTPLoggingSubsystemName, transport)
}
