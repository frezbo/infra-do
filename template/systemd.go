package template

import (
	"fmt"
	"io/ioutil"

	"github.com/coreos/go-systemd/unit"
)

const (
	k3sInstallDirectory = "/opt/bin"
	k3sLeaderInitArgs   = "server --cluster-init"
	k3sLeaderArgs       = "server --server"
	k3sLeaderPort       = "6443"
)

// SystemDConfig is used to map the systemD unit file name to systemD unit config
type SystemDConfig map[string][]*unit.UnitOption

func k3sSetupSystemDUnit() SystemDConfig {
	return SystemDConfig{
		"install-opt-dir.service": []*unit.UnitOption{
			{
				Section: "Unit",
				Name:    "Description",
				Value:   "Install /opt directories",
			},
			{
				Section: "Unit",
				Name:    "ConditionPathIsDirectory",
				Value:   "!/opt/libexec",
			},
			{
				Section: "Unit",
				Name:    "ConditionPathIsDirectory",
				Value:   "!/opt/libexec.wd",
			},
			{
				Section: "Service",
				Name:    "Type",
				Value:   "oneshot",
			},
			{
				Section: "Service",
				Name:    "ExecStart",
				Value:   "/usr/bin/mkdir -p /opt/libexec /opt/libexec.wd",
			},
			{
				Section: "Install",
				Name:    "WantedBy",
				Value:   "multi-user.target",
			},
		},
		"usr-libexec.mount": []*unit.UnitOption{
			{
				Section: "Unit",
				Name:    "Description",
				Value:   "Allow k8s CNI plugins to be installed",
			},
			{
				Section: "Unit",
				Name:    "Before",
				Value:   "local-fs.target",
			},
			{
				Section: "Unit",
				Name:    "Requires",
				Value:   "install-opt-dir.service",
			},
			{
				Section: "Unit",
				Name:    "ConditionPathExists",
				Value:   "/opt/libexec",
			},
			{
				Section: "Unit",
				Name:    "ConditionPathExists",
				Value:   "/opt/libexec.wd",
			},
			{
				Section: "Mount",
				Name:    "Type",
				Value:   "overlay",
			},
			{
				Section: "Mount",
				Name:    "What",
				Value:   "overlay",
			},
			{
				Section: "Mount",
				Name:    "Where",
				Value:   "/opt/libexec",
			},
			{
				Section: "Mount",
				Name:    "Options",
				Value:   "lowerdir=/usr/libexec,upperdir=/opt/libexec,workdir=/opt/libexec.wd",
			},
			{
				Section: "Install",
				Name:    "WantedBy",
				Value:   "local-fs.target",
			},
		},
	}
}

func k3sLeaderInitSystemDUnit(token string) SystemDConfig {
	installUnitSection := &unit.UnitOption{
		Section: "Service",
		Name:    "Environment",
		Value:   fmt.Sprintf("INSTALL_K3S_BIN_DIR=%s INSTALL_K3S_SELINUX_WARN=true K3S_TOKEN=%s INSTALL_K3S_EXEC=\"%s\"", k3sInstallDirectory, token, k3sLeaderInitArgs),
	}
	return SystemDConfig{
		"install-k3s.service": append(k3sInstallPartialSystemDUnit(), installUnitSection),
	}
}

func k3sLeaderSystemDUnit(token string, leaderIP string) SystemDConfig {
	installUnitSection := &unit.UnitOption{
		Section: "Service",
		Name:    "Environment",
		Value:   fmt.Sprintf("INSTALL_K3S_BIN_DIR=%s INSTALL_K3S_SELINUX_WARN=true K3S_TOKEN=%s INSTALL_K3S_EXEC=\"%s https://%s:%s\"", k3sInstallDirectory, token, k3sLeaderArgs, leaderIP, k3sLeaderPort),
	}
	return SystemDConfig{
		"install-k3s.service": append(k3sInstallPartialSystemDUnit(), installUnitSection),
	}
}

func k3sInstallPartialSystemDUnit() []*unit.UnitOption {
	return []*unit.UnitOption{
		{
			Section: "Unit",
			Name:    "Description",
			Value:   "Install k3s",
		},
		{
			Section: "Unit",
			Name:    "Requires",
			Value:   "usr-libexec.mount",
		},
		{
			Section: "Unit",
			Name:    "ConditionPathExists",
			Value:   "!/etc/.k3s-installed",
		},
		{
			Section: "Service",
			Name:    "Type",
			Value:   "oneshot",
		},
	}
}

// RenderSystemDUnit is to used to render systemD config into multi-line string
func RenderSystemDUnit(systemDUnit []*unit.UnitOption) (string, error) {
	outReader := unit.Serialize(systemDUnit)
	outBytes, err := ioutil.ReadAll(outReader)
	if err != nil {
		return "", err
	}
	return "\n" + string(outBytes), nil
}
