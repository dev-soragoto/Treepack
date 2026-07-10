package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"treepack/internal/ops"
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

type ResourcesConfig struct {
	Copy string `toml:"copy"`
}

type AssetConfig struct {
	Asset   string `toml:"asset"`
	Target  string `toml:"target"`
	Install string `toml:"install"`
}

type PackageConfig struct {
	Name     string                `toml:"name"`
	Source   string                `toml:"source"`
	Group    string                `toml:"group"`
	Required *bool                 `toml:"required"`
	Asset    string                `toml:"asset"`
	Target   string                `toml:"target"`
	Install  string                `toml:"install"`
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
	Path      string          `toml:"-"`
	Pack      PackConfig      `toml:"pack"`
	Paths     PathsConfig     `toml:"paths"`
	Build     BuildConfig     `toml:"build"`
	Layout    LayoutConfig    `toml:"layout"`
	Resources ResourcesConfig `toml:"resources"`
	Packages  []PackageConfig `toml:"packages"`
	Verify    VerifyConfig    `toml:"verify"`
}

// Load 读取 TOML 清单、填充默认值并执行结构校验。
func Load(path string) (*Manifest, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("cannot read manifest: %s", abs)
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
	ctx := fmt.Sprintf("packages[%d]", index)
	if p.Name == "" {
		return fmt.Errorf("%s.name is required", ctx)
	}
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
	if len(p.Assets) > 0 && p.Asset != "" {
		return fmt.Errorf("%s cannot declare both asset and assets", ctx)
	}
	if len(p.Assets) > 0 && (p.Target != "" || p.Install != "") {
		return fmt.Errorf("%s target/install cannot be used with assets", ctx)
	}
	if p.Asset == "" && len(p.Assets) == 0 && (strings.HasPrefix(p.Source, "github:") || strings.HasPrefix(p.Source, "url:")) {
		return fmt.Errorf("%s needs asset or assets for github/url source", ctx)
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
	}
	for i, op := range p.Steps {
		if err := ops.ValidateOperation(op, fmt.Sprintf("%s.steps[%d]", ctx, i+1)); err != nil {
			return err
		}
	}
	return nil
}
