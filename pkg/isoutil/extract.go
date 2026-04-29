package isoutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ExtractISOPath copies one file from an ISO9660 image using xorriso (same tool used for repacking).
func ExtractISOPath(xorrisoBin, isoPath, isoInternalPath, destPath string) error {
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// -osirrox enables extraction from ISO; paths use POSIX-style leading '/'.
	cmd := exec.Command(xorrisoBin,
		"-osirrox", "on",
		"-indev", isoPath,
		"-extract", isoInternalPath, destPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xorriso extract %s from %s: %w: %s", isoInternalPath, isoPath, err, stderr.String())
	}
	return nil
}
