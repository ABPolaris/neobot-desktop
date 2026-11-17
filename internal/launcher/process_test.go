package launcher

import (
	"path/filepath"
	"strings"
	"testing"
)

func envMap(entries []string) map[string]string {
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		if name, value, ok := strings.Cut(entry, "="); ok {
			values[strings.ToUpper(name)] = value
		}
	}
	return values
}

func TestBackendEnvExtraUsesDesktopPackageIndex(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	index := "https://pypi.tuna.tsinghua.edu.cn/simple"
	env := envMap(backendEnvExtra(root, data, Sources{Index: index}))
	want := map[string]string{
		"UV_DEFAULT_INDEX": index,
		"PIP_INDEX_URL":    index,
		"UV_CACHE_DIR":     filepath.Join(root, "cache"),
		"NEOBOT_DATA_DIR":  data,
	}
	for name, value := range want {
		if env[name] != value {
			t.Fatalf("%s = %q; want %q", name, env[name], value)
		}
	}
}

func TestBackendEnvExtraSkipsEmptyIndex(t *testing.T) {
	env := envMap(backendEnvExtra(t.TempDir(), "data", Sources{}))
	for _, name := range []string{"UV_DEFAULT_INDEX", "PIP_INDEX_URL"} {
		if value, ok := env[name]; ok {
			t.Fatalf("%s must not be set without an index: %q", name, value)
		}
	}
}
