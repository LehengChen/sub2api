package service

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Deployment control modes describe who is allowed to mutate the running
// application artifact. The default remains self_managed for local and
// standalone installations; production operators should set
// SUB2API_DEPLOYMENT_CONTROL_MODE=externally_managed.
const (
	DeploymentModeSelfManaged       = "self_managed"
	DeploymentModeExternallyManaged = "externally_managed"
)

const (
	deploymentControlModeEnv = "SUB2API_DEPLOYMENT_CONTROL_MODE"
	catalogSourceEnv         = "SUB2API_RELEASE_CATALOG_SOURCE"
	catalogRevisionEnv       = "SUB2API_RELEASE_CATALOG_REVISION"
	catalogVersionEnv        = "SUB2API_RELEASE_CATALOG_VERSION"
	appTagEnv                = "SUB2API_RELEASE_APP_TAG"
	sourceRepositoryEnv      = "SUB2API_RELEASE_SOURCE_REPOSITORY"
	sourceRevisionEnv        = "SUB2API_RELEASE_SOURCE_REVISION"
	imageTagEnv              = "SUB2API_RELEASE_IMAGE_TAG"
	imageDigestEnv           = "SUB2API_RELEASE_IMAGE_DIGEST"
	opsRevisionEnv           = "SUB2API_RELEASE_OPS_REVISION"
)

var (
	// Release metadata is deliberately restrictive. It is displayed to admins
	// and must never become an accidental channel for arbitrary environment data.
	metadataTokenPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/@+:-]{0,255}$`)
	hexRevisionPattern   = regexp.MustCompile(`^[0-9a-f]{40,64}$`)
	digestPattern        = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ErrExternallyManaged is returned for every in-process artifact mutation when
// deployment is controlled by the private release pipeline.
var ErrExternallyManaged = infraerrors.Forbidden(
	"EXTERNAL_DEPLOYMENT_MANAGED",
	"application updates, rollbacks, and restarts are managed by external operations",
)

// ReleaseCatalog is the non-secret, immutable release identity supplied by
// the deployment controller. Version is the approved catalog candidate (it is
// not fetched from an upstream "latest" endpoint).
type ReleaseCatalog struct {
	Source           string `json:"source,omitempty"`
	Revision         string `json:"revision,omitempty"`
	Version          string `json:"version,omitempty"`
	AppTag           string `json:"app_tag,omitempty"`
	SourceRepository string `json:"source_repository,omitempty"`
	SourceRevision   string `json:"source_revision,omitempty"`
	ImageTag         string `json:"image_tag,omitempty"`
	ImageDigest      string `json:"image_digest,omitempty"`
	OpsRevision      string `json:"ops_revision,omitempty"`
}

// DeploymentControl is injected from non-secret runtime environment. It is
// intentionally separate from the business config so a deployment policy
// cannot be changed through the admin settings UI or database.
type DeploymentControl struct {
	Mode    string
	Catalog ReleaseCatalog
}

// UpdateCapabilities communicates which operations are available to the
// console. CheckUpdates remains read-only in externally managed mode.
type UpdateCapabilities struct {
	CheckUpdates bool `json:"check_updates"`
	Update       bool `json:"update"`
	Rollback     bool `json:"rollback"`
	Restart      bool `json:"restart"`
}

// DefaultDeploymentControl returns the backwards-compatible local default.
func DefaultDeploymentControl() DeploymentControl {
	return DeploymentControl{Mode: DeploymentModeSelfManaged}
}

// LoadDeploymentControlFromEnv loads only non-secret release identity and
// control flags. An invalid mode or malformed identity fails startup rather
// than silently enabling a less restrictive deployment path.
func LoadDeploymentControlFromEnv() (DeploymentControl, error) {
	control := DefaultDeploymentControl()
	if mode := strings.TrimSpace(os.Getenv(deploymentControlModeEnv)); mode != "" {
		control.Mode = strings.ToLower(mode)
	}
	if control.Mode != DeploymentModeSelfManaged && control.Mode != DeploymentModeExternallyManaged {
		return DeploymentControl{}, fmt.Errorf("%s must be %q or %q", deploymentControlModeEnv, DeploymentModeSelfManaged, DeploymentModeExternallyManaged)
	}

	control.Catalog = ReleaseCatalog{
		Source:           strings.TrimSpace(os.Getenv(catalogSourceEnv)),
		Revision:         strings.TrimSpace(os.Getenv(catalogRevisionEnv)),
		Version:          strings.TrimSpace(os.Getenv(catalogVersionEnv)),
		AppTag:           strings.TrimSpace(os.Getenv(appTagEnv)),
		SourceRepository: strings.TrimSpace(os.Getenv(sourceRepositoryEnv)),
		SourceRevision:   strings.TrimSpace(os.Getenv(sourceRevisionEnv)),
		ImageTag:         strings.TrimSpace(os.Getenv(imageTagEnv)),
		ImageDigest:      strings.TrimSpace(os.Getenv(imageDigestEnv)),
		OpsRevision:      strings.TrimSpace(os.Getenv(opsRevisionEnv)),
	}

	if err := control.Catalog.ValidateMetadata(); err != nil {
		return DeploymentControl{}, err
	}
	return control, nil
}

// IsExternallyManaged reports whether the private release pipeline owns all
// artifact mutations.
func (d DeploymentControl) IsExternallyManaged() bool {
	return d.Mode == DeploymentModeExternallyManaged
}

// Capabilities returns the operation set exposed to the admin console.
func (d DeploymentControl) Capabilities() UpdateCapabilities {
	return UpdateCapabilities{
		CheckUpdates: true,
		Update:       !d.IsExternallyManaged(),
		Rollback:     !d.IsExternallyManaged(),
		Restart:      !d.IsExternallyManaged(),
	}
}

// MissingFields lists the catalog fields required to identify an approved
// release. It is intentionally non-sensitive and suitable for an admin warning.
func (c ReleaseCatalog) MissingFields() []string {
	required := []struct {
		name  string
		value string
	}{
		{"source", c.Source},
		{"revision", c.Revision},
		{"version", c.Version},
		{"app_tag", c.AppTag},
		{"source_repository", c.SourceRepository},
		{"source_revision", c.SourceRevision},
		{"image_tag", c.ImageTag},
		{"image_digest", c.ImageDigest},
		{"ops_revision", c.OpsRevision},
	}

	missing := make([]string, 0)
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			missing = append(missing, field.name)
		}
	}
	return missing
}

// ValidateMetadata validates supplied values but allows an incomplete catalog
// during a staged rollout. Completeness is surfaced by MissingFields and must
// make the UI show a warning; it must never enable mutation operations.
func (c ReleaseCatalog) ValidateMetadata() error {
	for name, value := range map[string]string{
		"source":            c.Source,
		"revision":          c.Revision,
		"version":           c.Version,
		"app_tag":           c.AppTag,
		"source_repository": c.SourceRepository,
		"image_tag":         c.ImageTag,
		"ops_revision":      c.OpsRevision,
	} {
		if value == "" {
			continue
		}
		if len(value) > 256 || !metadataTokenPattern.MatchString(value) {
			return fmt.Errorf("release catalog %s contains invalid characters", name)
		}
	}
	if c.SourceRevision != "" && !hexRevisionPattern.MatchString(c.SourceRevision) {
		return fmt.Errorf("release catalog source_revision must be a full hexadecimal commit SHA")
	}
	if c.ImageDigest != "" && !digestPattern.MatchString(c.ImageDigest) {
		return fmt.Errorf("release catalog image_digest must be a sha256 digest")
	}
	return nil
}

// Complete reports whether all release identity fields are present and valid.
func (c ReleaseCatalog) Complete() bool {
	return len(c.MissingFields()) == 0 && c.ValidateMetadata() == nil
}
