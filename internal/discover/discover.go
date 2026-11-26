// Package discover finds the shell scripts to analyse.
package discover

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// skipped are directories never walked into.
var skipped = map[string]bool{".git": true, "node_modules": true}

// Scripts returns the scripts under paths, sorted and without duplicates.
//
// A file named on the command line is always analysed. In a directory, a
// file is a script when its name ends in .sh or .bash, or when its first
// line is a sh or bash shebang. Paths matching an exclude glob are skipped.
func Scripts(paths []string, exclude []string) ([]string, error) {
	globs := make([]*regexp.Regexp, 0, len(exclude))
	for _, g := range exclude {
		globs = append(globs, Glob(g))
	}
	// A path is matched as written (scripts/x.sh when run on "scripts") and
	// relative to the directory walked (x.sh).
	excluded := func(root, path string) bool {
		candidates := []string{filepath.ToSlash(filepath.Clean(path))}
		if rel, err := filepath.Rel(root, path); err == nil {
			candidates = append(candidates, filepath.ToSlash(rel))
		}
		for _, g := range globs {
			for _, c := range candidates {
				if g.MatchString(c) {
					return true
				}
			}
		}
		return false
	}

	seen := map[string]bool{}
	var scripts []string
	add := func(path string) {
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			scripts = append(scripts, path)
		}
	}
	for _, root := range paths {
		info, err := os.Stat(root)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(root)
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path != root && excluded(root, path) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if path != root && skipped[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type().IsRegular() && IsScript(path) {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(scripts)
	return scripts, nil
}

var shebang = regexp.MustCompile(`^#!\s*\S*/(?:env\s+(?:-\S+\s+)*)?(?:ba)?sh(?:\s|$)`)

// IsScript reports whether path looks like a sh or bash script.
func IsScript(path string) bool {
	switch filepath.Ext(path) {
	case ".sh", ".bash":
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	line, _ := bufio.NewReader(f).ReadString('\n')
	return shebang.MatchString(line)
}

// Glob compiles a gitignore-like glob to a regular expression on slash
// separated relative paths:
//
//   - `*` matches within one path segment, `?` one character;
//   - `**` matches any number of segments;
//   - a glob without a slash matches a file or directory name anywhere
//     ("vendor", "*.generated.sh");
//   - a glob matching a directory excludes everything under it.
func Glob(glob string) *regexp.Regexp {
	glob = strings.TrimPrefix(filepath.ToSlash(glob), "./")
	glob = strings.TrimSuffix(glob, "/")
	var b strings.Builder
	b.WriteString("^")
	if !strings.Contains(glob, "/") {
		b.WriteString("(?:.*/)?")
	}
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch {
		case c == '*' && i+1 < len(glob) && glob[i+1] == '*':
			i++
			if i+1 < len(glob) && glob[i+1] == '/' {
				i++
				b.WriteString("(?:.*/)?")
			} else {
				b.WriteString(".*")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("(?:/.*)?$")
	return regexp.MustCompile(b.String())
}
