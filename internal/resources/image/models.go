package image

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ImageResourceModel describes the image resource data model.
// This is based on the schema defined in README.md
type ImageResourceModel struct {
	ID           types.String `tfsdk:"id"`
	ImageURI     types.String `tfsdk:"image_uri"`
	Build        types.String `tfsdk:"build"`
	Labels       types.Map    `tfsdk:"labels"`
	Triggers     types.Map    `tfsdk:"triggers"`
	DeleteImage  types.Bool   `tfsdk:"delete_image"`
	Auth         types.Object `tfsdk:"auth"`
	SHA256Digest types.String `tfsdk:"sha256_digest"`
}
