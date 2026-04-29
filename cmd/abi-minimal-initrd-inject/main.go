package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	ignitioncfg "github.com/coreos/ignition/v2/config"

	"github.com/abi/minimal-initrd-inject/pkg/fromignition"
	"github.com/abi/minimal-initrd-inject/pkg/ignitionimg"
	"github.com/abi/minimal-initrd-inject/pkg/isoutil"
	"github.com/abi/minimal-initrd-inject/pkg/ramdisk"
)

const isoIgnitionImg = "/images/ignition.img"

func main() {
	isoPath := flag.String("iso", "", "path to ABI minimal ISO (e.g. agent.x86_64.iso)")
	outPath := flag.String("output", "", "path for patched ISO output")
	xorriso := flag.String("xorriso", "xorriso", "path to xorriso")
	flag.Parse()
	if *isoPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "usage: abi-minimal-initrd-inject --iso PATH --output PATH")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if _, err := exec.LookPath(*xorriso); err != nil {
		fmt.Fprintf(os.Stderr, "xorriso not found in PATH: %v\n", err)
		os.Exit(1)
	}

	tmpIgn, err := os.CreateTemp("", "ignition.img")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	tmpIgnPath := tmpIgn.Name()
	_ = tmpIgn.Close()
	defer func() { _ = os.Remove(tmpIgnPath) }()

	if err := isoutil.ExtractISOPath(*xorriso, *isoPath, isoIgnitionImg, tmpIgnPath); err != nil {
		fmt.Fprintf(os.Stderr, "extract %s from ISO: %v\n", isoIgnitionImg, err)
		os.Exit(1)
	}

	configIgn, err := ignitionimg.ConfigIgnFromIgnitionImg(tmpIgnPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read config.ign from ignition.img: %v\n", err)
		os.Exit(1)
	}

	cfg, _, err := ignitioncfg.Parse(configIgn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse Ignition config: %v\n", err)
		os.Exit(1)
	}

	inputs, err := fromignition.FromConfig(&cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ignition: %v\n", err)
		os.Exit(1)
	}

	var scripts *ramdisk.ScriptOverrides
	if inputs.PreNetworkScript != nil || inputs.CommonNetworkScript != nil {
		scripts = &ramdisk.ScriptOverrides{
			PreNetwork:    inputs.PreNetworkScript,
			CommonNetwork: inputs.CommonNetworkScript,
		}
	}

	blob, err := ramdisk.RamdiskImageArchive(inputs.NetworkFiles, inputs.Proxy, scripts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ramdisk: %v\n", err)
		os.Exit(1)
	}
	if len(blob) == 0 {
		fmt.Fprintln(os.Stderr, "empty ramdisk (no network files and no proxy)")
		os.Exit(1)
	}

	tmpRam, err := os.CreateTemp("", "assisted_installer_custom-*.img")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	tmpRamPath := tmpRam.Name()
	defer func() { _ = os.Remove(tmpRamPath) }()
	if _, err := tmpRam.Write(blob); err != nil {
		_ = tmpRam.Close()
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	if err := tmpRam.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(*xorriso,
		"-indev", *isoPath,
		"-outdev", *outPath,
		"-map", tmpRamPath, ramdisk.RamDiskImagePath,
		"-boot_image", "any", "replay",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "xorriso: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote patched ISO to %s\n", *outPath)
}
