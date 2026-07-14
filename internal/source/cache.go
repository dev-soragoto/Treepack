package source

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"treepack/internal/logging"
)

type cacheConfig struct {
	Dir      string
	Disabled bool
	Logger   *logging.Logger
}

type cacheMetadata struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Digest string `json:"digest,omitempty"`
}

func cacheKey(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c cacheConfig) enabled() bool { return !c.Disabled && c.Dir != "" }
func (c cacheConfig) warn(format string, args ...any) {
	if c.Logger != nil {
		c.Logger.Warn(format, args...)
	}
}
func (c cacheConfig) info(format string, args ...any) {
	if c.Logger != nil {
		c.Logger.Info(format, args...)
	}
}

func cacheRead(c cacheConfig, key, target, name string, size int64, digest, expectedSHA string, h Hasher) (string, bool) {
	if !c.enabled() {
		return "", false
	}
	dir := filepath.Join(c.Dir, key[:2], key)
	invalid := func() { c.warn("cache invalid: %s", name); _ = os.RemoveAll(dir) }
	var meta cacheMetadata
	body, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil || json.Unmarshal(body, &meta) != nil {
		if err == nil || !os.IsNotExist(err) {
			invalid()
		}
		return "", false
	}
	data := filepath.Join(dir, "data")
	info, err := os.Lstat(data)
	if err != nil || !info.Mode().IsRegular() || meta.Name != name || meta.Size != info.Size() || (size >= 0 && meta.Size != size) || (digest != "" && !strings.EqualFold(meta.Digest, digest)) {
		invalid()
		return "", false
	}
	sum, err := h.SHA256File(data)
	if err != nil || !strings.EqualFold(sum, meta.SHA256) || (expectedSHA != "" && !strings.EqualFold(sum, expectedSHA)) {
		invalid()
		return "", false
	}
	if err := h.CopyFile(data, target); err != nil {
		c.warn("cache read warning for %s: %v", name, err)
		return "", false
	}
	c.info("cache hit: %s", name)
	return sum, true
}

func cacheWrite(c cacheConfig, key, source string, meta cacheMetadata, h Hasher) {
	if !c.enabled() {
		return
	}
	parent := filepath.Join(c.Dir, key[:2])
	if err := os.MkdirAll(parent, 0o755); err != nil {
		c.warn("cache write warning for %s: %v", meta.Name, err)
		return
	}
	tmp, err := os.MkdirTemp(parent, ".tmp-")
	if err != nil {
		c.warn("cache write warning for %s: %v", meta.Name, err)
		return
	}
	defer os.RemoveAll(tmp)
	if err = h.CopyFile(source, filepath.Join(tmp, "data")); err == nil {
		var body []byte
		body, err = json.Marshal(meta)
		if err == nil {
			err = os.WriteFile(filepath.Join(tmp, "metadata.json"), body, 0o644)
		}
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(parent, key))
	}
	if err != nil {
		if _, statErr := os.Stat(filepath.Join(parent, key)); statErr == nil {
			return
		}
		c.warn("cache write warning for %s: %v", meta.Name, err)
	}
}

func normalizedDigestSHA(digest string) string {
	algorithm, value, ok := strings.Cut(digest, ":")
	if ok && strings.EqualFold(algorithm, "sha256") {
		return strings.ToLower(value)
	}
	return ""
}

func cacheMeta(name string, size int64, sum, digest string) cacheMetadata {
	return cacheMetadata{Name: name, Size: size, SHA256: strings.ToLower(sum), Digest: digest}
}
