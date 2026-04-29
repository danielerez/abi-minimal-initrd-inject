package fromignition

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	igntypes "github.com/coreos/ignition/v2/config/v3_7_experimental/types"
	"github.com/pkg/errors"
	"github.com/vincent-petithory/dataurl"

	"github.com/abi/minimal-initrd-inject/pkg/networkfiles"
	"github.com/abi/minimal-initrd-inject/pkg/ramdisk"
)

const (
	assistedNetworkPrefix       = "/etc/assisted/network/"
	preNetworkScriptPath        = "/usr/local/bin/pre-network-manager-config.sh"
	commonNetworkScriptPath     = "/usr/local/bin/common_network_script.sh"
	systemdDefaultEnvPath       = "/etc/systemd/system.conf.d/10-default-env.conf"
	coreosLiveRootfsProxyDropin = "/etc/systemd/system/coreos-livepxe-rootfs.service.d/10-proxy.conf"
)

// RamdiskInputs collects initrd overlay material from a rendered agent Ignition config (config.ign).
type RamdiskInputs struct {
	NetworkFiles       []networkfiles.StaticNetworkConfigData
	Proxy              *ramdisk.ClusterProxyInfo
	PreNetworkScript   *string // optional override from ignition (else ramdisk uses embed)
	CommonNetworkScript *string
}

// FromConfig walks ignition storage.files and extracts assisted static-network paths,
// optional helper scripts, proxy env for rootfs fetch, and systemd drop-in proxy if present.
func FromConfig(cfg *igntypes.Config) (*RamdiskInputs, error) {
	if cfg == nil {
		return nil, errors.New("ignition config is nil")
	}
	out := &RamdiskInputs{}
	var proxyFromEnv *ramdisk.ClusterProxyInfo
	var proxyFromDropin *ramdisk.ClusterProxyInfo

	for i := range cfg.Storage.Files {
		f := &cfg.Storage.Files[i]
		p := f.Path
		switch {
		case strings.HasPrefix(p, assistedNetworkPrefix):
			rel, err := filepathRelAssistedNetwork(p)
			if err != nil {
				return nil, err
			}
			body, err := resourceContents(&f.Contents)
			if err != nil {
				return nil, errors.Wrapf(err, "decode %s", p)
			}
			out.NetworkFiles = append(out.NetworkFiles, networkfiles.StaticNetworkConfigData{
				FilePath:     rel,
				FileContents: body,
			})
		case p == preNetworkScriptPath:
			s, err := resourceContents(&f.Contents)
			if err != nil {
				return nil, errors.Wrapf(err, "decode %s", p)
			}
			out.PreNetworkScript = &s
		case p == commonNetworkScriptPath:
			s, err := resourceContents(&f.Contents)
			if err != nil {
				return nil, errors.Wrapf(err, "decode %s", p)
			}
			out.CommonNetworkScript = &s
		case p == systemdDefaultEnvPath:
			s, err := resourceContents(&f.Contents)
			if err != nil {
				return nil, errors.Wrapf(err, "decode %s", p)
			}
			proxyFromEnv = parseDefaultEnvironmentProxy(s)
		case p == coreosLiveRootfsProxyDropin:
			s, err := resourceContents(&f.Contents)
			if err != nil {
				return nil, errors.Wrapf(err, "decode %s", p)
			}
			proxyFromDropin = parseSystemdEnvironmentDropin(s)
		}
	}

	if len(out.NetworkFiles) == 0 {
		return nil, fmt.Errorf("ignition has no files under %q (static network is required for this tool)", assistedNetworkPrefix)
	}

	// Prefer explicit livepxe drop-in; else generic DefaultEnvironment from agent ISO.
	switch {
	case proxyFromDropin != nil && !proxyFromDropin.Empty():
		out.Proxy = proxyFromDropin
	case proxyFromEnv != nil && !proxyFromEnv.Empty():
		out.Proxy = proxyFromEnv
	default:
		out.Proxy = &ramdisk.ClusterProxyInfo{}
	}

	return out, nil
}

func filepathRelAssistedNetwork(full string) (string, error) {
	if !strings.HasPrefix(full, assistedNetworkPrefix) {
		return "", fmt.Errorf("path %q does not start with %q", full, assistedNetworkPrefix)
	}
	rel := strings.TrimPrefix(full, assistedNetworkPrefix)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || path.IsAbs(rel) {
		return "", fmt.Errorf("invalid assisted network relative path from %q", full)
	}
	return rel, nil
}

func resourceContents(res *igntypes.Resource) (string, error) {
	if res == nil || res.Source == nil {
		return "", errors.New("resource has no source")
	}
	u, err := dataurl.DecodeString(*res.Source)
	if err != nil {
		return "", err
	}
	data := u.Data
	if res.Compression != nil && *res.Compression == "gzip" {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		defer func() { _ = zr.Close() }()
		data, err = io.ReadAll(zr)
		if err != nil {
			return "", err
		}
	}
	return string(data), nil
}

var envLineRE = regexp.MustCompile(`(?m)^DefaultEnvironment=([A-Z0-9_]+)="([^"]*)"`)

func parseDefaultEnvironmentProxy(conf string) *ramdisk.ClusterProxyInfo {
	p := &ramdisk.ClusterProxyInfo{}
	for _, m := range envLineRE.FindAllStringSubmatch(conf, -1) {
		if len(m) < 3 {
			continue
		}
		k, v := m[1], m[2]
		switch k {
		case "HTTP_PROXY":
			p.HTTPProxy = v
		case "HTTPS_PROXY":
			p.HTTPSProxy = v
		case "NO_PROXY":
			p.NoProxy = v
		}
	}
	return p
}

func parseSystemdEnvironmentDropin(conf string) *ramdisk.ClusterProxyInfo {
	// Drop-in uses [Service] Environment=key=value lines (same pattern as assisted ramdisk).
	p := &ramdisk.ClusterProxyInfo{}
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Environment=") {
			continue
		}
		line = strings.TrimPrefix(line, "Environment=")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = strings.Trim(val, `"`)
		switch strings.ToLower(key) {
		case "http_proxy":
			p.HTTPProxy = val
		case "https_proxy":
			p.HTTPSProxy = val
		case "no_proxy":
			p.NoProxy = val
		}
	}
	return p
}
