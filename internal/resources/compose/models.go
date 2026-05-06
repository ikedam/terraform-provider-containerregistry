package compose

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type OptionModel struct {
	Pull     types.Bool   `tfsdk:"pull"`
	NoCache  types.Bool   `tfsdk:"no_cache"`
	Progress types.String `tfsdk:"progress"`
}

// BuildLogModel represents build log output configuration
type BuildLogModel struct {
	Timestamp types.Bool   `tfsdk:"timestamp"`
	Lines     types.Int64  `tfsdk:"lines"`
	Log       types.String `tfsdk:"log"`
}

type ComposeResourceModel struct {
	ID           types.String   `tfsdk:"id"`
	ImageURI     types.String   `tfsdk:"image_uri"`
	Build        types.String   `tfsdk:"build"`
	Labels       types.Map      `tfsdk:"labels"`
	Triggers     types.Map      `tfsdk:"triggers"`
	DeleteImage  types.Bool     `tfsdk:"delete_image"`
	Option       *OptionModel   `tfsdk:"option"`
	BuildLog     *BuildLogModel `tfsdk:"buildlog"`
	SHA256Digest types.String   `tfsdk:"sha256_digest"`
}
