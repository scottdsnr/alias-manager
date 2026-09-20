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
	KindFunc
)

// Node is one line of the document.
type Node struct {
	Kind Kind
	Raw  string // verbatim line, for KindRaw

	Group string // for KindGroup: its name. For KindAlias: owning group.

	Name    string // alias or function name
	Command string // alias body, unquoted
	Enabled bool
	Comment string // trailing comment on the alias/function header line, if any
	Indent  string
	quote   byte // original quote char, so we round-trip faithfully

	// Body holds the lines between the braces of a KindFunc, verbatim and
	// without the enclosing "name() {" / "}" lines.
	Body string
	// keyword records that the function was written as "function name()",
	// so we round-trip the author's style.
	keyword bool
	// noParens records "function name {" — legal only with the keyword.
	noParens bool
}

// Doc is a parsed alias file.
type Doc struct {
	Path  string
	Nodes []*Node
}

var (
	aliasRe = regexp.MustCompile(`^(\s*)alias\s+(-[a-zA-Z]+\s+)?([^\s=]+)=(.*)$`)
	groupRe = regexp.MustCompile(`^#\s*=====\s*(.*?)\s*=====\s*$`)
	funcRe  = regexp.MustCompile(`^(\s*)(function\s+)?([A-Za-z_][A-Za-z0-9_.\-]*)\s*(\(\s*\))?\s*\{\s*(#.*)?$`)
	endRe   = regexp.MustCompile(`^\s*\}\s*;?\s*$`)
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

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	group := Ungrouped
	for i := 0; i < len(lines); i++ {
		line := lines[i]
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
		if n, next := parseFunc(lines, i); n != nil {
			n.Group = group
			d.Nodes = append(d.Nodes, n)
			i = next
			continue
		}
		d.Nodes = append(d.Nodes, &Node{Kind: KindRaw, Raw: line})
	}
	return d, nil
}

// uncomment strips the disabled marker from a line, reporting whether it was
// there. Managed-but-disabled content keeps "#!" on every one of its lines.
func uncomment(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, disabledPrefix) {
		return line, false
	}
	idx := strings.Index(line, disabledPrefix)
	return line[:idx] + line[idx+len(disabledPrefix):], true
}

// parseFunc tries to read a shell function starting at lines[i]. It returns
// the node and the index of its closing brace, or nil if this is not one.
func parseFunc(lines []string, i int) (*Node, int) {
	head, disabled := uncomment(lines[i])
	m := funcRe.FindStringSubmatch(head)
	if m == nil {
		return nil, i
	}
	keyword, parens := m[2] != "", m[4] != ""
	// "name {" without either marker is a brace block, not a function.
	if !keyword && !parens {
		return nil, i
	}
	var body []string
	for j := i + 1; j < len(lines); j++ {
		raw := lines[j]
		if disabled {
			var off bool
			raw, off = uncomment(raw)
			// A disabled function keeps "#!" to its closing brace; a bare
			// line means we ran past the end of the managed block.
			if !off && strings.TrimSpace(raw) != "" {
				return nil, i
			}
		}
		if endRe.MatchString(raw) {
			comment := ""
			if m[5] != "" {
				comment = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m[5]), "#"))
			}
			return &Node{
				Kind:     KindFunc,
				Raw:      lines[i],
				Name:     m[3],
				Body:     strings.Join(body, "\n"),
				Enabled:  !disabled,
				Comment:  comment,
				Indent:   m[1],
				keyword:  keyword,
				noParens: keyword && !parens,
			}, j
		}
		body = append(body, raw)
	}
	return nil, i
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
		case KindFunc:
			b.WriteString(n.Lines())
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

// Lines renders a function as its full multi-line definition.
func (n *Node) Lines() string {
	head := n.Name + "()"
	if n.keyword {
		head = "function " + n.Name + "()"
		if n.noParens {
			head = "function " + n.Name
		}
	}
	head += " {"
	if n.Comment != "" {
		head += " # " + n.Comment
	}
	out := []string{n.Indent + head}
	// Body lines carry their own indentation, exactly as typed.
	out = append(out, strings.Split(n.Body, "\n")...)
	out = append(out, n.Indent+"}")
	if !n.Enabled {
		for i, l := range out {
			// Keep blank lines blank so the file stays readable.
			if strings.TrimSpace(l) == "" {
				continue
			}
			out[i] = disabledPrefix + l
		}
	}
	return strings.Join(out, "\n")
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
		if (n.Kind == KindAlias || n.Kind == KindFunc) && n.Group == Ungrouped {
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

// Functions returns the functions in a group, in file order.
func (d *Doc) Functions(group string) []*Node {
	var out []*Node
	for _, n := range d.Nodes {
		if n.Kind == KindFunc && n.Group == group {
			out = append(out, n)
		}
	}
	return out
}

// FindFunc returns the node for a function name, or nil.
func (d *Doc) FindFunc(name string) *Node {
	for _, n := range d.Nodes {
		if n.Kind == KindFunc && n.Name == name {
			return n
		}
	}
	return nil
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
		case (n.Kind == KindAlias || n.Kind == KindFunc) && n.Group == group:
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

// DeleteFunc removes a function.
func (d *Doc) DeleteFunc(name string) { d.removeKind(KindFunc, name); d.reflow() }

// UpsertFunc creates or updates a shell function, placing it at the end of
// its group. Body is the text between the braces, verbatim.
func (d *Doc) UpsertFunc(oldName string, n Node) error {
	if err := ValidateFuncName(n.Name); err != nil {
		return err
	}
	if strings.TrimSpace(n.Body) == "" {
		return fmt.Errorf("function body cannot be empty")
	}
	if n.Group == "" {
		n.Group = Ungrouped
	}
	if ex := d.FindFunc(n.Name); ex != nil && n.Name != oldName {
		return fmt.Errorf("function %q already exists", n.Name)
	}
	if oldName != "" {
		if ex := d.FindFunc(oldName); ex != nil {
			group := ex.Group
			ex.Name, ex.Body, ex.Enabled, ex.Comment = n.Name, n.Body, n.Enabled, n.Comment
			if group == n.Group {
				return nil
			}
			d.removeKind(KindFunc, oldName)
		}
	}
	node := &Node{Kind: KindFunc, Name: n.Name, Body: n.Body, Enabled: n.Enabled, Comment: n.Comment, Group: n.Group}
	d.insert(node)
	return nil
}

func (d *Doc) remove(name string) { d.removeKind(KindAlias, name) }

func (d *Doc) removeKind(kind Kind, name string) {
	for i, n := range d.Nodes {
		if n.Kind == kind && n.Name == name {
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
		case inGroup && (n.Kind == KindAlias || n.Kind == KindFunc):
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

var funcNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)

// ValidateFuncName rejects names the shell would not accept for a function.
func ValidateFuncName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("function name cannot be empty")
	}
	if !funcNameRe.MatchString(name) {
		return fmt.Errorf("invalid function name %q", name)
	}
	return nil
}

// MoveToGroup moves the named aliases into group, keeping their relative file
// order. Names that do not resolve to an alias are ignored.
func (d *Doc) MoveToGroup(names []string, group string) error {
	if strings.TrimSpace(group) == "" {
		group = Ungrouped
	}
	if group != Ungrouped {
		known := false
		for _, g := range d.Groups() {
			if g == group {
				known = true
			}
		}
		if !known {
			if err := d.AddGroup(group); err != nil {
				return err
			}
		}
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	// Collect in file order so the moved block keeps its original sequence.
	var moving []Node
	for _, n := range d.Nodes {
		if n.Kind == KindAlias && want[n.Name] && n.Group != group {
			moving = append(moving, *n)
		}
	}
	for _, n := range moving {
		d.remove(n.Name)
	}
	for _, n := range moving {
		node := n
		node.Group = group
		node.Raw = ""
		d.insert(&node)
	}
	d.reflow()
	return nil
}
