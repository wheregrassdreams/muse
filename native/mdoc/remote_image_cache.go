package mdoc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"muse/cachepath"
)

const maxRemoteImageBytes = 20 << 20

func remoteImageCachePath(src string) (string, error) {
	root, err := cachepath.Root()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(src))
	return filepath.Join(root, "images", hex.EncodeToString(digest[:])), nil
}

// fetchRemoteImageData reads a previously downloaded and validated image
// before making a request. All callers run it outside the UI draw path.
func fetchRemoteImageData(src string) ([]byte, error) {
	path, pathErr := remoteImageCachePath(src)
	if pathErr == nil {
		if st, err := os.Stat(path); err == nil {
			if st.Size() > 0 && st.Size() <= maxRemoteImageBytes {
				if data, readErr := os.ReadFile(path); readErr == nil {
					if _, _, _, decodeErr := decodeImageFrames(data); decodeErr == nil {
						return data, nil
					}
				}
			}
			_ = os.Remove(path)
		}
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(src)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download image: HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRemoteImageBytes {
		return nil, fmt.Errorf("download image: response exceeds %d bytes", maxRemoteImageBytes)
	}
	if _, _, _, err := decodeImageFrames(data); err != nil {
		return nil, err
	}
	if pathErr == nil {
		_ = writeRemoteImageCache(path, data)
	}
	return data, nil
}

func writeRemoteImageCache(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".image-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return nil
}
