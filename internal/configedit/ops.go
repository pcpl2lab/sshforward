package configedit

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The operations below return the new file content and leave d unchanged;
// d still describes the original source, so re-Parse the result to edit it.

// Add appends s at the end of the services section, after its last line
// indented deeper than the services key. Without a services key, the key and
// the block are appended at the end of the file. An explicit null value
// (services: ~) is removed from the key line, its inline comment kept.
func (d *Document) Add(s Service) []byte {
	block := Render(s, d.base, d.step)
	if d.servicesLine < 0 {
		head := ensureEOL(strings.Join(d.lines, ""), d.eol)
		return []byte(head + "services:" + d.eol + d.joinNew(block, true))
	}

	lines := d.lines
	if d.nullCol >= 0 {
		// services: ~ must become services: before entries can follow it.
		lines = append([]string(nil), d.lines...)
		line := lines[d.servicesLine]
		lines[d.servicesLine] = strings.TrimRight(line[:d.nullCol], " 	") + line[d.nullCol+d.nullLen:]
	}
	last := d.sectionEnd()
	if last > d.servicesLine && isComment(trimEOL(lines[last])) {
		block = append([]string{""}, block...)
	}
	var b strings.Builder
	for _, l := range lines[:last] {
		b.WriteString(l)
	}
	b.WriteString(ensureEOL(lines[last], d.eol))
	b.WriteString(d.joinNew(block, true))
	for _, l := range lines[last+1:] {
		b.WriteString(l)
	}
	return []byte(b.String())
}

// sectionEnd returns the last line of the services section: the last
// non-blank line after the services key indented deeper than it. A line at
// or left of the key's column ends the section, unless it is a comment.
func (d *Document) sectionEnd() int {
	last := d.servicesLine
	for i := d.servicesLine + 1; i < len(d.lines); i++ {
		line := trimEOL(d.lines[i])
		if isBlank(line) {
			continue
		}
		if indentOf(line) > d.servicesCol {
			last = i
		} else if !isComment(line) {
			break
		}
	}
	return last
}

// Replace swaps the lines of service name for s rendered at the same indent.
// The head comment above the key and the lines after the value stay.
func (d *Document) Replace(name string, s Service) ([]byte, error) {
	i, err := d.find(name)
	if err != nil {
		return nil, err
	}
	if s.Name != name {
		if _, err := d.find(s.Name); err == nil {
			return nil, fmt.Errorf("service %q already exists", s.Name)
		}
	}
	sp := d.spans[i]
	key := trimEOL(d.lines[sp.keyLine])
	block := Render(s, key[:indentOf(key)], d.step)
	// Keep a missing final newline missing.
	final := strings.HasSuffix(d.lines[sp.endLine], "\n")
	return d.splice(sp.keyLine, sp.endLine+1, d.joinNew(block, final)), nil
}

// Rename rewrites the key token of service oldName as newName, quoted only
// when a plain key would not read back as newName (callers check it with
// ValidateName). The rest of the line, inline comment included, stays byte
// for byte.
func (d *Document) Rename(oldName, newName string) ([]byte, error) {
	i, err := d.find(oldName)
	if err != nil {
		return nil, err
	}
	if newName != oldName {
		if _, err := d.find(newName); err == nil {
			return nil, fmt.Errorf("service %q already exists", newName)
		}
	}
	sp, key := d.spans[i], d.keys[i]
	line := d.lines[sp.keyLine]
	start := key.Column - 1
	end := keyEnd(line, start, key.Style)
	if end < 0 {
		return nil, fmt.Errorf("line %d: cannot find the key of service %q", sp.keyLine+1, oldName)
	}
	return d.splice(sp.keyLine, sp.keyLine+1, line[:start]+scalar(newName)+line[end:]), nil
}

// keyEnd returns the offset just past the key token starting at start:
// a quoted key up to its closing quote, a plain key up to the ':' that is
// followed by a space or the line end. It returns -1 when there is none.
func keyEnd(line string, start int, style yaml.Style) int {
	if start < 0 || start >= len(line) {
		return -1
	}
	switch {
	case style&yaml.DoubleQuotedStyle != 0:
		for i := start + 1; i < len(line); i++ {
			switch line[i] {
			case '\\':
				i++
			case '"':
				return i + 1
			}
		}
	case style&yaml.SingleQuotedStyle != 0:
		for i := start + 1; i < len(line); i++ {
			if line[i] != '\'' {
				continue
			}
			if i+1 < len(line) && line[i+1] == '\'' {
				i++ // '' is an escaped quote
				continue
			}
			return i + 1
		}
	default:
		for i := start; i < len(line); i++ {
			if line[i] == ':' && (i+1 == len(line) || strings.ContainsRune(" \t\r\n", rune(line[i+1]))) {
				return i
			}
		}
	}
	return -1
}

// Delete removes service name together with its head comment: the comment
// lines directly above the key, up to a blank line or the services key.
// A comment indented deeper than the key belongs to the service above it
// and stays. removed holds every deleted line without its line ending.
func (d *Document) Delete(name string) (out []byte, removed []string, err error) {
	i, err := d.find(name)
	if err != nil {
		return nil, nil, err
	}
	sp := d.spans[i]
	start := sp.keyLine
	keyCol := indentOf(trimEOL(d.lines[sp.keyLine]))
	for start-1 > d.servicesLine {
		line := trimEOL(d.lines[start-1])
		if !isComment(line) || indentOf(line) > keyCol {
			break
		}
		start--
	}
	for _, l := range d.lines[start : sp.endLine+1] {
		removed = append(removed, trimEOL(l))
	}
	return d.splice(start, sp.endLine+1, ""), removed, nil
}

// DroppedComments returns the comments Replace would drop for service name:
// comment lines inside its span and inline comments on its lines, in file
// order. It returns nil when there are none or the service does not exist.
func (d *Document) DroppedComments(name string) []string {
	i, err := d.find(name)
	if err != nil {
		return nil
	}
	sp := d.spans[i]
	type comment struct {
		line int
		text string
	}
	var found []comment
	for l := sp.keyLine; l <= sp.endLine; l++ {
		if line := trimEOL(d.lines[l]); isComment(line) {
			found = append(found, comment{l, strings.TrimSpace(line)})
		}
	}
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.LineComment != "" {
			found = append(found, comment{n.Line - 1, n.LineComment})
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(d.keys[i])
	walk(d.values[i])

	if len(found) == 0 {
		return nil
	}
	sort.SliceStable(found, func(a, b int) bool { return found[a].line < found[b].line })
	texts := make([]string, len(found))
	for j, c := range found {
		texts[j] = c.text
	}
	return texts
}

// find returns the index of service name.
func (d *Document) find(name string) (int, error) {
	for i, s := range d.spans {
		if s.name == name {
			return i, nil
		}
	}
	return -1, fmt.Errorf("service %q not found", name)
}

// splice returns the source with lines [from, to) replaced by text.
func (d *Document) splice(from, to int, text string) []byte {
	var b strings.Builder
	for _, l := range d.lines[:from] {
		b.WriteString(l)
	}
	b.WriteString(text)
	for _, l := range d.lines[to:] {
		b.WriteString(l)
	}
	return []byte(b.String())
}

// joinNew joins new lines with the document's line ending, ending the last
// one too when final is set.
func (d *Document) joinNew(lines []string, final bool) string {
	s := strings.Join(lines, d.eol)
	if final {
		s += d.eol
	}
	return s
}

// ensureEOL adds eol to a non-empty s that does not end with a newline.
func ensureEOL(s, eol string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + eol
}

func trimEOL(line string) string {
	return strings.TrimRight(line, "\r\n")
}

func isBlank(line string) bool {
	return strings.TrimSpace(line) == ""
}

func isComment(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "#")
}

// indentOf returns the number of leading spaces.
func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}
