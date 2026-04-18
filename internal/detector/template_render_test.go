package detector

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestTemplateRendersValidGo runs `go vet` on the generated logger.go to make
// sure the template compiles. Without this, template breakage only shows up
// after a user installs.
func TestTemplateRendersValidGo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	g := &GoLanguage{}
	cfg := Config{
		LogLevel:   "INFO",
		OutputDir:  filepath.Join(dir, "out"),
		MaxFiles:   30,
		DateFormat: "2006-01-02",
	}
	if err := g.Install(dir, cfg); err != nil {
		t.Fatalf("install: %v", err)
	}

	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet failed: %v\n%s", err, out)
	}
}
