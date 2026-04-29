package ramdisk

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/cavaliergopher/cpio"
	"github.com/pkg/errors"

	"github.com/abi/minimal-initrd-inject/pkg/networkfiles"
)

//go:embed embeddata/pre-network-manager-config.sh
var preNetworkConfigScript string

//go:embed embeddata/common_network_script.sh
var commonNetworkScript string

//go:embed embeddata/pre-network-manager-config.service
var minimalISONetworkConfigService string

const rootfsServiceConfigFormat = `[Service]
Environment=http_proxy={{.HTTP_PROXY}}
Environment=https_proxy={{.HTTPS_PROXY}}
Environment=no_proxy={{.NO_PROXY}}
Environment=HTTP_PROXY={{.HTTP_PROXY}}
Environment=HTTPS_PROXY={{.HTTPS_PROXY}}
Environment=NO_PROXY={{.NO_PROXY}}`

// ClusterProxyInfo is written into coreos-livepxe-rootfs.service.d for HTTP(S) rootfs fetch.
type ClusterProxyInfo struct {
	HTTPProxy  string
	HTTPSProxy string
	NoProxy    string
}

func (i *ClusterProxyInfo) Empty() bool {
	return i == nil || (i.HTTPProxy == "" && i.HTTPSProxy == "" && i.NoProxy == "")
}

// RamDiskImagePath is the ISO9660 path RHCOS uses for the extra gzip initrd payload.
const RamDiskImagePath = "/images/assisted_installer_custom.img"

// ScriptOverrides optionally replaces embedded NM helper scripts (e.g. when bytes are taken from the same Ignition as the ISO).
type ScriptOverrides struct {
	PreNetwork    *string
	CommonNetwork *string
}

// RamdiskImageArchive builds the gzip-cpio blob matching assisted-service minimal-initrd (keyfiles path).
func RamdiskImageArchive(netFiles []networkfiles.StaticNetworkConfigData, clusterProxyInfo *ClusterProxyInfo, scripts *ScriptOverrides) ([]byte, error) {
	if len(netFiles) == 0 && clusterProxyInfo.Empty() {
		return nil, nil
	}
	buffer := new(bytes.Buffer)
	w := cpio.NewWriter(buffer)
	if len(netFiles) > 0 {
		for _, file := range netFiles {
			err := addFileToArchive(w, filepath.Join("/etc/assisted/network", file.FilePath), file.FileContents, 0o600)
			if err != nil {
				return nil, err
			}
		}
		pre := preNetworkConfigScript
		common := commonNetworkScript
		if scripts != nil {
			if scripts.PreNetwork != nil {
				pre = *scripts.PreNetwork
			}
			if scripts.CommonNetwork != nil {
				common = *scripts.CommonNetwork
			}
		}
		if err := addFileToArchive(w, "/usr/local/bin/pre-network-manager-config.sh", pre, 0o755); err != nil {
			return nil, err
		}
		if err := addFileToArchive(w, "/usr/local/bin/common_network_script.sh", common, 0o755); err != nil {
			return nil, err
		}
		servicePath := "/etc/systemd/system/pre-network-manager-config.service"
		if err := addFileToArchive(w, servicePath, minimalISONetworkConfigService, 0o644); err != nil {
			return nil, err
		}
		serviceLink := "/etc/systemd/system/initrd.target.wants/pre-network-manager-config.service"
		if err := addFileToArchive(w, serviceLink, servicePath, cpio.TypeSymlink|cpio.FileMode(0o777)); err != nil {
			return nil, err
		}
	}
	if !clusterProxyInfo.Empty() {
		rootfsServiceConfigPath := "/etc/systemd/system/coreos-livepxe-rootfs.service.d/10-proxy.conf"
		rootfsServiceConfig, err := formatRootfsServiceConfigFile(clusterProxyInfo)
		if err != nil {
			return nil, err
		}
		if err := addFileToArchive(w, rootfsServiceConfigPath, rootfsServiceConfig, 0o664); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	compressedBuffer := new(bytes.Buffer)
	gzipWriter := gzip.NewWriter(compressedBuffer)
	if _, err := gzipWriter.Write(buffer.Bytes()); err != nil {
		return nil, errors.Wrap(err, "gzip archive")
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, errors.Wrap(err, "gzip close")
	}
	return compressedBuffer.Bytes(), nil
}

func formatRootfsServiceConfigFile(clusterProxyInfo *ClusterProxyInfo) (string, error) {
	params := map[string]string{
		"HTTP_PROXY":  strings.ReplaceAll(clusterProxyInfo.HTTPProxy, "%", "%%"),
		"HTTPS_PROXY": strings.ReplaceAll(clusterProxyInfo.HTTPSProxy, "%", "%%"),
		"NO_PROXY":    clusterProxyInfo.NoProxy,
	}
	tmpl, err := template.New("rootfsServiceConfig").Parse(rootfsServiceConfigFormat)
	if err != nil {
		return "", err
	}
	buf := &bytes.Buffer{}
	if err = tmpl.Execute(buf, params); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func addFileToArchive(w *cpio.Writer, path string, content string, mode cpio.FileMode) error {
	dirsStack := []string{}
	for dir := filepath.Dir(path); dir != "" && dir != "/"; dir = filepath.Dir(dir) {
		dirsStack = append(dirsStack, dir)
	}
	for i := len(dirsStack) - 1; i >= 0; i-- {
		hdr := &cpio.Header{
			Name: dirsStack[i],
			Mode: 040755,
			Size: 0,
		}
		if err := w.WriteHeader(hdr); err != nil {
			return err
		}
	}
	hdr := &cpio.Header{
		Name: path,
		Mode: mode,
		Size: int64(len(content)),
	}
	if err := w.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := w.Write([]byte(content)); err != nil {
		return err
	}
	return nil
}
