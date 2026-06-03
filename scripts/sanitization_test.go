package scripts

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepositoryExamplesUseReservedPlaceholders(t *testing.T) {
	root := repoRoot(t)
	allowedExt := map[string]bool{
		".go":   true,
		".md":   true,
		".sh":   true,
		".sql":  true,
		".txt":  true,
		".yml":  true,
		".yaml": true,
	}
	banned := []string{
		"ike-" + "nicholas.xyz",
		"test." + "ike-" + "nicholas.xyz",
		"domain." + "example.com",
		"master." + "example.com",
		"example." + "net",
		"1.1.1." + "1",
		"2.2.2." + "2",
		"1.2.3." + "4",
		"5.6.7." + "8",
		"9.9.9." + "9",
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if !allowedExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		for _, needle := range banned {
			if strings.Contains(text, needle) {
				t.Fatalf("unexpected example residue %q in %s", needle, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(file))
}
