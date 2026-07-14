package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"treepack/internal/fsutil"

	"golang.org/x/text/unicode/norm"
)

type archiveOutput interface {
	io.Writer
	io.Closer
	Name() string
}

type Options struct {
	Raw bool
}

var createArchiveFile = func(dir, pattern string) (archiveOutput, error) {
	return os.CreateTemp(dir, pattern)
}

type ExtractionPlan struct {
	Entries     []PlannedEntry
	Corrections []PathCorrection
}

type PlannedEntry struct {
	File      *zip.File
	Original  string
	Extracted string
	IsDir     bool
}

type PathCorrection struct {
	Original  string
	Extracted string
	Reasons   []string
}

type ExtractResult struct {
	Corrections []PathCorrection
}

const maxPortableComponentBytes = 255

// ExtractZip preflights the complete central directory and extracts through a
// temporary directory so a rejected or corrupt archive cannot partially modify outputDir.
func ExtractZip(archivePath, outputDir string) (ExtractResult, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return ExtractResult{}, err
	}
	defer reader.Close()
	plan, err := planExtraction(reader.File)
	if err != nil {
		return ExtractResult{}, err
	}
	parent := filepath.Dir(outputDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return ExtractResult{}, err
	}
	tmp, err := os.MkdirTemp(parent, ".treepack-extract-*")
	if err != nil {
		return ExtractResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(tmp)
		}
	}()
	for _, entry := range plan.Entries {
		if err := extractPlannedEntry(entry, tmp); err != nil {
			return ExtractResult{}, err
		}
	}
	if err := fsutil.Remove(outputDir); err != nil {
		return ExtractResult{}, err
	}
	if err := os.Rename(tmp, outputDir); err != nil {
		return ExtractResult{}, err
	}
	committed = true
	return ExtractResult{Corrections: plan.Corrections}, nil
}

func planExtraction(files []*zip.File) (ExtractionPlan, error) {
	plan := ExtractionPlan{Entries: make([]PlannedEntry, 0, len(files))}
	originals := map[string]bool{}
	targets := map[string]PlannedEntry{}
	for _, file := range files {
		original := file.Name
		if original == "" || !utf8.ValidString(original) {
			return ExtractionPlan{}, fmt.Errorf("zip entry has an empty or invalid UTF-8 name")
		}
		if originals[original] {
			return ExtractionPlan{}, fmt.Errorf("zip contains duplicate entry: %s", original)
		}
		originals[original] = true
		if isAbsoluteArchivePath(original) {
			return ExtractionPlan{}, fmt.Errorf("zip entry uses an absolute or drive path: %s", original)
		}
		mode := file.FileInfo().Mode()
		if mode&os.ModeSymlink != 0 {
			return ExtractionPlan{}, fmt.Errorf("zip symlink entries are not supported: %s", original)
		}
		if !mode.IsDir() && !mode.IsRegular() {
			return ExtractionPlan{}, fmt.Errorf("zip special file entries are not supported: %s", original)
		}
		isDir := mode.IsDir() || strings.HasSuffix(original, "/")
		trimmed := strings.TrimSuffix(original, "/")
		if trimmed == "" {
			return ExtractionPlan{}, fmt.Errorf("zip entry has an empty name: %s", original)
		}
		parts := strings.Split(trimmed, "/")
		correctedParts := make([]string, len(parts))
		reasons := []string{}
		for i, component := range parts {
			if component == "" || component == "." || component == ".." {
				return ExtractionPlan{}, fmt.Errorf("zip entry contains an unsafe path component: %s", original)
			}
			corrected, componentReasons := correctZipComponent(component)
			if len([]byte(corrected)) > maxPortableComponentBytes {
				return ExtractionPlan{}, fmt.Errorf("zip entry component exceeds %d bytes: %s", maxPortableComponentBytes, original)
			}
			correctedParts[i] = corrected
			reasons = appendUnique(reasons, componentReasons...)
		}
		extracted := strings.Join(correctedParts, "/")
		entry := PlannedEntry{File: file, Original: original, Extracted: extracted, IsDir: isDir}
		key := portablePathKey(extracted)
		if previous, exists := targets[key]; exists {
			return ExtractionPlan{}, fmt.Errorf("zip entries conflict after portable path normalization: %s and %s", previous.Original, original)
		}
		targets[key] = entry
		plan.Entries = append(plan.Entries, entry)
		shownExtracted := extracted
		if strings.HasSuffix(original, "/") {
			shownExtracted += "/"
		}
		if extracted != trimmed {
			plan.Corrections = append(plan.Corrections, PathCorrection{Original: original, Extracted: shownExtracted, Reasons: reasons})
		}
	}
	for _, entry := range plan.Entries {
		parts := strings.Split(entry.Extracted, "/")
		for i := 1; i < len(parts); i++ {
			if parent, exists := targets[portablePathKey(strings.Join(parts[:i], "/"))]; exists && !parent.IsDir {
				return ExtractionPlan{}, fmt.Errorf("zip file is the parent of another entry: %s and %s", parent.Original, entry.Original)
			}
		}
	}
	return plan, nil
}

func extractPlannedEntry(entry PlannedEntry, root string) error {
	target := filepath.Join(root, filepath.FromSlash(entry.Extracted))
	if entry.IsDir {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	src, err := entry.File.Open()
	if err != nil {
		return err
	}
	perm := entry.File.FileInfo().Mode().Perm()
	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		_ = src.Close()
		return err
	}
	_, copyErr := io.Copy(dst, src)
	dstErr := dst.Close()
	srcErr := src.Close()
	if copyErr != nil {
		return copyErr
	}
	if dstErr != nil {
		return dstErr
	}
	if srcErr != nil {
		return srcErr
	}
	_ = os.Chmod(target, perm)
	return nil
}

// CorrectZipPath applies archive component corrections to a safe relative literal path.
// It is used by package steps that refer to names before extraction correction.
func CorrectZipPath(value string) (string, []PathCorrection) {
	parts := strings.Split(filepath.ToSlash(value), "/")
	correctedParts := append([]string(nil), parts...)
	var reasons []string
	changed := false
	for i, component := range parts {
		if component == "" || component == "." || component == ".." {
			continue
		}
		corrected, componentReasons := correctZipComponent(component)
		correctedParts[i] = corrected
		if corrected != component {
			changed = true
			reasons = appendUnique(reasons, componentReasons...)
		}
	}
	corrected := strings.Join(correctedParts, "/")
	if !changed {
		return value, nil
	}
	return corrected, []PathCorrection{{Original: value, Extracted: corrected, Reasons: reasons}}
}

func correctZipComponent(name string) (string, []string) {
	var b strings.Builder
	var reasons []string
	for _, r := range name {
		if r == 0 || unicode.IsControl(r) {
			b.WriteByte('_')
			reasons = appendUnique(reasons, "control character")
			continue
		}
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteByte('_')
			reasons = appendUnique(reasons, "unsupported portable character")
			continue
		}
		b.WriteRune(r)
	}
	corrected := b.String()
	deviceName := isWindowsDeviceComponent(strings.TrimRight(corrected, ". "))
	runes := []rune(corrected)
	for i := len(runes) - 1; i >= 0 && (runes[i] == '.' || runes[i] == ' '); i-- {
		if runes[i] == '.' {
			reasons = appendUnique(reasons, "trailing dot")
		} else {
			reasons = appendUnique(reasons, "trailing space")
		}
		runes[i] = '_'
	}
	corrected = string(runes)
	if deviceName {
		corrected = "_" + corrected
		reasons = appendUnique(reasons, "Windows reserved device name")
	}
	return corrected, reasons
}

func isWindowsDeviceComponent(value string) bool {
	base := value
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	base = strings.ToUpper(base)
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9'
}

func isAbsoluteArchivePath(name string) bool {
	return strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) || strings.HasPrefix(name, "//") || strings.HasPrefix(name, `\\`) || (len(name) >= 2 && ((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z')) && name[1] == ':') || path.IsAbs(name)
}

func portablePathKey(value string) string {
	return strings.ToLower(norm.NFC.String(value))
}

func appendUnique(values []string, additions ...string) []string {
	for _, addition := range additions {
		found := false
		for _, value := range values {
			if value == addition {
				found = true
				break
			}
		}
		if !found {
			values = append(values, addition)
		}
	}
	return values
}

// MakeZip 从源目录创建 zip 归档，并通过临时文件保证失败时不替换既有归档。
func MakeZip(sourceDir, archivePath string, options Options) error {
	archiveDir := filepath.Dir(archivePath)
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(archivePath); err == nil && info.IsDir() {
		return fmt.Errorf("archive path is an existing directory: %s", archivePath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	out, err := createArchiveFile(archiveDir, ".treepack-*.zip")
	if err != nil {
		return err
	}
	tmpPath := out.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	writer := zip.NewWriter(out)
	writeErr := writeZip(sourceDir, writer, options)
	closeErr := out.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Remove(archivePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(tmpPath, archivePath); err != nil {
		return err
	}
	committed = true
	return nil
}

// writeZip 按稳定顺序把源目录中的普通文件和真实目录写入 zip writer。
func writeZip(sourceDir string, writer *zip.Writer, options Options) error {
	var entries []archiveEntry
	if err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == sourceDir {
			return nil
		}
		_, kind, err := fsutil.ValidateEntry(path)
		if err != nil {
			return err
		}
		if !options.Raw && skipDefaultArchiveEntry(d.Name(), kind) {
			if kind == fsutil.EntryDir {
				return filepath.SkipDir
			}
			return nil
		}
		if kind == fsutil.EntryFile || kind == fsutil.EntryDir {
			rel, err := filepath.Rel(sourceDir, path)
			if err != nil {
				return err
			}
			name := filepath.ToSlash(rel)
			if kind == fsutil.EntryDir {
				name += "/"
			}
			entries = append(entries, archiveEntry{path: path, name: name, kind: kind})
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})
	for _, entry := range entries {
		info, _, err := fsutil.ValidateEntry(entry.path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = entry.name
		if entry.kind == fsutil.EntryDir {
			header.Method = zip.Store
			if _, err := writer.CreateHeader(header); err != nil {
				return err
			}
			continue
		}
		header.Method = zip.Deflate
		dst, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		in, err := os.Open(entry.path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(dst, in)
		in.Close()
		if copyErr != nil {
			writer.Close()
			return copyErr
		}
	}
	return writer.Close()
}

// isRealDir 判断文件信息是否表示真实目录而不是其他目录类特殊项。
func isRealDir(info os.FileInfo) bool {
	return info.IsDir() && info.Mode()&os.ModeType == os.ModeDir
}

// skipDefaultArchiveEntry identifies common desktop metadata that should not be included in generated archives.
func skipDefaultArchiveEntry(name string, kind fsutil.EntryKind) bool {
	if kind == fsutil.EntryDir {
		switch name {
		case "__MACOSX", ".AppleDouble", "$RECYCLE.BIN", "System Volume Information":
			return true
		}
		return strings.HasPrefix(name, ".Trash-")
	}
	if kind == fsutil.EntryFile {
		switch name {
		case ".DS_Store", "Thumbs.db", "ehthumbs.db", "Desktop.ini", ".directory":
			return true
		}
		return strings.HasPrefix(name, "._")
	}
	return false
}

type archiveEntry struct {
	path string
	name string
	kind fsutil.EntryKind
}
