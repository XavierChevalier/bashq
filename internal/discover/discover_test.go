package discover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScripts(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.sh", "echo a\n")
	write(t, root, "lib/b.bash", "echo b\n")
	write(t, root, "bin/tool", "#!/usr/bin/env bash\necho tool\n")
	write(t, root, "bin/posix", "#!/bin/sh\necho posix\n")
	write(t, root, "bin/env-flags", "#!/usr/bin/env -S bash -eu\necho e\n")
	write(t, root, "bin/python", "#!/usr/bin/env python3\nprint()\n")
	write(t, root, "bin/zsh", "#!/bin/zsh\necho z\n")
	write(t, root, "README.md", "# sh\n")
	write(t, root, "vendor/dep.sh", "echo dep\n")
	write(t, root, "node_modules/x.sh", "echo x\n")
	write(t, root, ".git/hooks/pre-commit.sh", "echo hook\n")
	write(t, root, "gen/out.generated.sh", "echo gen\n")

	got, err := Scripts([]string{root}, []string{"vendor", "*.generated.sh"})
	if err != nil {
		t.Fatal(err)
	}
	var rel []string
	for _, p := range got {
		r, _ := filepath.Rel(root, p)
		rel = append(rel, filepath.ToSlash(r))
	}
	want := "a.sh bin/env-flags bin/posix bin/tool lib/b.bash"
	if strings.Join(rel, " ") != want {
		t.Errorf("scripts = %v, want %s", rel, want)
	}
}

func TestANamedFileIsAlwaysAnalysed(t *testing.T) {
	root := t.TempDir()
	write(t, root, "hook", "echo no shebang\n")
	got, err := Scripts([]string{filepath.Join(root, "hook"), filepath.Join(root, "hook")}, nil)
	if err != nil || len(got) != 1 {
		t.Fatalf("scripts = %v, %v", got, err)
	}
}

func TestMissingPath(t *testing.T) {
	if _, err := Scripts([]string{"does/not/exist"}, nil); err == nil {
		t.Fatal("want an error")
	}
}

func TestGlob(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"vendor", "vendor", true},
		{"vendor", "a/vendor/x.sh", true},
		{"vendor", "vendors/x.sh", false},
		{"*.gen.sh", "a/b/c.gen.sh", true},
		{"scripts/*.sh", "scripts/a.sh", true},
		{"scripts/*.sh", "scripts/sub/a.sh", false},
		{"scripts/**/*.sh", "scripts/sub/deep/a.sh", true},
		{"scripts/**/*.sh", "scripts/a.sh", true},
		{"**/testdata", "x/y/testdata/f.sh", true},
		{"./docs/", "docs/a.sh", true},
		{"a?.sh", "ab.sh", true},
		{"a?.sh", "a/.sh", false},
	}
	for _, tc := range cases {
		if got := Glob(tc.glob).MatchString(tc.path); got != tc.want {
			t.Errorf("Glob(%q) on %q = %v, want %v", tc.glob, tc.path, got, tc.want)
		}
	}
}
