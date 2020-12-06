package template

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const (
	k3sTestToken    = "someRandomToken"
	k3sTestLeaderIP = "someIP"
)

func TestSystemDUnits(t *testing.T) {
	cases := map[string]struct {
		systemDUnitConfig SystemDConfig
		systemDUnits      map[string]string
	}{
		"k3sSetupSystemDUnit": {
			systemDUnitConfig: k3sSetupSystemDUnit(),
			systemDUnits: map[string]string{
				"install-opt-dir.service": `
[Unit]
Description=Install /opt directories
ConditionPathIsDirectory=!/opt/libexec
ConditionPathIsDirectory=!/opt/libexec.wd

[Service]
Type=oneshot
ExecStart=/usr/bin/mkdir -p /opt/libexec /opt/libexec.wd

[Install]
WantedBy=multi-user.target
`,

				"usr-libexec.mount": `
[Unit]
Description=Allow k8s CNI plugins to be installed
Before=local-fs.target
Requires=install-opt-dir.service
ConditionPathExists=/opt/libexec
ConditionPathExists=/opt/libexec.wd

[Mount]
Type=overlay
What=overlay
Where=/opt/libexec
Options=lowerdir=/usr/libexec,upperdir=/opt/libexec,workdir=/opt/libexec.wd

[Install]
WantedBy=local-fs.target
`,
			},
		},
		"k3sLeaderInitSystemDUnit": {
			systemDUnitConfig: k3sLeaderInitSystemDUnit(k3sTestToken),
			systemDUnits: map[string]string{
				"install-k3s.service": fmt.Sprintf(`
[Unit]
Description=Install k3s
Requires=usr-libexec.mount
ConditionPathExists=!/etc/.k3s-installed

[Service]
Type=oneshot
Environment=INSTALL_K3S_BIN_DIR=/opt/bin INSTALL_K3S_SELINUX_WARN=true K3S_TOKEN=%s INSTALL_K3S_EXEC="server --cluster-init"
`, k3sTestToken),
			},
		},
		"k3sLeaderSystemDUnit": {
			systemDUnitConfig: k3sLeaderSystemDUnit(k3sTestToken, k3sTestLeaderIP),
			systemDUnits: map[string]string{
				"install-k3s.service": fmt.Sprintf(`
[Unit]
Description=Install k3s
Requires=usr-libexec.mount
ConditionPathExists=!/etc/.k3s-installed

[Service]
Type=oneshot
Environment=INSTALL_K3S_BIN_DIR=/opt/bin INSTALL_K3S_SELINUX_WARN=true K3S_TOKEN=%s INSTALL_K3S_EXEC="server --server https://%s:6443"
`, k3sTestToken, k3sTestLeaderIP),
			},
		},
	}
	for name, systemDTestConfig := range cases {
		t.Run(name, func(t *testing.T) {
			for unitName, systemDUnit := range systemDTestConfig.systemDUnitConfig {
				systemDUnitStringFromConfig, _ := RenderSystemDUnit(systemDUnit)
				systemDUnitString := systemDTestConfig.systemDUnits[unitName]
				if diff := cmp.Diff(systemDUnitString, systemDUnitStringFromConfig); diff != "" {
					t.Errorf("r: -want, +got:\n%s", diff)
				}
			}
		})
	}
}
