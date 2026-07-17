// Package version holds build metadata and the base image tag.
package version

// Version is the sandbox-cli release version. Overridable at build time via
// -ldflags "-X github.com/amitghadge/sandbox-cli/internal/version.Version=x.y.z".
var Version = "0.0.1.beta.1"

// baseImageVersion is the tag of the base image. Bumping it invalidates any
// previously built local image and triggers a rebuild.
const baseImageVersion = "0.1.1"

// BaseImage returns the fully-qualified default sandbox image reference.
func BaseImage() string {
	return "sandbox-base:" + baseImageVersion
}
