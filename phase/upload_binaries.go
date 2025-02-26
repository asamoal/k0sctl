package phase

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/k0sproject/k0sctl/pkg/apis/k0sctl.k0sproject.io/v1beta1"
	"github.com/k0sproject/k0sctl/pkg/apis/k0sctl.k0sproject.io/v1beta1/cluster"
	"github.com/k0sproject/rig/exec"
	"github.com/k0sproject/version"
	log "github.com/sirupsen/logrus"
)

// UploadBinaries uploads k0s binaries from localhost to target
type UploadBinaries struct {
	GenericPhase
	hosts cluster.Hosts
}

// Title for the phase
func (p *UploadBinaries) Title() string {
	return "Upload k0s binaries to hosts"
}

// Prepare the phase
func (p *UploadBinaries) Prepare(config *v1beta1.Cluster) error {
	p.Config = config
	p.hosts = p.Config.Spec.Hosts.Filter(func(h *cluster.Host) bool {
		// Nothing to upload
		if h.UploadBinaryPath == "" {
			return false
		}

		// Nothing to upload
		if h.Reset {
			return false
		}

		// Upgrade is handled separately (k0s stopped, binary uploaded, k0s restarted)
		if h.Metadata.NeedsUpgrade {
			return false
		}

		// The version is already correct
		if h.Metadata.K0sBinaryVersion == p.Config.Spec.K0s.Version {
			return false
		}

		return true
	})
	return nil
}

// ShouldRun is true when there are hosts that need binary uploading
func (p *UploadBinaries) ShouldRun() bool {
	return len(p.hosts) > 0
}

// Run the phase
func (p *UploadBinaries) Run() error {
	return p.parallelDoUpload(p.hosts, p.uploadBinary)
}

func (p *UploadBinaries) ensureBinPath(h *cluster.Host) error {
	dir := filepath.Dir(h.Configurer.K0sBinaryPath())
	// FileExist uses "-e" which also works for dirs
	if h.Configurer.FileExist(h, dir) {
		return nil
	}
	if err := h.Configurer.MkDir(h, dir, exec.Sudo(h)); err != nil {
		return fmt.Errorf("failed to create %s: %w", dir, err)
	}
	if err := h.Configurer.Chmod(h, dir, "0755", exec.Sudo(h)); err != nil {
		return fmt.Errorf("failed to chmod %s: %w", dir, err)
	}
	return nil
}

func (p *UploadBinaries) uploadBinary(h *cluster.Host) error {
	stat, err := os.Stat(h.UploadBinaryPath)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", h.UploadBinaryPath, err)
	}
	if h.FileChanged(h.UploadBinaryPath, h.Configurer.K0sBinaryPath()) {
		if err := p.ensureBinPath(h); err != nil {
			return err
		}
		log.Infof("%s: uploading k0s binary from %s", h, h.UploadBinaryPath)
		if err := h.Upload(h.UploadBinaryPath, h.Configurer.K0sBinaryPath(), exec.Sudo(h)); err != nil {
			return err
		}
	} else {
		log.Infof("%s: k0s binary %s already exists on the target and hasn't been changed, skipping upload", h, h.UploadBinaryPath)
	}

	if err := h.Configurer.Chmod(h, h.Configurer.K0sBinaryPath(), "0700", exec.Sudo(h)); err != nil {
		return err
	}

	log.Debugf("%s: touching %s", h, h.Configurer.K0sBinaryPath())
	if err := h.Configurer.Touch(h, h.Configurer.K0sBinaryPath(), stat.ModTime(), exec.Sudo(h)); err != nil {
		return fmt.Errorf("failed to touch %s: %w", h.Configurer.K0sBinaryPath(), err)
	}

	uploadedVersion, err := h.Configurer.K0sBinaryVersion(h)
	if err != nil {
		return fmt.Errorf("failed to get uploaded k0s binary version: %w", err)
	}

	h.Metadata.K0sBinaryVersion = uploadedVersion.String()
	log.Debugf("%s: has k0s binary version %s", h, h.Metadata.K0sBinaryVersion)

	if version, err := version.NewVersion(p.Config.Spec.K0s.Version); err == nil && !version.Equal(uploadedVersion) {
		return fmt.Errorf("uploaded k0s binary version is %s not %s", uploadedVersion, version)
	}

	return nil
}
