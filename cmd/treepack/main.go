package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"

	"treepack/internal/build"
	"treepack/internal/logging"
	"treepack/internal/manifest"
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
	config := "kit.toml"
	source := ""
	output := ""
	workDir := ""
	keepWork := false
	rawArchive := false
	proxy := ""
	githubToken := ""
	downloadRetries := 3
	showVersion := false
	showHelp := false
	fs.StringVar(&config, "config", config, "path to kit.toml")
	fs.StringVar(&config, "c", config, "path to kit.toml")
	fs.StringVar(&source, "source", source, "source directory")
	fs.StringVar(&source, "s", source, "source directory")
	fs.StringVar(&output, "output", output, "output directory")
	fs.StringVar(&output, "o", output, "output directory")
	fs.StringVar(&workDir, "work-dir", workDir, "work directory")
	fs.BoolVar(&keepWork, "keep-work", keepWork, "keep work directory")
	fs.BoolVar(&rawArchive, "raw-archive", rawArchive, "include default metadata entries in generated archive")
	fs.StringVar(&proxy, "proxy", proxy, "proxy URL for downloads")
	fs.StringVar(&proxy, "p", proxy, "proxy URL for downloads")
	fs.StringVar(&githubToken, "github-token", githubToken, "GitHub token for release API and asset downloads")
	fs.IntVar(&downloadRetries, "download-retries", downloadRetries, "total download attempts including the first request")
	fs.BoolVar(&showVersion, "version", showVersion, "show version")
	fs.BoolVar(&showVersion, "v", showVersion, "show version")
	fs.BoolVar(&showHelp, "help", showHelp, "show help")
	fs.BoolVar(&showHelp, "h", showHelp, "show help")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if showHelp {
		usage(stdout)
		return 0
	}
	if showVersion {
		fmt.Fprintf(stdout, "treepack %s\n", version)
		return 0
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "treepack: unexpected argument: %s\n", fs.Arg(0))
		usage(stderr)
		return 2
	}
	if proxy != "" {
		if err := validateProxy(proxy); err != nil {
			fmt.Fprintf(stderr, "treepack: invalid proxy: %s\n", err)
			usage(stderr)
			return 2
		}
	}
	if downloadRetries < 1 {
		fmt.Fprintf(stderr, "treepack: invalid download retries: must be at least 1\n")
		usage(stderr)
		return 2
	}
	token := githubToken
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	rep, err := build.Build(build.Options{
		ConfigPath:  config,
		Source:      source,
		Output:      output,
		WorkDir:     workDir,
		KeepWork:    keepWork,
		RawArchive:  rawArchive,
		Retries:     downloadRetries,
		GitHubToken: token,
		Proxy:       proxy,
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
