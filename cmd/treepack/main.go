package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"treepack/internal/build"
	"treepack/internal/logging"
	"treepack/internal/manifest"
)

var version = "dev"

//go:embed help/*.txt
var helpFiles embed.FS

type optionKind int

const (
	stringOption optionKind = iota
	intOption
	boolOption
)

type optionTarget int

const (
	configOption optionTarget = iota
	sourceOption
	outputOption
	workDirOption
	keepWorkOption
	rawArchiveOption
	proxyOption
	downloadRetriesOption
	githubTokenOption
	versionOption
	helpOption
)

type optionSpec struct {
	target      optionTarget
	kind        optionKind
	long        string
	short       string
	metavar     string
	defaultText string
	description string
}

type cliOptions struct {
	config          string
	source          string
	output          string
	workDir         string
	keepWork        bool
	rawArchive      bool
	proxy           string
	githubToken     string
	downloadRetries int
	showVersion     bool
	showHelp        bool
}

var optionSpecs = []optionSpec{
	{target: configOption, kind: stringOption, long: "config", short: "c", metavar: "kit.toml", defaultText: "kit.toml", description: "Manifest path."},
	{target: sourceOption, kind: stringOption, long: "source", short: "s", metavar: "DIR", description: "Override paths.source from the manifest."},
	{target: outputOption, kind: stringOption, long: "output", short: "o", metavar: "DIR", description: "Override paths.output from the manifest."},
	{target: workDirOption, kind: stringOption, long: "work-dir", metavar: "DIR", description: "Override paths.work from the manifest."},
	{target: keepWorkOption, kind: boolOption, long: "keep-work", description: "Keep the work run directory after the build."},
	{target: rawArchiveOption, kind: boolOption, long: "raw-archive", description: "Include common OS desktop metadata in generated zip archives."},
	{target: proxyOption, kind: stringOption, long: "proxy", short: "p", metavar: "URL", description: "Proxy for downloads. Supports http, https, socks5, and socks5h."},
	{target: downloadRetriesOption, kind: intOption, long: "download-retries", metavar: "N", defaultText: "3", description: "Total download attempts, including the first request."},
	{target: githubTokenOption, kind: stringOption, long: "github-token", metavar: "TOKEN", description: "GitHub token for release API requests and GitHub asset downloads."},
	{target: versionOption, kind: boolOption, long: "version", short: "v", description: "Print the treepack version and exit."},
	{target: helpOption, kind: boolOption, long: "help", short: "h", description: "Print overview help or topic help and exit."},
}

var helpTopics = []string{"config", "packages", "build", "paths", "verify", "examples"}

// main 运行 treepack 命令行入口并把退出码交给操作系统。
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run 解析命令行参数、执行构建流程并返回进程退出码。
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("treepack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { usage(stderr) }
	var opts cliOptions
	registerOptions(fs, &opts)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if opts.showHelp {
		return showHelp(fs.Args(), stdout, stderr)
	}
	if opts.showVersion {
		fmt.Fprintf(stdout, "treepack %s\n", version)
		return 0
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "treepack: unexpected argument: %s\n", fs.Arg(0))
		usage(stderr)
		return 2
	}
	if opts.proxy != "" {
		if err := validateProxy(opts.proxy); err != nil {
			fmt.Fprintf(stderr, "treepack: invalid proxy: %s\n", err)
			usage(stderr)
			return 2
		}
	}
	if opts.downloadRetries < 1 {
		fmt.Fprintf(stderr, "treepack: invalid download retries: must be at least 1\n")
		usage(stderr)
		return 2
	}
	token := opts.githubToken
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	rep, err := build.Build(build.Options{
		ConfigPath:  opts.config,
		Source:      opts.source,
		Output:      opts.output,
		WorkDir:     opts.workDir,
		KeepWork:    opts.keepWork,
		RawArchive:  opts.rawArchive,
		Retries:     opts.downloadRetries,
		GitHubToken: token,
		Proxy:       opts.proxy,
		Version:     version,
		Logger:      logging.New(stderr),
		Progress:    stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "treepack: %s\n", err)
		if errors.Is(err, manifest.ErrManifestNotFound) {
			usage(stderr)
		}
		return 1
	}
	fmt.Fprintf(stdout, "built %s into %s\n", rep.Manifest.Pack.Name, rep.Paths.OutputRoot)
	return 0
}

// registerOptions binds every command-line option from the shared option spec.
func registerOptions(fs *flag.FlagSet, opts *cliOptions) {
	for _, spec := range optionSpecs {
		switch spec.kind {
		case stringOption:
			ptr := stringOptionValue(opts, spec.target)
			*ptr = spec.defaultText
			fs.StringVar(ptr, spec.long, spec.defaultText, spec.description)
			if spec.short != "" {
				fs.StringVar(ptr, spec.short, spec.defaultText, spec.description)
			}
		case intOption:
			value, err := strconv.Atoi(spec.defaultText)
			if err != nil {
				panic(fmt.Sprintf("invalid default for --%s: %s", spec.long, spec.defaultText))
			}
			ptr := intOptionValue(opts, spec.target)
			*ptr = value
			fs.IntVar(ptr, spec.long, value, spec.description)
		case boolOption:
			ptr := boolOptionValue(opts, spec.target)
			fs.BoolVar(ptr, spec.long, false, spec.description)
			if spec.short != "" {
				fs.BoolVar(ptr, spec.short, false, spec.description)
			}
		}
	}
}

func stringOptionValue(opts *cliOptions, target optionTarget) *string {
	switch target {
	case configOption:
		return &opts.config
	case sourceOption:
		return &opts.source
	case outputOption:
		return &opts.output
	case workDirOption:
		return &opts.workDir
	case proxyOption:
		return &opts.proxy
	case githubTokenOption:
		return &opts.githubToken
	default:
		panic("invalid string option target")
	}
}

func intOptionValue(opts *cliOptions, target optionTarget) *int {
	switch target {
	case downloadRetriesOption:
		return &opts.downloadRetries
	default:
		panic("invalid int option target")
	}
}

func boolOptionValue(opts *cliOptions, target optionTarget) *bool {
	switch target {
	case keepWorkOption:
		return &opts.keepWork
	case rawArchiveOption:
		return &opts.rawArchive
	case versionOption:
		return &opts.showVersion
	case helpOption:
		return &opts.showHelp
	default:
		panic("invalid bool option target")
	}
}

// validateProxy 校验下载代理地址是否使用受支持的协议并包含主机名。
func validateProxy(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return fmt.Errorf("scheme must be http, https, socks5, or socks5h")
	}
	if parsed.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

func showHelp(args []string, stdout, stderr io.Writer) int {
	switch len(args) {
	case 0:
		usage(stdout)
		return 0
	case 1:
		if body, ok := topicHelp(args[0]); ok {
			fmt.Fprint(stdout, body)
			return 0
		}
		fmt.Fprintf(stderr, "treepack: unknown help topic: %s\n", args[0])
		fmt.Fprintf(stderr, "available help topics: %s\n", strings.Join(helpTopics, ", "))
		return 2
	default:
		fmt.Fprintf(stderr, "treepack: unexpected argument: %s\n", args[1])
		usage(stderr)
		return 2
	}
}

func topicHelp(topic string) (string, bool) {
	for _, known := range helpTopics {
		if topic != known {
			continue
		}
		body, err := helpFiles.ReadFile("help/" + topic + ".txt")
		if err != nil {
			panic(err)
		}
		return string(body), true
	}
	return "", false
}

// usage 将生成的总览帮助写入指定输出。
func usage(out io.Writer) {
	fmt.Fprint(out, "Usage:\n")
	fmt.Fprintf(out, "  %s\n\n", usageLine())
	fmt.Fprint(out, "Options:\n")
	fmt.Fprint(out, optionHelp())
	body, err := helpFiles.ReadFile("help/overview.txt")
	if err != nil {
		panic(err)
	}
	fmt.Fprint(out, string(body))
}

func usageLine() string {
	parts := []string{"treepack"}
	for _, spec := range optionSpecs {
		name := "--" + spec.long
		if spec.short != "" {
			name = "-" + spec.short
		}
		if spec.metavar != "" {
			name += " " + spec.metavar
		}
		if spec.target == helpOption {
			name += " [TOPIC]"
		}
		parts = append(parts, "["+name+"]")
	}
	return strings.Join(parts, " ")
}

func optionHelp() string {
	var lines []string
	width := 0
	for _, spec := range optionSpecs {
		name := optionName(spec)
		if len(name) > width {
			width = len(name)
		}
		lines = append(lines, name)
	}
	var b strings.Builder
	for i, spec := range optionSpecs {
		description := spec.description
		if spec.defaultText != "" {
			description += " Default: " + spec.defaultText + "."
		}
		fmt.Fprintf(&b, "  %-*s  %s\n", width, lines[i], description)
	}
	return b.String()
}

func optionName(spec optionSpec) string {
	long := "--" + spec.long
	if spec.metavar != "" {
		long += " " + spec.metavar
	}
	if spec.short == "" {
		return "    " + long
	}
	return "-" + spec.short + ", " + long
}
