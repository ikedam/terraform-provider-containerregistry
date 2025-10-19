package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// parseBuildSpec parses the build specification from the model
func (r *ComposeResource) parseBuildSpec(ctx context.Context, model *ComposeResourceModel) (map[string]interface{}, error) {
	// The build attribute contains a Docker Compose compatible build specification in JSON format
	buildJSON := model.Build.ValueString()
	if buildJSON == "" {
		return nil, errors.New("build specification is empty")
	}

	// Parse the JSON into a map
	var buildConfig map[string]interface{}
	err := json.Unmarshal([]byte(buildJSON), &buildConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON in build specification: %w", err)
	}

	return buildConfig, nil
}

// Docker Compose APIを使ってのビルドはdocker_push.goに実装されています

// これらの関数はDocker Compose APIを使用するようになったため不要になりました

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
