package engine

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// Ignore decides which paths a sync leaves behind. Patterns follow the rules
// people already know from git's ignore files, because that is what "robust
// ignore" means in practice:
//
//	dist            a file or directory named dist, at any depth
//	dist/           a directory named dist, at any depth
//	/dist           only the dist at the root of the source
//	*.exe           any file ending in .exe, at any depth
//	cmd/**/tmp      anything under a cmd/.../tmp path
//	!keep.exe       re-include something an earlier pattern excluded
//	# comment       ignored, like a blank line
//
// A pattern that matches a directory drops everything inside it, exactly like
// git (so there is no point in re-including a file whose parent is excluded).
type Ignore struct {
	rules []ignoreRule
}

type ignoreRule struct {
	expr    string
	re      *regexp.Regexp
	tree    *regexp.Regexp
	dirOnly bool
	negate  bool
}

// NewIgnore builds an empty set: nothing is excluded.
func NewIgnore() *Ignore { return &Ignore{} }

// Empty reports whether nothing would be excluded, so callers can skip the
// work of matching entirely.
func (ig *Ignore) Empty() bool { return len(ig.rules) == 0 }

// Add records one pattern. Blank lines and comments are ignored, so a file can
// be fed in line by line.
func (ig *Ignore) Add(pattern string) {
	line := strings.TrimSpace(pattern)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}

	// An escaped # or ! is a literal first character.
	if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
		line = line[1:]
	}

	rule := ignoreRule{expr: line}
	if strings.HasPrefix(line, "!") {
		rule.negate = true
		line = strings.TrimPrefix(line, "!")
	}
	if strings.HasSuffix(line, "/") {
		rule.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	anchored := strings.HasPrefix(line, "/")
	line = strings.TrimPrefix(line, "/")
	// A pattern with a slash inside it is relative to the root; one without
	// matches at any depth.
	anchored = anchored || strings.Contains(line, "/")

	if line == "" {
		return
	}

	body := globToRegex(line, anchored)
	re, err := regexp.Compile("^" + body + "$")
	if err != nil {
		return
	}
	tree, err := regexp.Compile("^" + body + "(?:/.*)?$")
	if err != nil {
		return
	}

	rule.re = re
	rule.tree = tree
	ig.rules = append(ig.rules, rule)
}

// AddFile reads patterns from a file, one per line. A missing file is not an
// error: an ignore file is optional.
func (ig *Ignore) AddFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ig.Add(scanner.Text())
	}
	return scanner.Err()
}

// Patterns lists the patterns in the order they were added, for messages.
func (ig *Ignore) Patterns() []string {
	out := make([]string, 0, len(ig.rules))
	for _, rule := range ig.rules {
		out = append(out, rule.expr)
	}
	return out
}

// Match reports whether rel — a slash separated path relative to the source —
// is ignored. Later patterns win, so "!keep" after "*.log" brings keep back.
func (ig *Ignore) Match(rel string, isDir bool) bool {
	if len(ig.rules) == 0 || rel == "" || rel == "." {
		return false
	}
	rel = strings.ReplaceAll(rel, `\`, "/")

	ignored := false
	for _, rule := range ig.rules {
		if rule.matches(rel, isDir) {
			ignored = !rule.negate
		}
	}
	return ignored
}

// matches reports whether one rule covers a path: the entry itself (where a
// directory-only pattern also asks the entry to be a directory) or anything
// inside a directory it covers.
func (r ignoreRule) matches(rel string, isDir bool) bool {
	if r.re.MatchString(rel) {
		return !r.dirOnly || isDir
	}
	return r.tree.MatchString(rel)
}

// globToRegex turns one ignore pattern into the body of a regular expression
// matching a whole relative path.
func globToRegex(pattern string, anchored bool) string {
	var b strings.Builder
	if !anchored {
		// No slash in the pattern: it matches at any depth.
		b.WriteString("(?:.*/)?")
	}

	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				// "**/" spans any number of directories, a bare "**" spans
				// everything.
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
				continue
			}
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '/':
			b.WriteString("/")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}

	return b.String()
}
