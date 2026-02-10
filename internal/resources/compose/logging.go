package compose

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
)

// httpLoggingSubsystemName is the tflog subsystem name used for HTTP logging.
const httpLoggingSubsystemName = "containerregistry"

// withHTTPLoggingSubsystem initializes the tflog subsystem used for HTTP
// logging and configures masking of sensitive HTTP headers for all
// downstream HTTP calls that use this context.
func withHTTPLoggingSubsystem(ctx context.Context) context.Context {
	ctx = tflog.NewSubsystem(ctx, httpLoggingSubsystemName)
	ctx = tflog.SubsystemMaskFieldValuesWithFieldKeys(ctx, httpLoggingSubsystemName, "Authorization")
	ctx = tflog.SubsystemMaskFieldValuesWithFieldKeys(ctx, httpLoggingSubsystemName, "Proxy-Authorization")
	return ctx
}

// newHTTPLoggingClient returns an *http.Client whose Transport is wrapped by
// NewSubsystemLoggingHTTPTransport so that HTTP requests/responses are logged
// via tflog for the containerregistry subsystem.
func newHTTPLoggingClient() *http.Client {
	transport := injectLoggingToTransport(http.DefaultTransport)
	return &http.Client{Transport: transport}
}

func injectLoggingToTransport(transport http.RoundTripper) http.RoundTripper {
	return logging.NewSubsystemLoggingHTTPTransport(httpLoggingSubsystemName, transport)
}
