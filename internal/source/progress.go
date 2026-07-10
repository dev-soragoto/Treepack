package source

import (
	"fmt"
	"io"
)

type downloadProgress struct {
	name        string
	total       int64
	written     int64
	nextPercent int64
	nextBytes   int64
	out         io.Writer
}

// newDownloadProgress 创建按百分比或字节数输出的下载进度计数器。
func newDownloadProgress(name string, total int64, out io.Writer) *downloadProgress {
	if out == nil {
		out = io.Discard
	}
	p := &downloadProgress{name: name, total: total, nextPercent: 10, nextBytes: 5 * 1024 * 1024, out: out}
	if total > 0 {
		fmt.Fprintf(out, "downloading %s: 0%%\n", name)
	} else {
		fmt.Fprintf(out, "downloading %s\n", name)
	}
	return p
}

// Write 记录下载进度并在达到阈值时输出进度日志。
func (p *downloadProgress) Write(data []byte) (int, error) {
	n := len(data)
	p.written += int64(n)
	if p.total > 0 {
		percent := p.written * 100 / p.total
		for percent >= p.nextPercent && p.nextPercent < 100 {
			fmt.Fprintf(p.out, "downloading %s: %d%%\n", p.name, p.nextPercent)
			p.nextPercent += 10
		}
		return n, nil
	}
	for p.written >= p.nextBytes {
		fmt.Fprintf(p.out, "downloading %s: %d MiB\n", p.name, p.nextBytes/(1024*1024))
		p.nextBytes += 5 * 1024 * 1024
	}
	return n, nil
}

// done 输出下载完成时的最终进度信息。
func (p *downloadProgress) done() {
	if p.total > 0 {
		fmt.Fprintf(p.out, "downloaded %s: 100%%\n", p.name)
		return
	}
	fmt.Fprintf(p.out, "downloaded %s: %d bytes\n", p.name, p.written)
}
