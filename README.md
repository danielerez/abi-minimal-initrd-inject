# abi-minimal-initrd-inject

Patches an **agent-based installer minimal ISO** so the bootloader loads a gzip-cpio overlay at `/images/assisted_installer_custom.img`, matching **Assisted Installer** minimal-initrd static networking (NM keyfiles under `/etc/assisted/network` plus `pre-network-manager-config` scripts).

The live ISO already embeds the same network definitions in **Ignition** (`config.ign` inside `images/ignition.img`). This tool reads that Ignition and rebuilds the initrd overlay from those files—no `nmstatectl`, `agent-config.yaml`, or `install-config.yaml`.

## Requirements

- **Minimal ABI ISO** whose embedded Ignition includes static network files under `/etc/assisted/network/` (as produced by `openshift-install agent create image` when NMState / static networking is configured).
- **`xorriso`** on `$PATH` (to extract `images/ignition.img` from the ISO and write the patched output ISO).
- **Go 1.24+** to build (see `go.mod`).

## Usage

```bash
go build -o abi-minimal-initrd-inject ./cmd/abi-minimal-initrd-inject

./abi-minimal-initrd-inject \
  --iso agent.x86_64.iso \
  --output agent-static.x86_64.iso
```

- **`--xorriso`**: path to `xorriso` if not on `PATH`.

### Container image

The image uses **`WORKDIR /work`**. Bind-mount your current directory so inputs and the patched ISO live in **`$PWD`** (no extra output folder):

```bash
podman build -t abi-minimal-initrd-inject .

podman run --rm \
  -v "$PWD:/work" \
  abi-minimal-initrd-inject \
  --iso /work/agent.x86_64.iso \
  --output /work/agent-static.x86_64.iso
```

Runtime image is **UBI 9 minimal** with **xorriso** only (no nmstate).

## What it does

1. Extracts `/images/ignition.img` from the ISO (gzip-cpio container used by RHCOS live images).
2. Reads `config.ign` from that archive and parses Ignition 3.x JSON.
3. Collects `storage.files` entries:
   - **`/etc/assisted/network/...`** → NM keyfiles and `mac_interface.ini` (paths under `host0/` etc., same layout as assisted-service).
   - **`/usr/local/bin/pre-network-manager-config.sh`** and **`common_network_script.sh`** → optional overrides instead of the embedded copies (when present in Ignition).
   - **`/etc/systemd/system.conf.d/10-default-env.conf`** → optional HTTP(S) proxy env for generating **`coreos-livepxe-rootfs.service.d/10-proxy.conf`** in the overlay (early rootfs fetch).
   - **`/etc/systemd/system/coreos-livepxe-rootfs.service.d/10-proxy.conf`** → preferred source for proxy if present.
4. Builds a gzip-cpio blob matching `internal/isoeditor.RamdiskImageArchive` in assisted-service (initrd-specific systemd unit still uses embedded `pre-network-manager-config.service` under `pkg/ramdisk/embeddata/`).
5. Maps that blob onto `/images/assisted_installer_custom.img` in the output ISO via `xorriso`.

## Notes

- If Ignition has **no** files under `/etc/assisted/network/`, the tool exits with an error (nothing to apply in initrd).
- Ignition must live at **`images/ignition.img`** on the ISO (standard agent/RHCOS layout). Images that only embed ignition elsewhere (`coreos/igninfo.json` indirection) are not handled yet.
- Validate on real hardware or a VM; ISO editing can break boot if boot catalog expectations differ.

## License

Apache-2.0 (same family as OpenShift/Assisted components). Embedded scripts under `pkg/ramdisk/embeddata/` align with `openshift/assisted-service` `internal/constants`.
