package phase

import (
	"fmt"

	"github.com/k0sproject/k0sctl/pkg/apis/k0sctl.k0sproject.io/v1beta1/cluster"
)

// ValidateHosts performs remote OS detection
type ValidateHosts struct {
	GenericPhase
	hncount map[string]int
}

// Title for the phase
func (p *ValidateHosts) Title() string {
	return "Validate hosts"
}

// Run the phase
func (p *ValidateHosts) Run() error {
	p.hncount = make(map[string]int, len(p.Config.Spec.Hosts))
	for _, h := range p.Config.Spec.Hosts {
		p.hncount[h.Metadata.Hostname]++
	}

	return p.parallelDo(p.Config.Spec.Hosts, p.validateUniqueHostname, p.validateSudo)
}

func (p *ValidateHosts) validateUniqueHostname(h *cluster.Host) error {
	if p.hncount[h.Metadata.Hostname] > 1 {
		return fmt.Errorf("hostname is not unique: %s", h.Metadata.Hostname)
	}

	return nil
}

func (p *ValidateHosts) validateSudo(h *cluster.Host) error {
	if err := h.Configurer.CheckPrivilege(h); err != nil {
		return err
	}

	return nil
}
