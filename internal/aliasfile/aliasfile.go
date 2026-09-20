// Package aliasfile parses and rewrites a shell alias file as an ordered
// document, so that unrelated content in .bashrc/.zshrc survives round-trips.
package aliasfile

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// groupPrefix marks a group header comment written by alias-manager.
	groupPrefix = "# ===== "
	groupSuffix = " ====="
	// disabledPrefix marks an alias that is commented out but still managed.
	disabledPrefix = "#!"
	// Ungrouped is the bucket for aliases that sit outside any group header.
	Ungrouped = "Ungrouped"
)

type Kind int

const (
	KindRaw Kind = iota
	KindGroup
	KindAlias
)

// Node is one line of the document.
type Node struct {
	Kind Kind
	Raw  string // verbatim line, for KindRaw

	Group string // for KindGroup: its name. For KindAlias: owning group.

	Name    string // alias name
	Command string // alias body, unquoted
	Enabled bool
	Comment string // trailing comment on the alias line, if any
	Indent  string
	quote   byte // original quote char, so we round-trip faithfully
}

// Doc is a parsed alias file.
type Doc struct {
	Path  string
	Nodes []*Node
}

var (
	aliasRe = regexp.MustCompile(`^(\s*)alias\s+(-[a-zA-Z]+\s+)?([^\s=]+)=(.*)$`)
	groupRe = regexp.MustCompile(`^#\s*=====\s*(.*?)\s*=====\s*$`)
)

// Load reads path into a Doc. A missing file parses as an empty document.
func Load(path string) (*Doc, error) {
	d := &Doc{Path: path}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	group := Ungrouped
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if m := groupRe.FindStringSubmatch(line); m != nil {
			group = m[1]
			d.Nodes = append(d.Nodes, &Node{Kind: KindGroup, Group: group, Raw: line})
			continue
		}
		if n := parseAlias(line); n != nil {
			n.Group = group
			d.Nodes = append(d.Nodes, n)
			continue
		}
		d.Nodes = append(d.Nodes, &Node{Kind: KindRaw, Raw: line})
	}
	return d, sc.Err()
}

func parseAlias(line string) *Node {
	body, enabled := line, true
	if strings.HasPrefix(strings.TrimSpace(line), disabledPrefix) {
		enabled = false
		idx := strings.Index(line, disabledPrefix)
		body = line[:idx] + line[idx+len(disabledPrefix):]
	}
	m := aliasRe.FindStringSubmatch(body)
	if m == nil {
		return nil
	}
	cmd, comment, quote := splitValue(m[4])
	return &Node{
		Kind:    KindAlias,
		Raw:     line,
		Name:    m[3],
		Command: cmd,
		Enabled: enabled,
		Comment: comment,
		Indent:  m[1],
		quote:   quote,
	}
}

// splitValue pulls the alias body out of the right-hand side of an alias line,
// separating a trailing comment that sits outside the quotes.
func splitValue(rhs string) (cmd, comment string, quote byte) {
	rhs = strings.TrimSpace(rhs)
	if rhs == "" {
		return "", "", '\''
	}
	if q := rhs[0]; q == '\'' || q == '"' {
		if end := strings.LastIndexByte(rhs, q); end > 0 {
			cmd = rhs[1:end]
			rest := strings.TrimSpace(rhs[end+1:])
			if strings.HasPrefix(rest, "#") {
				comment = strings.TrimSpace(strings.TrimPrefix(rest, "#"))
			}
			return cmd, comment, q
		}
		return rhs[1:], "", q
	}
	if i := strings.Index(rhs, " #"); i >= 0 {
		return strings.TrimSpace(rhs[:i]), strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rhs[i:]), "#")), '\''
	}
	return rhs, "", '\''
}

// Render turns the document back into file text.
func (d *Doc) Render() string {
	var b strings.Builder
	for _, n := range d.Nodes {
		switch n.Kind {
		case KindGroup:
			b.WriteString(groupPrefix + n.Group + groupSuffix)
		case KindAlias:
			b.WriteString(n.Line())
		default:
			b.WriteString(n.Raw)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Line renders a single alias line.
func (n *Node) Line() string {
	q := n.quote
	if q == 0 {
		q = '\''
	}
	if q == '\'' && strings.Contains(n.Command, "'") {
		q = '"'
	}
	var b strings.Builder
	b.WriteString(n.Indent)
	if !n.Enabled {
		b.WriteString(disabledPrefix)
	}
	fmt.Fprintf(&b, "alias %s=%c%s%c", n.Name, q, n.Command, q)
	if n.Comment != "" {
		b.WriteString(" # " + n.Comment)
	}
	return b.String()
}

// Save writes the document atomically, keeping a .bak of the previous content.
func (d *Doc) Save() error {
	if err := os.MkdirAll(filepath.Dir(d.Path), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(d.Path); err == nil {
		mode = fi.Mode().Perm()
		if prev, err := os.ReadFile(d.Path); err == nil {
			_ = os.WriteFile(d.Path+".bak", prev, mode)
		}
	}
	tmp := fmt.Sprintf("%s.tmp-%d", d.Path, time.Now().UnixNano())
	if err := os.WriteFile(tmp, []byte(d.Render()), mode); err != nil {
		return err
	}
	return os.Rename(tmp, d.Path)
}

// Groups returns group names in file order, always including Ungrouped first
// when it holds anything.
func (d *Doc) Groups() []string {
	var out []string
	seen := map[string]bool{}
	add := func(g string) {
		if !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	for _, n := range d.Nodes {
		if n.Kind == KindAlias && n.Group == Ungrouped {
			add(Ungrouped)
		}
	}
	for _, n := range d.Nodes {
		if n.Kind == KindGroup {
			add(n.Group)
		}
	}
	return out
}

// Aliases returns the aliases in a group, in file order.
func (d *Doc) Aliases(group string) []*Node {
	var out []*Node
	for _, n := range d.Nodes {
		if n.Kind == KindAlias && n.Group == group {
			out = append(out, n)
		}
	}
	return out
}

// Find returns the node for an alias name, or nil.
func (d *Doc) Find(name string) *Node {
	for _, n := range d.Nodes {
		if n.Kind == KindAlias && n.Name == name {
			return n
		}
	}
	return nil
}

// AddGroup appends a group header if it does not already exist.
func (d *Doc) AddGroup(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("group name cannot be empty")
	}
	if strings.Contains(name, "=====") {
		return fmt.Errorf("group name cannot contain %q", "=====")
	}
	for _, g := range d.Groups() {
		if strings.EqualFold(g, name) {
			return fmt.Errorf("group %q already exists", name)
		}
	}
	if len(d.Nodes) > 0 && strings.TrimSpace(d.Nodes[len(d.Nodes)-1].Raw) != "" {
		d.Nodes = append(d.Nodes, &Node{Kind: KindRaw, Raw: ""})
	}
	d.Nodes = append(d.Nodes, &Node{Kind: KindGroup, Group: name, Raw: groupPrefix + name + groupSuffix})
	return nil
}

// RenameGroup renames a group header and its members.
func (d *Doc) RenameGroup(old, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || old == Ungrouped {
		return fmt.Errorf("cannot rename this group")
	}
	for _, n := range d.Nodes {
		if n.Group == old {
			n.Group = name
		}
	}
	return nil
}

// DeleteGroup removes the header; its aliases fall back to Ungrouped unless
// withAliases is set, in which case they are deleted too.
func (d *Doc) DeleteGroup(group string, withAliases bool) {
	var keep []*Node
	for _, n := range d.Nodes {
		switch {
		case n.Kind == KindGroup && n.Group == group:
			continue
		case n.Kind == KindAlias && n.Group == group:
			if withAliases {
				continue
			}
			n.Group = Ungrouped
			keep = append(keep, n)
		default:
			keep = append(keep, n)
		}
	}
	d.Nodes = keep
	d.reflow()
}

// Upsert creates or updates an alias, placing it at the end of its group.
func (d *Doc) Upsert(oldName string, n Node) error {
	if err := ValidateName(n.Name); err != nil {
		return err
	}
	if strings.TrimSpace(n.Command) == "" {
		return fmt.Errorf("command cannot be empty")
	}
	if n.Group == "" {
		n.Group = Ungrouped
	}
	if ex := d.Find(n.Name); ex != nil && n.Name != oldName {
		return fmt.Errorf("alias %q already exists", n.Name)
	}
	if oldName != "" {
		if ex := d.Find(oldName); ex != nil {
			group := ex.Group
			ex.Name, ex.Command, ex.Enabled, ex.Comment = n.Name, n.Command, n.Enabled, n.Comment
			if group == n.Group {
				return nil
			}
			d.remove(oldName)
		}
	}
	node := &Node{Kind: KindAlias, Name: n.Name, Command: n.Command, Enabled: n.Enabled, Comment: n.Comment, Group: n.Group}
	d.insert(node)
	return nil
}

// Delete removes an alias.
func (d *Doc) Delete(name string) { d.remove(name); d.reflow() }

func (d *Doc) remove(name string) {
	for i, n := range d.Nodes {
		if n.Kind == KindAlias && n.Name == name {
			d.Nodes = append(d.Nodes[:i], d.Nodes[i+1:]...)
			return
		}
	}
}

// insert places node after the last line belonging to its group.
func (d *Doc) insert(node *Node) {
	if node.Group == Ungrouped {
		// Before the first group header, so it stays out of every group.
		for i, n := range d.Nodes {
			if n.Kind == KindGroup {
				d.Nodes = append(d.Nodes[:i], append([]*Node{node}, d.Nodes[i:]...)...)
				return
			}
		}
		d.Nodes = append(d.Nodes, node)
		return
	}
	last := -1
	inGroup := false
	for i, n := range d.Nodes {
		switch {
		case n.Kind == KindGroup && n.Group == node.Group:
			inGroup, last = true, i
		case n.Kind == KindGroup:
			inGroup = false
		case inGroup && n.Kind == KindAlias:
			last = i
		}
	}
	if last < 0 {
		_ = d.AddGroup(node.Group)
		d.Nodes = append(d.Nodes, node)
		return
	}
	d.Nodes = append(d.Nodes[:last+1], append([]*Node{node}, d.Nodes[last+1:]...)...)
}

// reflow collapses runs of blank lines left behind by deletions.
func (d *Doc) reflow() {
	var keep []*Node
	blank := 0
	for _, n := range d.Nodes {
		if n.Kind == KindRaw && strings.TrimSpace(n.Raw) == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		keep = append(keep, n)
	}
	d.Nodes = keep
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_.\-+:@!?\[\]]+$`)

// ValidateName rejects alias names the shell would not accept.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("alias name cannot be empty")
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid alias name %q", name)
	}
	return nil
}
