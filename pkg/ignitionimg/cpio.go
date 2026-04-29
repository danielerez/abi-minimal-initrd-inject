package ignitionimg

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"

	"github.com/cavaliergopher/cpio"
)

const configIgnEntry = "config.ign"

// ConfigIgnFromIgnitionImg reads images/ignition.img (gzip-compressed cpio) and returns config.ign bytes.
func ConfigIgnFromIgnitionImg(ignitionImgPath string) ([]byte, error) {
	f, err := os.Open(ignitionImgPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("ignition.img: not gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()

	cr := cpio.NewReader(gz)
	for {
		hdr, err := cr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cpio: %w", err)
		}
		if hdr.Name != configIgnEntry {
			if _, err := io.Copy(io.Discard, cr); err != nil {
				return nil, err
			}
			continue
		}
		body, err := io.ReadAll(cr)
		if err != nil {
			return nil, err
		}
		return body, nil
	}
	return nil, fmt.Errorf("cpio archive has no %q entry", configIgnEntry)
}
