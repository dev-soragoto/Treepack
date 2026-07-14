package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"treepack/internal/ops"

	"github.com/BurntSushi/toml"
)

type PackConfig struct {
	Name    string `toml:"name"`
	Version string `toml:"version"`
}

type BuildConfig struct {
	Archive   string `toml:"archive"`
	BuildInfo string `toml:"build_info"`
	KeepWork  bool   `toml:"keep_work"`
}

type PathsConfig struct {
	Source string `toml:"source"`
	Output string `toml:"output"`
	Work   string `toml:"work"`
}

type LayoutConfig struct {
	Dirs []string `toml:"dirs"`
}

type AssetConfig struct {
	Asset   string `toml:"asset"`
	Target  string `toml:"target"`
	Install string `toml:"install"`
	SHA256  string `toml:"sha256"`
}

type PackageConfig struct {
	Name     string                `toml:"name"`
	Source   string                `toml:"source"`
	Group    string                `toml:"group"`
	Required *bool                 `toml:"required"`
	Asset    string                `toml:"asset"`
	Target   string                `toml:"target"`
	Install  string                `toml:"install"`
	SHA256   string                `toml:"sha256"`
	Assets   []AssetConfig         `toml:"assets"`
	Steps    []ops.OperationConfig `toml:"steps"`
}

// IsRequired 返回包是否应作为必需包处理。
func (p PackageConfig) IsRequired() bool {
	return p.Required == nil || *p.Required
}

type VerifyConfig struct {
	Files  []string `toml:"files"`
	Dirs   []string `toml:"dirs"`
	Absent []string `toml:"absent"`
}

type Manifest struct {
	Path     string          `toml:"-"`
	Pack     PackConfig      `toml:"pack"`
	Paths    PathsConfig     `toml:"paths"`
	Build    BuildConfig     `toml:"build"`
	Layout   LayoutConfig    `toml:"layout"`
	Packages []PackageConfig `toml:"packages"`
	Verify   VerifyConfig    `toml:"verify"`
}

var ErrManifestNotFound = errors.New("manifest not found")

// Load 读取 TOML 清单、填充默认值并执行结构校验。
func Load(path string) (*Manifest, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cannot read manifest: %s: %w", abs, ErrManifestNotFound)
		}
		return nil, fmt.Errorf("cannot read manifest: %s: %w", abs, err)
	}
	var m Manifest
	md, err := toml.DecodeFile(abs, &m)
	if err != nil {
		return nil, fmt.Errorf("invalid TOML in %s: %w", abs, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("unknown config key: %s", undecoded[0].String())
	}
	m.Path = abs
	if m.Pack.Name == "" {
		return nil, fmt.Errorf("pack.name is required")
	}
	if m.Build.BuildInfo == "" {
		m.Build.BuildInfo = "BUILD_INFO.txt"
	}
	for i := range m.Packages {
		if err := validatePackage(&m.Packages[i], i+1); err != nil {
			return nil, err
		}
	}
	return &m, nil
}

// validatePackage 校验单个包配置并补齐包级默认值。
func validatePackage(p *PackageConfig, index int) error {
	if p.Name == "" {
		return fmt.Errorf("package(position: %d).name is required", index)
	}
	ctx := fmt.Sprintf("package(name: %q)", p.Name)
	if p.Source == "" {
		return fmt.Errorf("%s.source is required", ctx)
	}
	if !(strings.HasPrefix(p.Source, "github:") || strings.HasPrefix(p.Source, "url:") || strings.HasPrefix(p.Source, "file:")) {
		return fmt.Errorf("%s.source must start with github:, url:, or file:", ctx)
	}
	if p.Group == "" {
		p.Group = "default"
	}
	if p.Install != "" && p.Install != "extract" {
		return fmt.Errorf("%s.install only supports 'extract'", ctx)
	}
	if err := validateSHA256(p.SHA256); err != nil {
		return fmt.Errorf("%s.sha256 %w", ctx, err)
	}
	if len(p.Assets) > 0 && p.Asset != "" {
		return fmt.Errorf("%s cannot declare both asset and assets", ctx)
	}
	if len(p.Assets) > 0 && (p.Target != "" || p.Install != "" || p.SHA256 != "") {
		return fmt.Errorf("%s target/install/sha256 cannot be used with assets", ctx)
	}
	if strings.HasPrefix(p.Source, "url:") && len(p.Assets) > 1 {
		return fmt.Errorf("%s.assets must contain exactly one asset for url source", ctx)
	}
	if p.Asset == "" && len(p.Assets) == 0 && (strings.HasPrefix(p.Source, "github:") || strings.HasPrefix(p.Source, "url:")) {
		return fmt.Errorf("%s needs asset or assets for github/url source", ctx)
	}
	if p.Install == "extract" && p.Target != "" {
		return fmt.Errorf("%s.target cannot be used with install = 'extract'", ctx)
	}
	if p.Asset != "" {
		if _, err := regexp.Compile(p.Asset); err != nil {
			return fmt.Errorf("%s.asset invalid regex: %w", ctx, err)
		}
	}
	for i, asset := range p.Assets {
		if asset.Asset == "" {
			return fmt.Errorf("%s.assets[%d].asset is required", ctx, i+1)
		}
		if _, err := regexp.Compile(asset.Asset); err != nil {
			return fmt.Errorf("%s.assets[%d].asset invalid regex: %w", ctx, i+1, err)
		}
		if asset.Install != "" && asset.Install != "extract" {
			return fmt.Errorf("%s.assets[%d].install only supports 'extract'", ctx, i+1)
		}
		if asset.Install == "extract" && asset.Target != "" {
			return fmt.Errorf("%s.assets[%d].target cannot be used with install = 'extract'", ctx, i+1)
		}
		if err := validateSHA256(asset.SHA256); err != nil {
			return fmt.Errorf("%s.assets[%d].sha256 %w", ctx, i+1, err)
		}
	}
	for i, op := range p.Steps {
		if err := ops.ValidateOperation(op, fmt.Sprintf("%s.steps[%d]", ctx, i+1)); err != nil {
			return err
		}
	}
	return nil
}

func validateSHA256(value string) error {
	if value == "" {
		return nil
	}
	if len(value) != 64 {
		return fmt.Errorf("must be a 64-character hex string")
	}
	for _, ch := range value {
		if ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F' {
			continue
		}
		return fmt.Errorf("must be a 64-character hex string")
	}
	return nil
}
