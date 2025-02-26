package configurer

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/alessio/shellescape"
	"github.com/k0sproject/rig/exec"
	"github.com/k0sproject/rig/os"
	"github.com/k0sproject/version"
)

// Static Constants Interface for overriding by distro-specific structs
type PathFuncs interface {
	K0sBinaryPath() string
	K0sConfigPath() string
	K0sJoinTokenPath() string
	KubeconfigPath(h os.Host) string
}

// Linux is a base module for various linux OS support packages
type Linux struct {
	PathFuncs
}

// NOTE The Linux struct does not embed rig/os.Linux because it will confuse
// go as the distro-configurers' parents embed it too. This means you can't
// add functions to base Linux package that call functions in the rig/os.Linux package,
// you can however write those functions in the distro-configurers.
// An example of this problem is the ReplaceK0sTokenPath function, which would like to
// call `l.ServiceScriptPath("kos")`, which was worked around here by getting the
// path as a parameter.

// Arch returns the host processor architecture in the format k0s expects it
func (l Linux) Arch(h os.Host) (string, error) {
	arch, err := h.ExecOutput("uname -m")
	if err != nil {
		return "", err
	}
	switch arch {
	case "x86_64":
		return "amd64", nil
	case "aarch64":
		return "arm64", nil
	case "armv7l", "armv8l", "aarch32", "arm32", "armhfp", "arm-32":
		return "arm", nil
	default:
		return arch, nil
	}
}

// K0sCmdf can be used to construct k0s commands in sprintf style.
func (l Linux) K0sCmdf(template string, args ...interface{}) string {
	return fmt.Sprintf("%s %s", l.PathFuncs.K0sBinaryPath(), fmt.Sprintf(template, args...))
}

// K0sBinaryPath returns the location of k0s binary
func (l Linux) K0sBinaryPath() string {
	return "/usr/local/bin/k0s"
}

func (l Linux) K0sBinaryVersion(h os.Host) (*version.Version, error) {
	k0sVersionCmd := l.K0sCmdf("version")
	output, err := h.ExecOutput(k0sVersionCmd, exec.Sudo(h))
	if err != nil {
		return nil, err
	}

	version, err := version.NewVersion(output)
	if err != nil {
		return nil, err
	}

	return version, nil
}

// K0sConfigPath returns the location of k0s configuration file
func (l Linux) K0sConfigPath() string {
	return "/etc/k0s/k0s.yaml"
}

// K0sJoinTokenPath returns the location of k0s join token file
func (l Linux) K0sJoinTokenPath() string {
	return "/etc/k0s/k0stoken"
}

// K0sctlLockFilePath returns a path to a lock file
func (l Linux) K0sctlLockFilePath(h os.Host) string {
	if h.Exec("test -d /run/lock", exec.Sudo(h)) == nil {
		return "/run/lock/k0sctl"
	}

	return "/tmp/k0sctl.lock"
}

// TempFile returns a temp file path
func (l Linux) TempFile(h os.Host) (string, error) {
	return h.ExecOutput("mktemp")
}

// TempDir returns a temp dir path
func (l Linux) TempDir(h os.Host) (string, error) {
	return h.ExecOutput("mktemp -d")
}

// DownloadURL performs a download from a URL on the host
func (l Linux) DownloadURL(h os.Host, url, destination string, opts ...exec.Option) error {
	return h.Exec(fmt.Sprintf(`curl -sSLf -o %s %s`, shellescape.Quote(destination), shellescape.Quote(url)), opts...)
}

// DownloadK0s performs k0s binary download from github on the host
func (l Linux) DownloadK0s(h os.Host, version *version.Version, arch string) error {
	tmp, err := l.TempFile(h)
	if err != nil {
		return err
	}
	defer func() { _ = h.Execf(`rm -f "%s"`, tmp) }()

	url := fmt.Sprintf("https://github.com/k0sproject/k0s/releases/download/%s/k0s-%s-%s", version, version, arch)
	if err := l.DownloadURL(h, url, tmp); err != nil {
		return err
	}

	if err := h.Execf(`install -m 0755 -o root -g root -d "%s"`, path.Dir(l.PathFuncs.K0sBinaryPath()), exec.Sudo(h)); err != nil {
		return err
	}

	return h.Execf(`install -m 0750 -o root -g root "%s" "%s"`, tmp, l.PathFuncs.K0sBinaryPath(), exec.Sudo(h))
}

// ReplaceK0sTokenPath replaces the config path in the service stub
func (l Linux) ReplaceK0sTokenPath(h os.Host, spath string) error {
	return h.Exec(fmt.Sprintf("sed -i 's^REPLACEME^%s^g' %s", l.PathFuncs.K0sJoinTokenPath(), spath))
}

// FileContains returns true if a file contains the substring
func (l Linux) FileContains(h os.Host, path, s string) bool {
	return h.Execf(`grep -q "%s" "%s"`, s, path, exec.Sudo(h)) == nil
}

// MoveFile moves a file on the host
func (l Linux) MoveFile(h os.Host, src, dst string) error {
	return h.Execf(`mv "%s" "%s"`, src, dst, exec.Sudo(h))
}

// KubeconfigPath returns the path to a kubeconfig on the host
func (l Linux) KubeconfigPath(h os.Host) string {
	linux := &os.Linux{}
	if linux.FileExist(h, "/var/lib/k0s/pki/admin.conf") {
		return "/var/lib/k0s/pki/admin.conf"
	}
	return "/var/lib/k0s/kubelet.conf"
}

// KubectlCmdf returns a command line in sprintf manner for running kubectl on the host using the kubeconfig from KubeconfigPath
func (l Linux) KubectlCmdf(h os.Host, s string, args ...interface{}) string {
	return fmt.Sprintf(`env "KUBECONFIG=%s" %s`, l.PathFuncs.KubeconfigPath(h), l.K0sCmdf(`kubectl %s`, fmt.Sprintf(s, args...)))
}

// HTTPStatus makes a HTTP GET request to the url and returns the status code or an error
func (l Linux) HTTPStatus(h os.Host, url string) (int, error) {
	output, err := h.ExecOutput(fmt.Sprintf(`curl -kso /dev/null -w "%%{http_code}" "%s"`, url))
	if err != nil {
		return -1, err
	}
	status, err := strconv.Atoi(output)
	if err != nil {
		return -1, fmt.Errorf("invalid response: %s", err.Error())
	}

	return status, nil
}

const sbinPath = `PATH=/usr/local/sbin:/usr/sbin:/sbin:$PATH`

// PrivateInterface tries to find a private network interface
func (l Linux) PrivateInterface(h os.Host) (string, error) {
	output, err := h.ExecOutput(fmt.Sprintf(`%s; (ip route list scope global | grep -E "\b(172|10|192\.168)\.") || (ip route list | grep -m1 default)`, sbinPath))
	if err == nil {
		re := regexp.MustCompile(`\bdev (\w+)`)
		match := re.FindSubmatch([]byte(output))
		if len(match) > 0 {
			return string(match[1]), nil
		}
		err = fmt.Errorf("can't find 'dev' in output")
	}

	return "", fmt.Errorf("failed to detect a private network interface, define the host privateInterface manually (%s)", err.Error())
}

// PrivateAddress resolves internal ip from private interface
func (l Linux) PrivateAddress(h os.Host, iface, publicip string) (string, error) {
	output, err := h.ExecOutput(fmt.Sprintf("%s ip -o addr show dev %s scope global", sbinPath, iface))
	if err != nil {
		return "", fmt.Errorf("failed to find private interface with name %s: %s. Make sure you've set correct 'privateInterface' for the host in config", iface, output)
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		items := strings.Fields(line)
		if len(items) < 4 {
			continue
		}
		// When subnet mask is 255.255.255.255, CIDR notation is not /32, but it is omitted instead.
		index := strings.Index(items[3], "/")
		addr := items[3]
		if index >= 0 {
			addr = items[3][:index]
		}
		if len(strings.Split(addr, ".")) == 4 {
			if publicip != addr {
				return addr, nil
			}
		}
	}

	return "", fmt.Errorf("not found")
}

// UpsertFile creates a file in path with content only if the file does not exist already
func (l Linux) UpsertFile(h os.Host, path, content string) error {
	tmpf, err := l.TempFile(h)
	if err != nil {
		return err
	}
	if err := h.Execf(`cat > "%s"`, tmpf, exec.Stdin(content), exec.Sudo(h)); err != nil {
		return err
	}

	defer func() {
		_ = h.Execf(`rm -f "%s"`, tmpf, exec.Sudo(h))
	}()

	// mv -n is atomic
	if err := h.Execf(`mv -n "%s" "%s"`, tmpf, path, exec.Sudo(h)); err != nil {
		return fmt.Errorf("upsert failed: %w", err)
	}

	// if original tempfile still exists, error out
	if h.Execf(`test -f "%s"`, tmpf) == nil {
		return fmt.Errorf("upsert failed")
	}

	return nil
}

func (l Linux) DeleteDir(h os.Host, path string, opts ...exec.Option) error {
	return h.Exec(fmt.Sprintf(`rmdir %s`, shellescape.Quote(path)), opts...)
}
