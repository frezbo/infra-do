package main

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/pulumi/pulumi-digitalocean/sdk/v4/go/digitalocean"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
)

const (
	projectName             = "k3s"
	sshKeyPub               = "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAHiG+Ardtv6uR67SN0J6Q8Rug2/X4qrDTS6nC8/f+tnFVHG672LxXEXgYxrjxw5dLzZTexU2DGt8MiULFlN5dXx5gB+BI5bsv/2SZmyPJjdAcZAuxE5evu6i4+Z1f/d7BUWGhDlP0vniDM9Wz6q0mBskt/6BULu2rT1rZxgL1bchx78kA=="
	flatCarDistributionType = "CoreOS"
	flatCarVersion          = "2605.7.0"
	flatCarURL              = "https://stable.release.flatcar-linux.net/amd64-usr/" + flatCarVersion + "/flatcar_production_digitalocean_image.bin.bz2"
	k3sLeaderMachineSize    = "s-2vcpu-4gb"
	digitaloceanRegion      = "ams3"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		image, err := digitalocean.NewCustomImage(ctx, "flatcar_image", &digitalocean.CustomImageArgs{
			Description:  pulumi.String("FlatCar Linux"),
			Distribution: pulumi.String(flatCarDistributionType),
			Name:         pulumi.String(generateResourceName(ctx, "FlatCar Linux")),
			Regions:      pulumi.StringArray{pulumi.String(digitaloceanRegion)},
			Tags:         generateResourceTags(ctx, []string{"FlatCar", "KinVolk"}),
			Url:          pulumi.String(flatCarURL),
		})
		if err != nil {
			return err
		}
		DOSSHKeyPub, err := digitalocean.NewSshKey(ctx, "ssh_key_pub", &digitalocean.SshKeyArgs{
			Name:      pulumi.String(generateResourceName(ctx, ctx.Project())),
			PublicKey: pulumi.String(sshKeyPub),
		})
		if err != nil {
			return err
		}
		vpcCIDR, err := vpcCIDRsByStack(ctx)
		if err != nil {
			return err
		}
		vpc, err := digitalocean.NewVpc(ctx, "vpc", &digitalocean.VpcArgs{
			Description: pulumi.String(generateResourceName(ctx, "vpc")),
			IpRange:     pulumi.String(vpcCIDR),
			Name:        pulumi.String(generateResourceName(ctx, "vpc")),
			Region:      pulumi.String(digitaloceanRegion),
		})
		if err != nil {
			return err
		}
		k3sToken, err := random.NewRandomPassword(ctx, "k3s_token", &random.RandomPasswordArgs{
			Length:  pulumi.Int(70),
			Special: pulumi.Bool(false),
		})
		if err != nil {
			return err
		}
		k3sInitdroplet, err := digitalocean.NewDroplet(ctx, "k3s-init-leader", &digitalocean.DropletArgs{
			Image:    pulumi.Sprintf("%d", image.ImageId),
			Name:     pulumi.String(generateResourceName(ctx, "leader-00")),
			Region:   pulumi.String(digitaloceanRegion),
			Size:     pulumi.String(k3sLeaderMachineSize),
			SshKeys:  pulumi.StringArray{DOSSHKeyPub.Fingerprint},
			Tags:     generateResourceTags(ctx, []string{"leader-00"}),
			UserData: k3sInitServerIgnitionConfig(k3sToken.Result),
			VpcUuid:  vpc.ID(),
		})
		if err != nil {
			return err
		}
		var leaderIPs pulumi.StringArray
		for i := 1; i < 3; i++ {
			name := fmt.Sprintf("leader-0%d", i)
			droplet, err := digitalocean.NewDroplet(ctx, name, &digitalocean.DropletArgs{
				Image:             pulumi.Sprintf("%d", image.ImageId),
				Name:              pulumi.String(generateResourceName(ctx, name)),
				Region:            pulumi.String(digitaloceanRegion),
				Size:              pulumi.String(k3sLeaderMachineSize),
				PrivateNetworking: pulumi.Bool(true),
				SshKeys:           pulumi.StringArray{DOSSHKeyPub.Fingerprint},
				Tags:              generateResourceTags(ctx, []string{name}),
				UserData:          k3sServerIgnitionConfig(k3sToken.Result, k3sInitdroplet.Ipv4AddressPrivate),
				VpcUuid:           vpc.ID(),
			})
			if err != nil {
				return err
			}
			leaderIPs = append(leaderIPs, droplet.Ipv4AddressPrivate)
		}
		ctx.Export("k3s-init-ip", k3sInitdroplet.Ipv4Address)
		ctx.Export("k3s-server-ips", leaderIPs)
		return nil
	})
}

func generateResourceName(ctx *pulumi.Context, resourceName string) string {
	return fmt.Sprintf("%s-%s-%s-%s", ctx.Stack(), digitaloceanRegion, projectName, resourceName)
}

func generateResourceTags(ctx *pulumi.Context, resourceTags []string) pulumi.StringArray {
	tags := pulumi.StringArray{
		pulumi.String(digitaloceanRegion),
		pulumi.String(ctx.Stack()),
	}
	for _, tag := range resourceTags {
		tags = append(tags, pulumi.String(tag))
	}
	return tags
}

func vpcCIDRsByStack(ctx *pulumi.Context) (string, error) {
	vpcCIDRS := map[string]string{
		"dev":  "10.16.32.0/20",
		"prod": "10.16.48.0/20",
	}
	vpcCIDR, ok := vpcCIDRS[ctx.Stack()]
	if !ok {
		return "", fmt.Errorf("VPC CIDR for stack %s, not defined", ctx.Stack())
	}
	return vpcCIDR, nil

}

func k3sInitServerIgnitionConfig(token pulumi.StringOutput) pulumi.StringOutput {
	return pulumi.Sprintf(`
{
  "ignition": {
    "version": "2.1.0"
  },
  "storage": {
    "files": [{
      "filesystem": "root",
      "path": "/etc/modules-load.d/iscsi_tcp.conf",
      "mode": 420,
      "contents": { "source": "data:,iscsi_tcp" }
    }]
  },
  "systemd": {
    "units": [
      %s,
      {
        "name": "install-k3s.service",
        "enabled": true,
        "contents": "[Unit]\n\tDescription=Install k3s\n\tRequires=usr-libexec.mount\n\tConditionPathExists=!/etc/.k3s-installed\n[Service]\n\tType=oneshot\n\tEnvironment=INSTALL_K3S_BIN_DIR=/opt/bin INSTALL_K3S_SELINUX_WARN=true K3S_TOKEN=%s INSTALL_K3S_EXEC=\"server --cluster-init --disable local-storage\"\n\tExecStart=/usr/bin/mkdir -p /opt/bin\n\tExecStart=/usr/bin/curl -sfSL -o /tmp/k3s.sh https://get.k3s.io\n\tExecStart=/usr/bin/chmod +x /tmp/k3s.sh\n\tExecStart=/tmp/k3s.sh\n\tExecStart=/usr/bin/touch /etc/.k3s-installed\n[Install]\n\tWantedBy=multi-user.target\n"
      }
    ]
  }
}
`, k3sCommonConfig(), token)
}

func k3sServerIgnitionConfig(token pulumi.StringOutput, ip pulumi.StringOutput) pulumi.StringOutput {
	return pulumi.Sprintf(`
{
  "ignition": {
    "version": "2.1.0"
  },
  "storage": {
    "files": [{
      "filesystem": "root",
      "path": "/etc/modules-load.d/iscsi_tcp.conf",
      "mode": 420,
      "contents": { "source": "data:,iscsi_tcp" }
    }]
  },
  "systemd": {
    "units": [
      %s,
      {
        "name": "install-k3s.service",
        "enabled": true,
        "contents": "[Unit]\n\tDescription=Install k3s\n\tRequires=usr-libexec.mount\n\tConditionPathExists=!/etc/.k3s-installed\n[Service]\n\tType=oneshot\n\tEnvironment=INSTALL_K3S_BIN_DIR=/opt/bin INSTALL_K3S_SELINUX_WARN=true K3S_TOKEN=%s INSTALL_K3S_EXEC=\"server --disable local-storage --server https://%s:6443\"\n\tExecStart=/usr/bin/mkdir -p /opt/bin\n\tExecStart=/usr/bin/curl -sfSL -o /tmp/k3s.sh https://get.k3s.io\n\tExecStart=/usr/bin/chmod +x /tmp/k3s.sh\n\tExecStart=/tmp/k3s.sh\n\tExecStart=/usr/bin/touch /etc/.k3s-installed\n[Install]\n\tWantedBy=multi-user.target\n"
      }
    ]
  }
}
`, k3sCommonConfig(), token, ip)
}

func k3sAgentIgnitionConfig(token pulumi.StringOutput, ip pulumi.StringOutput) pulumi.StringOutput {
	return pulumi.Sprintf(`
{
  "ignition": {
    "version": "2.1.0"
  },
  "storage": {
    "files": [{
      "filesystem": "root",
      "path": "/etc/modules-load.d/iscsi_tcp.conf",
      "mode": 420,
      "contents": { "source": "data:,iscsi_tcp" }
    }]
  },
  "systemd": {
    "units": [
      %s,
      {
        "name": "install-k3s.service",
        "enabled": true,
        "contents": "[Unit]\n\tDescription=Install k3s\n\tRequires=usr-libexec.mount\n\tConditionPathExists=!/etc/.k3s-installed\n[Service]\n\tType=oneshot\n\tEnvironment=INSTALL_K3S_BIN_DIR=/opt/bin INSTALL_K3S_SELINUX_WARN=true K3S_TOKEN=%s INSTALL_K3S_EXEC=\"agent --server https://%s:6443\"\n\tExecStart=/usr/bin/mkdir -p /opt/bin\n\tExecStart=/usr/bin/curl -sfSL -o /tmp/k3s.sh https://get.k3s.io\n\tExecStart=/usr/bin/chmod +x /tmp/k3s.sh\n\tExecStart=/tmp/k3s.sh\n\tExecStart=/usr/bin/touch /etc/.k3s-installed\n[Install]\n\tWantedBy=multi-user.target\n"
      }
    ]
  }
}
`, k3sCommonConfig(), token, ip)
}

func k3sCommonConfig() string {
	return `
      {
        "name": "locksmithd.service",
        "mask": true
      },
      {
        "name": "update-engine.service",
        "enabled": true
      },
      {
        "name": "iscsid-initiatorname.service",
        "dropins": [{
          "name": "iscsi-initiatorname.conf",
          "contents": "[Install]\n\tWantedBy=multi-user.target\n"
        }]
      },
      {
        "name": "iscsid-initiatorname.service",
        "enabled": true
	  },
      {
        "name": "install-opt-dir.service",
        "enabled": true,
        "contents": "[Unit]\n\tDescription=Install /opt directories\n\tConditionPathIsDirectory=!/opt/libexec\n\tConditionPathIsDirectory=!/opt/libexec.wd\n\n[Service]\n\tType=oneshot\n\tExecStart=/usr/bin/mkdir -p /opt/libexec /opt/libexec.wd\n\n[Install]\n\tWantedBy=multi-user.target\n"
      },
      {
        "name": "usr-libexec.mount",
        "enabled": true,
        "contents": "[Unit]\n\tDescription=Allow k8s CNI plugins to be installed\n\tBefore=local-fs.target\n\tRequires=install-opt-dir.service\n\tConditionPathExists=/opt/libexec\n\tConditionPathExists=/opt/libexec.wd\n[Mount]\n\tType=overlay\n\tWhat=overlay\n\tWhere=/usr/libexec\n\tOptions=lowerdir=/usr/libexec,upperdir=/opt/libexec,workdir=/opt/libexec.wd\n[Install]\n\tWantedBy=local-fs.target\n"
      }
`
}
