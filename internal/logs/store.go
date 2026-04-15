package logs

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/LFroesch/logdog/internal/config"
)

var ErrNoFiles = errors.New("no log files found")

type FormatKind string

const (
	FormatUnknown FormatKind = "unknown"
	FormatJSON    FormatKind = "json"
	FormatLogfmt  FormatKind = "logfmt"
	FormatPlain   FormatKind = "plain"
)

type Entry struct {
	Timestamp  string         `json:"timestamp,omitempty"`
	Level      string         `json:"level,omitempty"`
	Message    string         `json:"message,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	Raw        string         `json:"-"`
	Source     string         `json:"-"`
	Format     FormatKind     `json:"-"`
	LineNumber int            `json:"-"`
}

type FileInfo struct {
	Path       string
	Root       string
	Name       string
	Size       int64
	ModTime    time.Time
	Format     FormatKind
	Compressed bool
	Rotated    bool
	EntryCount int
}

type CleanupCandidate struct {
	Path    string
	Kind    string
	Size    int64
	ModTime time.Time
}

type Query struct {
	Level   string
	Search  string
	Limit   int
	Reverse bool
}

type Store struct {
	cwd    string
	config config.Config
}

func NewStore() Store {
	cwd, _ := filepath.Abs(".")
	cfg, _ := config.Load()
	return Store{cwd: cwd, config: cfg}
}

func NewStoreWithConfig(cwd string, cfg config.Config) Store {
	abs, _ := filepath.Abs(cwd)
	return Store{cwd: abs, config: cfg}
}

func (s Store) CWD() string {
	return s.cwd
}

func (s Store) Config() config.Config {
	return s.config
}

func (s *Store) ReloadConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	s.config = cfg
	return nil
}

func (s Store) DiscoverFiles() ([]FileInfo, error) {
	rootSet := []string{s.cwd}
	for _, root := range s.config.Roots {
		if root == "" || samePath(root, s.cwd) {
			continue
		}
		rootSet = append(rootSet, root)
	}

	filesByPath := map[string]FileInfo{}
	for _, root := range rootSet {
		if err := s.walkRoot(root, filesByPath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	files := make([]FileInfo, 0, len(filesByPath))
	for _, file := range filesByPath {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool {
		leftGroup := fileSortGroup(files[i])
		rightGroup := fileSortGroup(files[j])
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		if files[i].ModTime.Equal(files[j].ModTime) {
			return files[i].Path < files[j].Path
		}
		return files[i].ModTime.After(files[j].ModTime)
	})
	return files, nil
}

func fileSortGroup(file FileInfo) string {
	return filepath.Clean(filepath.Join(file.Root, filepath.Dir(file.Path)))
}

func (s Store) DiscoverCleanupCandidates() ([]CleanupCandidate, error) {
	files, err := s.DiscoverFiles()
	if err != nil {
		return nil, err
	}

	candidates := make([]CleanupCandidate, 0, len(files))
	for _, file := range files {
		kind := "log"
		if file.Compressed {
			kind = "archive"
		} else if file.Rotated {
			kind = "rotated"
		}
		candidates = append(candidates, CleanupCandidate{
			Path:    file.Path,
			Kind:    kind,
			Size:    file.Size,
			ModTime: file.ModTime,
		})
	}

	emptyDirs, err := s.findEmptyLogDirs()
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, emptyDirs...)

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ModTime.Equal(candidates[j].ModTime) {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].ModTime.After(candidates[j].ModTime)
	})
	return candidates, nil
}

func (s Store) LatestFile() (FileInfo, error) {
	files, err := s.DiscoverFiles()
	if err != nil {
		return FileInfo{}, err
	}
	if len(files) == 0 {
		return FileInfo{}, ErrNoFiles
	}
	return files[0], nil
}

func CountEntries(path string, limit int) int {
	reader, closeFn, err := openMaybeCompressed(path)
	if err != nil {
		return 0
	}
	defer closeFn()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
		if limit > 0 && count >= limit {
			break
		}
	}
	return count
}

func (s Store) ReadEntries(path string, query Query) ([]Entry, error) {
	reader, closeFn, err := openMaybeCompressed(path)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	entries, err := DecodeEntries(reader, path, query)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (s Store) PruneOlderThan(days int) (int, error) {
	if days < 0 {
		days = s.config.CleanupKeepDays
	}
	candidates, err := s.DiscoverCleanupCandidates()
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	deleted := 0
	for _, candidate := range candidates {
		if candidate.ModTime.After(cutoff) {
			continue
		}
		if err := removeCandidate(candidate); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func DeleteCandidates(candidates []CleanupCandidate) (int, error) {
	deleted := 0
	for _, candidate := range candidates {
		if err := removeCandidate(candidate); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func DecodeEntries(r io.Reader, source string, query Query) ([]Entry, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var entries []Entry
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		entry := parseLine(line, source, lineNo)
		if !matches(entry, query) {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if query.Reverse {
		slicesReverse(entries)
	}
	if query.Limit > 0 && len(entries) > query.Limit {
		entries = entries[len(entries)-query.Limit:]
	}
	return entries, nil
}

func TailFile(path string, startAtEnd bool, onEntry func(Entry), onRaw func(string), stop <-chan struct{}) error {
	if isCompressed(path) {
		return fmt.Errorf("cannot tail compressed log file: %s", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if startAtEnd {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			return err
		}
	}

	offset, _ := f.Seek(0, io.SeekCurrent)
	lineNo := 0
	for {
		select {
		case <-stop:
			return nil
		default:
		}

		// Skip the read if the file hasn't grown since we last checked.
		if info, err := f.Stat(); err == nil && info.Size() <= offset {
			time.Sleep(300 * time.Millisecond)
			continue
		}

		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		reader := bufio.NewReader(f)
		for {
			line, err := reader.ReadString('\n')
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			offset += int64(len(line))
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			lineNo++
			entry := parseLine(line, path, lineNo)
			if entry.Format == FormatPlain && onRaw != nil {
				onRaw(entry.Raw)
				continue
			}
			onEntry(entry)
		}
	}
}

func DetectFormat(path string) FormatKind {
	reader, closeFn, err := openMaybeCompressed(path)
	if err != nil {
		return FormatUnknown
	}
	defer closeFn()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	sample := make([]string, 0, 12)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		sample = append(sample, line)
		if len(sample) >= 12 {
			break
		}
	}
	return detectFormat(sample)
}

func FormatEntry(entry Entry) string {
	ts := formatTimestamp(entry.Timestamp)
	level := strings.ToUpper(entry.Level)
	if level == "" {
		level = "LINE"
	}
	if level != "LINE" && len(level) < 5 {
		level += strings.Repeat(" ", 5-len(level))
	}

	var details []string
	for key, value := range entry.Data {
		details = append(details, fmt.Sprintf("%s=%v", key, value))
	}
	sort.Strings(details)

	switch {
	case ts == "-" && len(details) == 0 && entry.Message != "":
		return entry.Message
	case len(details) == 0:
		return fmt.Sprintf("%s %s %s", ts, level, entry.Message)
	default:
		return fmt.Sprintf("%s %s %s {%s}", ts, level, entry.Message, strings.Join(details, " "))
	}
}

func parseLine(line, source string, lineNo int) Entry {
	entry := Entry{
		Raw:        line,
		Source:     source,
		Format:     FormatPlain,
		LineNumber: lineNo,
	}

	if parsed, ok := parseJSONLine(line); ok {
		parsed.Raw = line
		parsed.Source = source
		parsed.Format = FormatJSON
		parsed.LineNumber = lineNo
		return parsed
	}
	if parsed, ok := parseLogfmtLine(line); ok {
		parsed.Raw = line
		parsed.Source = source
		parsed.Format = FormatLogfmt
		parsed.LineNumber = lineNo
		return parsed
	}

	ts, level, msg := extractPlainParts(line)
	entry.Timestamp = ts
	entry.Level = level
	entry.Message = msg
	return entry
}

func parseJSONLine(line string) (Entry, bool) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err != nil {
		return Entry{}, false
	}
	entry := Entry{
		Timestamp: firstString(payload, "timestamp", "time", "ts"),
		Level:     strings.ToUpper(firstString(payload, "level", "severity", "lvl")),
		Message:   firstString(payload, "message", "msg", "event"),
		Data:      map[string]any{},
	}

	if nested, ok := payload["data"].(map[string]any); ok {
		for key, value := range nested {
			entry.Data[key] = value
		}
		delete(payload, "data")
	}
	if nested, ok := payload["fields"].(map[string]any); ok {
		for key, value := range nested {
			entry.Data[key] = value
		}
		delete(payload, "fields")
	}

	delete(payload, "timestamp")
	delete(payload, "time")
	delete(payload, "ts")
	delete(payload, "level")
	delete(payload, "severity")
	delete(payload, "lvl")
	delete(payload, "message")
	delete(payload, "msg")
	delete(payload, "event")

	for key, value := range payload {
		entry.Data[key] = value
	}
	if entry.Message == "" {
		entry.Message = line
	}
	return entry, true
}

func parseLogfmtLine(line string) (Entry, bool) {
	fields := map[string]any{}
	parts := splitLogfmt(line)
	pairs := 0
	for _, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" {
			continue
		}
		pairs++
		value = strings.Trim(value, `"`)
		fields[key] = value
	}
	if pairs < 2 {
		return Entry{}, false
	}

	entry := Entry{
		Timestamp: stringValue(fields, "timestamp", "time", "ts"),
		Level:     strings.ToUpper(stringValue(fields, "level", "severity", "lvl")),
		Message:   stringValue(fields, "message", "msg"),
		Data:      map[string]any{},
	}
	delete(fields, "timestamp")
	delete(fields, "time")
	delete(fields, "ts")
	delete(fields, "level")
	delete(fields, "severity")
	delete(fields, "lvl")
	delete(fields, "message")
	delete(fields, "msg")

	for key, value := range fields {
		entry.Data[key] = value
	}
	if entry.Message == "" {
		entry.Message = line
	}
	return entry, true
}

func extractPlainParts(line string) (string, string, string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", "", line
	}

	level := ""
	message := line
	timestamp := ""

	if len(fields) >= 1 && looksLikeTimestamp(fields[0]) {
		timestamp = fields[0]
		message = strings.TrimSpace(strings.TrimPrefix(message, fields[0]))
		if len(fields) >= 2 && looksLikeTimestamp(fields[0]+" "+fields[1]) {
			timestamp = fields[0] + " " + fields[1]
			message = strings.TrimSpace(strings.TrimPrefix(line, timestamp))
		}
	}

	remaining := strings.Fields(message)
	if len(remaining) > 0 {
		candidate := strings.Trim(remaining[0], "[]():")
		upper := strings.ToUpper(candidate)
		switch upper {
		case "ERROR", "WARN", "WARNING", "INFO", "DEBUG", "TRACE", "FATAL":
			level = upper
			message = strings.TrimSpace(strings.TrimPrefix(message, remaining[0]))
		}
	}
	return timestamp, level, message
}

func matches(entry Entry, query Query) bool {
	if query.Level != "" && !strings.EqualFold(entry.Level, query.Level) {
		return false
	}
	if query.Search == "" {
		return true
	}
	q := strings.ToLower(query.Search)
	if strings.Contains(strings.ToLower(entry.Message), q) || strings.Contains(strings.ToLower(entry.Raw), q) {
		return true
	}
	for key, value := range entry.Data {
		if strings.Contains(strings.ToLower(key), q) || strings.Contains(strings.ToLower(fmt.Sprintf("%v", value)), q) {
			return true
		}
	}
	return false
}

func (s Store) walkRoot(root string, filesByPath map[string]FileInfo) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			return nil
		}

		if d.IsDir() {
			if path != root && shouldIgnore(path, s.config.IgnorePatterns) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || shouldIgnore(path, s.config.IgnorePatterns) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		if s.config.MaxFileBytes > 0 && info.Size() > s.config.MaxFileBytes {
			return nil
		}
		if !isLogCandidate(path, s.config.Extensions) {
			return nil
		}

		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		filesByPath[abs] = FileInfo{
			Path:       abs,
			Root:       root,
			Name:       filepath.Base(path),
			Size:       info.Size(),
			ModTime:    info.ModTime(),
			Format:     FormatUnknown,
			Compressed: isCompressed(abs),
			Rotated:    isRotated(abs),
			EntryCount: 0,
		}
		return nil
	})
}

func (s Store) findEmptyLogDirs() ([]CleanupCandidate, error) {
	roots := []string{s.cwd}
	for _, root := range s.config.Roots {
		if !samePath(root, s.cwd) {
			roots = append(roots, root)
		}
	}

	var candidates []CleanupCandidate
	seen := map[string]struct{}{}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			if path != root && shouldIgnore(path, s.config.IgnorePatterns) {
				return filepath.SkipDir
			}
			if samePath(path, root) {
				return nil
			}
			entries, err := os.ReadDir(path)
			if err != nil || len(entries) != 0 {
				return nil
			}
			if !strings.Contains(strings.ToLower(filepath.Base(path)), "log") {
				parentEntries, err := os.ReadDir(filepath.Dir(path))
				if err != nil || len(parentEntries) == 0 {
					return nil
				}
			}
			info, err := os.Stat(path)
			if err != nil {
				return nil
			}
			abs, _ := filepath.Abs(path)
			if _, ok := seen[abs]; ok {
				return nil
			}
			seen[abs] = struct{}{}
			candidates = append(candidates, CleanupCandidate{
				Path:    abs,
				Kind:    "empty-dir",
				Size:    0,
				ModTime: info.ModTime(),
			})
			return nil
		})
	}
	return candidates, nil
}

func detectFormat(sample []string) FormatKind {
	if len(sample) == 0 {
		return FormatUnknown
	}
	jsonCount := 0
	logfmtCount := 0
	for _, line := range sample {
		if _, ok := parseJSONLine(line); ok {
			jsonCount++
			continue
		}
		if _, ok := parseLogfmtLine(line); ok {
			logfmtCount++
		}
	}
	if jsonCount >= max(1, len(sample)/2) {
		return FormatJSON
	}
	if logfmtCount >= max(1, len(sample)/2) {
		return FormatLogfmt
	}
	return FormatPlain
}

func splitLogfmt(line string) []string {
	var parts []string
	var buf strings.Builder
	inQuote := false
	for _, r := range line {
		switch r {
		case '"':
			inQuote = !inQuote
			buf.WriteRune(r)
		case ' ', '\t':
			if inQuote {
				buf.WriteRune(r)
				continue
			}
			if buf.Len() > 0 {
				parts = append(parts, buf.String())
				buf.Reset()
			}
		default:
			buf.WriteRune(r)
		}
	}
	if buf.Len() > 0 {
		parts = append(parts, buf.String())
	}
	return parts
}

func shouldIgnore(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if strings.Contains(path, string(filepath.Separator)+pattern+string(filepath.Separator)) ||
			strings.HasSuffix(path, string(filepath.Separator)+pattern) ||
			filepath.Base(path) == pattern {
			return true
		}
	}
	return false
}

func isLogCandidate(path string, extensions []string) bool {
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(base))
	explicitExt := false
	for _, allowed := range extensions {
		if ext == strings.ToLower(allowed) {
			explicitExt = true
			break
		}
	}

	if isRotated(path) || isCompressed(path) || strings.HasSuffix(base, ".out") || strings.HasSuffix(base, ".err") {
		return true
	}
	if ext == ".log" {
		return true
	}

	pathHint := hasLogPathHint(path)
	if pathHint && explicitExt {
		return true
	}
	if hasLogNameHint(base) {
		return true
	}
	if !explicitExt {
		return false
	}
	return contentLooksLikeLog(path)
}

func hasLogPathHint(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(strings.ToLower(path)), "/") {
		if part == "log" || part == "logs" {
			return true
		}
	}
	return false
}

func hasLogNameHint(base string) bool {
	name := strings.TrimSuffix(base, filepath.Ext(base))
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	for _, field := range fields {
		if field == "log" || field == "logs" {
			return true
		}
	}
	if strings.HasSuffix(name, ".log") || strings.HasSuffix(name, "_log") || strings.HasSuffix(name, "-log") {
		return true
	}
	return false
}

func contentLooksLikeLog(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(io.LimitReader(f, 128*1024))
	scanner.Buffer(make([]byte, 0, 8*1024), 128*1024)

	nonEmpty := 0
	loggy := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		nonEmpty++
		if lineLooksLikeLog(line) {
			loggy++
		}
		if nonEmpty >= 12 {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return false
	}
	if nonEmpty == 0 {
		return false
	}
	if nonEmpty == 1 {
		return loggy == 1
	}
	return loggy*2 >= nonEmpty
}

func lineLooksLikeLog(line string) bool {
	if parsed, ok := parseJSONLine(line); ok {
		return parsed.Timestamp != "" || parsed.Level != "" || parsed.Message != line
	}
	if parsed, ok := parseLogfmtLine(line); ok {
		return parsed.Timestamp != "" || parsed.Level != "" || parsed.Message != line
	}
	ts, level, msg := extractPlainParts(line)
	if ts != "" || level != "" {
		return true
	}
	return msg != "" && msg != line && len(strings.Fields(line)) >= 3
}

func isCompressed(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, ".gz")
}

func isRotated(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if strings.Contains(base, ".log.") {
		return true
	}
	parts := strings.Split(base, ".")
	if len(parts) < 2 {
		return false
	}
	last := parts[len(parts)-1]
	if _, err := strconv.Atoi(last); err == nil {
		return true
	}
	if len(parts) >= 2 {
		prev := parts[len(parts)-2]
		if _, err := strconv.Atoi(prev); err == nil {
			return true
		}
	}
	return false
}

func openMaybeCompressed(path string) (io.Reader, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	closeFn := f.Close
	if !isCompressed(path) {
		return f, closeFn, nil
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return gz, func() error {
		_ = gz.Close()
		return f.Close()
	}, nil
}

func removeCandidate(candidate CleanupCandidate) error {
	if candidate.Kind == "empty-dir" {
		return os.Remove(candidate.Path)
	}
	return os.Remove(candidate.Path) // files only — os.Remove fails on non-empty dirs by design
}

func stringValue(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			switch typed := value.(type) {
			case string:
				return typed
			default:
				return fmt.Sprintf("%v", typed)
			}
		}
	}
	return ""
}

func firstString(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := fields[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		default:
			return fmt.Sprintf("%v", typed)
		}
	}
	return ""
}

func formatTimestamp(value string) string {
	if value == "" {
		return "-"
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", time.RFC3339Nano} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02 15:04:05")
		}
	}
	return value
}

func looksLikeTimestamp(value string) bool {
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05"} {
		if _, err := time.Parse(layout, value); err == nil {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return aa == bb
}

func slicesReverse(entries []Entry) {
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
}

