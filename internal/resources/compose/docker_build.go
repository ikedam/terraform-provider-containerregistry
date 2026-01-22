package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	composeinterp "github.com/compose-spec/compose-go/v2/interpolation"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/go-viper/mapstructure/v2"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// parseBuildSpec parses the build specification from the model
// This function mimics the behavior of docker compose's loader (loadYamlFile) by:
// 1. Parsing JSON to map[string]any
// 2. Performing variable interpolation (${VAR} expansion)
// 3. Using mapstructure to decode to BuildConfig (which calls DecodeMapstructure for args)
func (r *ComposeResource) parseBuildSpec(ctx context.Context, model *ComposeResourceModel) (*composetypes.BuildConfig, error) {
	// The build attribute contains a Docker Compose compatible build specification in JSON format
	buildJSON := model.Build.ValueString()
	if buildJSON == "" {
		return nil, errors.New("build specification is empty")
	}

	// Step 1: Parse JSON to map[string]any
	var raw map[string]any
	if err := json.Unmarshal([]byte(buildJSON), &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON in build specification: %w", err)
	}

	// Step 2: Perform variable interpolation (${VAR} expansion)
	// This uses os.LookupEnv by default to resolve environment variables
	interpolated, err := composeinterp.Interpolate(raw, composeinterp.Options{
		LookupValue: os.LookupEnv,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to interpolate variables in build specification: %w", err)
	}

	// Step 2.5: Resolve build args with nil values from environment variables
	// This mimics normalize.go's resolve function for build.args
	// It handles cases like {"KEY": null} or ["KEY"] by looking up KEY from environment
	// The JSON is expected to be a direct BuildConfig structure (not nested under "build")
	// Note: This is implemented in normalize.go in docker compose (not exported)
	if args, ok := interpolated["args"]; ok {
		resolvedArgs, _ := resolveBuildArgs(args, os.LookupEnv)
		interpolated["args"] = resolvedArgs
	}

	// Step 3: Use mapstructure to decode to BuildConfig
	// This ensures DecodeMapstructure is called for MappingWithEquals (args field),
	// which supports both array format (["KEY=VALUE"]) and map format ({"KEY": "VALUE"})
	// The decoderHook is required to call DecodeMapstructure method for custom types
	var buildConfig composetypes.BuildConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: decoderHook,
		Result:     &buildConfig,
		TagName:    "json",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create mapstructure decoder: %w", err)
	}

	if err := decoder.Decode(interpolated); err != nil {
		return nil, fmt.Errorf("failed to decode build specification: %w", err)
	}

	return &buildConfig, nil
}

// extractLabels extracts labels from the model
func (r *ComposeResource) extractLabels(model *ComposeResourceModel) map[string]string {
	labels := make(map[string]string)

	// Extract labels from the model if they exist
	if !model.Labels.IsNull() && !model.Labels.IsUnknown() {
		elements := model.Labels.Elements()
		for k, v := range elements {
			if strVal, ok := v.(types.String); ok {
				labels[k] = strVal.ValueString()
			}
		}
	}

	return labels
}

// resolveBuildArgs resolves build args with nil values from environment variables
// This mimics the behavior of normalize.go's resolve function for build.args
// It supports both array format (["KEY", "KEY2=VALUE2"]) and map format ({"KEY": null, "KEY2": "VALUE2"})
func resolveBuildArgs(args any, lookupEnv func(string) (string, bool)) (any, bool) {
	switch v := args.(type) {
	case []any:
		// Array format: ["KEY", "KEY2=VALUE2"]
		var resolved []any
		for _, val := range v {
			if str, ok := val.(string); ok {
				if !strings.Contains(str, "=") {
					// Key without value, look up from environment
					if envVal, ok := lookupEnv(str); ok {
						resolved = append(resolved, fmt.Sprintf("%s=%s", str, envVal))
					} else {
						// Keep as-is if not found in environment
						resolved = append(resolved, str)
					}
				} else {
					// Key=Value format, keep as-is
					resolved = append(resolved, str)
				}
			} else {
				resolved = append(resolved, val)
			}
		}
		return resolved, true
	case map[string]any:
		// Map format: {"KEY": null, "KEY2": "VALUE2"}
		resolved := make(map[string]any)
		for key, val := range v {
			if val == nil {
				// Key with nil value, look up from environment
				if envVal, ok := lookupEnv(key); ok {
					resolved[key] = envVal
				} else {
					// Keep as nil if not found in environment
					resolved[key] = nil
				}
			} else {
				// Key with value, keep as-is
				resolved[key] = val
			}
		}
		return resolved, true
	default:
		return args, false
	}
}
