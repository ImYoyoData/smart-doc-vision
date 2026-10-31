package logging

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
)

func TestRollingFileRotatesCompressesAndRetains(t *testing.T) {
	dir := t.TempDir()
	writer, err := newRollingFile(dir, "info", 256, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		if _, err := writer.Write([]byte(strings.Repeat("x", 64) + "\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	archives := listArchives(t, dir, "info")
	if len(archives) == 0 {
		t.Fatal("expected rotated archives")
	}
	if len(archives) > 3 {
		t.Fatalf("expected at most 3 archives, got %d: %v", len(archives), archives)
	}
	for _, name := range archives {
		if !strings.HasSuffix(name, ".log.zst") {
			t.Fatalf("expected every archive to be compressed, got %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "info.log")); err != nil {
		t.Fatalf("active log missing: %v", err)
	}
	content := readZstd(t, filepath.Join(dir, archives[0]))
	if !strings.Contains(content, "xxxxxxxx") {
		t.Fatalf("decompressed content looks wrong: %.40q", content)
	}
}

func TestRollingFileRotatesOnDateChange(t *testing.T) {
	dir := t.TempDir()
	writer, err := newRollingFile(dir, "error", 1<<20, 30)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 1, 9, 30, 0, 0, time.Local)
	writer.now = func() time.Time { return base }
	if _, err := writer.Write([]byte("first-day\n")); err != nil {
		t.Fatal(err)
	}
	writer.now = func() time.Time { return base.Add(24 * time.Hour) }
	if _, err := writer.Write([]byte("second-day\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	archives := listArchives(t, dir, "error")
	if len(archives) != 1 {
		t.Fatalf("expected one archive after date change, got %v", archives)
	}
	if !strings.Contains(archives[0], "2026-11-01") {
		t.Fatalf("archive name should carry the rotation date: %s", archives[0])
	}
	if content := readZstd(t, filepath.Join(dir, archives[0])); content != "first-day\n" {
		t.Fatalf("unexpected archived content: %q", content)
	}
	active, err := os.ReadFile(filepath.Join(dir, "error.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(active) != "second-day\n" {
		t.Fatalf("active log should only hold the new day: %q", active)
	}
}

func TestRollingFileCompressesLeftoverArchives(t *testing.T) {
	dir := t.TempDir()
	leftover := filepath.Join(dir, "info-2026-10-31-23-59-59.log")
	if err := os.WriteFile(leftover, []byte("crashed-run\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writer, err := newRollingFile(dir, "info", 1<<20, 30)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Fatalf("leftover archive should be removed after compression, stat err = %v", err)
	}
	if content := readZstd(t, leftover+".zst"); content != "crashed-run\n" {
		t.Fatalf("unexpected compressed content: %q", content)
	}
}

func TestRequestLoggerWritesInfoLine(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := Setup(); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestLogger())
	engine.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ping?x=1", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	Close()

	raw, err := os.ReadFile(filepath.Join("logs", "info", "info.log"))
	if err != nil {
		t.Fatal(err)
	}
	line := string(raw)
	for _, want := range []string{"request", "method=GET", "path=/ping", "status=200", "query=x=1"} {
		if !strings.Contains(line, want) {
			t.Fatalf("info log missing %q: %s", want, line)
		}
	}
}

func listArchives(t *testing.T, dir, prefix string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, prefix+"-") && (strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.zst")) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func readZstd(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := zstd.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
