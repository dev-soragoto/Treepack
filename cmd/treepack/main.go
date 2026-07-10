package main

import (
	_ "embed"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"treepack/internal/build"
	"treepack/internal/logging"
)

var version = "dev"

//go:embed help.txt
var helpText string

// main 运行 treepack 命令行入口并把退出码交给操作系统。
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run 解析命令行参数、执行构建流程并返回进程退出码。
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("treepack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { usage(stderr) }
	config := fs.String("config", "kit.toml", "path to kit.toml")
	configShort := fs.String("c", "", "path to kit.toml")
	source := fs.String("source", "", "source directory")
	sourceShort := fs.String("s", "", "source directory")
	output := fs.String("output", "", "output directory")
	outputShort := fs.String("o", "", "output directory")
	workDir := fs.String("work-dir", "", "work directory")
	keepWork := fs.Bool("keep-work", false, "keep work directory")
	proxy := fs.String("proxy", "", "proxy URL for downloads")
	proxyShort := fs.String("p", "", "proxy URL for downloads")
	githubToken := fs.String("github-token", "", "GitHub token for release API and asset downloads")
	downloadRetries := fs.Int("download-retries", 3, "total download attempts including the first request")
	showVersion := fs.Bool("version", false, "show version")
	showVersionShort := fs.Bool("v", false, "show version")
	showHelp := fs.Bool("help", false, "show help")
	showHelpShort := fs.Bool("h", false, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showHelp || *showHelpShort {
		usage(stdout)
		return 0
	}
	if *showVersion || *showVersionShort {
		fmt.Fprintf(stdout, "treepack %s\n", version)
		return 0
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "treepack: unexpected argument: %s\n", fs.Arg(0))
		usage(stderr)
		return 2
	}
	if *configShort != "" {
		*config = *configShort
	}
	if *sourceShort != "" {
		*source = *sourceShort
	}
	if *outputShort != "" {
		*output = *outputShort
	}
	if *proxyShort != "" {
		*proxy = *proxyShort
	}
	if *proxy != "" {
		if err := validateProxy(*proxy); err != nil {
			fmt.Fprintf(stderr, "treepack: invalid proxy: %s\n", err)
			usage(stderr)
			return 2
		}
	}
	if *downloadRetries < 1 {
		fmt.Fprintf(stderr, "treepack: invalid download retries: must be at least 1\n")
		usage(stderr)
		return 2
	}
	configPath := *config
	token := *githubToken
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	rep, err := build.Build(build.Options{
		ConfigPath:  configPath,
		Source:      *source,
		Output:      *output,
		WorkDir:     *workDir,
		KeepWork:    *keepWork,
		Retries:     *downloadRetries,
		GitHubToken: token,
		Proxy:       *proxy,
		Version:     version,
		Logger:      logging.New(stderr),
		Progress:    stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "treepack: %s\n", err)
		if strings.Contains(err.Error(), "cannot read manifest:") {
			usage(stderr)
		}
		return 1
	}
	fmt.Fprintf(stdout, "built %s into %s\n", rep.Manifest.Pack.Name, rep.Manifest.Paths.Output)
	return 0
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

// usage 将内嵌的帮助文本写入指定输出。
func usage(out io.Writer) {
	fmt.Fprint(out, helpText)
}
