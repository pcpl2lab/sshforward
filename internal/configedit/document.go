package configedit

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrFlowStyle is returned for a services section written in flow style
// ({...}), which cannot be edited line by line.
var ErrFlowStyle = errors.New("services are written in flow style; edit the file with 'sshforward edit'")

// Document is a config file located for text-level edits. yaml.Node is used
// only to find line numbers; edits work on lines so untouched bytes stay.
type Document struct {
	lines        []string      // source split with strings.SplitAfter(src, "\n"), terminators kept
	eol          string        // "\r\n" if the source contains one, else "\n"
	servicesLine int           // 0-based line of the services key; -1 when there is none
	servicesCol  int           // 0-based column of the services key
	nullCol      int           // 0-based column of an explicit null value (~, null) of services; -1 if none
	nullLen      int           // length of that null token
	spans        []serviceSpan // services in file order
	keys         []*yaml.Node  // key node of each service, parallel to spans
	values       []*yaml.Node  // value node of each service, parallel to spans
	base         string        // indent of a service key line
	step         string        // one nesting level
}

// serviceSpan is the line range of one service, key line to last value line.
type serviceSpan struct {
	name    string // key as parsed (unquoted)
	keyLine int    // 0-based line of the service key
	endLine int    // 0-based last line of the value, inclusive
}

// Parse locates the services section and each service in src.
func Parse(src []byte) (*Document, error) {
	text := string(src)
	d := &Document{
		lines:        strings.SplitAfter(text, "\n"),
		eol:          "\n",
		servicesLine: -1,
		nullCol:      -1,
		step:         "  ",
	}
	if strings.Contains(text, "\r\n") {
		d.eol = "\r\n"
	}

	var root yaml.Node
	if err := yaml.Unmarshal(src, &root); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	services, err := findServices(d, &root)
	if err != nil {
		return nil, err
	}

	if services != nil && services.Kind == yaml.MappingNode {
		if services.Style&yaml.FlowStyle != 0 {
			return nil, ErrFlowStyle
		}
		for i := 0; i+1 < len(services.Content); i += 2 {
			key, val := services.Content[i], services.Content[i+1]
			if val.Kind == yaml.MappingNode && val.Style&yaml.FlowStyle != 0 {
				return nil, ErrFlowStyle
			}
			end := max(key.Line, lastLine(val)) - 1
			d.spans = append(d.spans, serviceSpan{
				name:    key.Value,
				keyLine: key.Line - 1,
				endLine: d.extendEnd(end, key.Column-1),
			})
			d.keys = append(d.keys, key)
			d.values = append(d.values, val)
		}
		first, firstVal := services.Content[0], services.Content[1]
		d.base = strings.Repeat(" ", first.Column-1)
		if firstVal.Kind == yaml.MappingNode && len(firstVal.Content) > 0 {
			if n := firstVal.Content[0].Column - first.Column; n > 0 {
				d.step = strings.Repeat(" ", n)
			}
		}
		return d, nil
	}

	d.base = strings.Repeat(" ", d.servicesCol) + d.step
	return d, nil
}

// findServices records the services key position in d and returns its value
// node, or nil when the file has no services key. A null value is valid (the
// edit template has only comments under services:).
func findServices(d *Document, root *yaml.Node) (*yaml.Node, error) {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil, nil // empty file
	}
	doc := root.Content[0]
	if isNull(doc) {
		return nil, nil // comments only
	}
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("line %d: config must be a mapping", doc.Line)
	}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		key, val := doc.Content[i], doc.Content[i+1]
		if key.Value != "services" {
			continue
		}
		d.servicesLine = key.Line - 1
		d.servicesCol = key.Column - 1
		if isNull(val) {
			if val.Value != "" && val.Line == key.Line {
				d.nullCol, d.nullLen = val.Column-1, len(val.Value)
			}
			return nil, nil
		}
		if val.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("line %d: 'services' must be a mapping", val.Line)
		}
		return val, nil
	}
	return nil, nil
}

// lastLine returns the highest 1-based line of any node in the subtree.
// An implicit null (nothing written after the key) does not count, since
// the parser may place it on the following line.
func lastLine(n *yaml.Node) int {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" && n.Value == "" {
		return 0
	}
	last := n.Line
	for _, c := range n.Content {
		last = max(last, lastLine(c))
	}
	return last
}

// extendEnd moves a span end past the rest of a value that continues over
// several lines (a block or multi-line scalar, a flow list split over lines):
// nodes only give the line a value starts on. Following lines indented deeper
// than the service key belong to it. Blank and comment lines are taken only
// when such a line follows them, so trailing comments stay outside the span.
func (d *Document) extendEnd(end, keyCol int) int {
	for i := end + 1; i < len(d.lines); i++ {
		line := trimEOL(d.lines[i])
		if isBlank(line) || isComment(line) {
			continue
		}
		if indentOf(line) <= keyCol {
			break
		}
		end = i
	}
	return end
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == "!!null"
}

// Names returns the service names in file order.
func (d *Document) Names() []string {
	names := make([]string, len(d.spans))
	for i, s := range d.spans {
		names[i] = s.name
	}
	return names
}

// Service returns the raw values of a service as written, for prefilling
// the edit form: an omitted local_port is LocalSame, auto/0 LocalAuto, any
// other number LocalCustom (mirrors config.parseLocalPort).
func (d *Document) Service(name string) (Service, error) {
	for i, s := range d.spans {
		if s.name == name {
			return serviceFromNode(name, d.values[i])
		}
	}
	return Service{}, fmt.Errorf("service %q not found", name)
}

func serviceFromNode(name string, n *yaml.Node) (Service, error) {
	svc := Service{Name: name}
	if isNull(n) {
		return Service{Name: name, Ports: []Port{{}}}, nil
	}
	if n.Kind != yaml.MappingNode {
		return svc, fmt.Errorf("service %q: line %d: expected a mapping", name, n.Line)
	}

	var short Port
	var portsNode *yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i].Value, n.Content[i+1]
		var err error
		switch key {
		case "remote_host":
			svc.RemoteHost = textValue(val)
		case "ports":
			portsNode = val
		default:
			err = setPortField(&short, key, val)
		}
		if err != nil {
			return svc, fmt.Errorf("service %q: %w", name, err)
		}
	}

	if portsNode == nil || isNull(portsNode) {
		svc.Ports = []Port{short}
		return svc, nil
	}
	if portsNode.Kind != yaml.SequenceNode {
		return svc, fmt.Errorf("service %q: line %d: 'ports' must be a list", name, portsNode.Line)
	}
	for _, item := range portsNode.Content {
		if item.Kind != yaml.MappingNode {
			return svc, fmt.Errorf("service %q: line %d: port entry must be a mapping", name, item.Line)
		}
		var p Port
		for i := 0; i+1 < len(item.Content); i += 2 {
			key, val := item.Content[i].Value, item.Content[i+1]
			var err error
			switch key {
			case "name":
				p.Name = textValue(val)
			case "remote_host":
				p.RemoteHost = textValue(val)
			default:
				err = setPortField(&p, key, val)
			}
			if err != nil {
				return svc, fmt.Errorf("service %q: %w", name, err)
			}
		}
		svc.Ports = append(svc.Ports, p)
	}
	return svc, nil
}

// textValue returns a string field as the loader reads it: null (~, null)
// is empty, not the literal token.
func textValue(val *yaml.Node) string {
	if isNull(val) {
		return ""
	}
	return val.Value
}

// setPortField reads remote_port and local_port into p; other keys are
// ignored (unknown keys are the loader's business).
func setPortField(p *Port, key string, val *yaml.Node) error {
	switch key {
	case "remote_port":
		if isNull(val) {
			return nil
		}
		if err := val.Decode(&p.RemotePort); err != nil {
			return fmt.Errorf("line %d: remote_port must be a number", val.Line)
		}
	case "local_port":
		return setLocal(p, val)
	}
	return nil
}

// setLocal mirrors config.parseLocalPort: absent, null or "" means same as
// remote, "auto" or 0 a random port, any other number a custom port.
func setLocal(p *Port, val *yaml.Node) error {
	if isNull(val) {
		return nil
	}
	if val.Kind == yaml.ScalarNode && val.Tag == "!!str" {
		switch val.Value {
		case "":
			return nil
		case "auto":
			p.Local, p.AutoRaw = LocalAuto, val.Value
			return nil
		}
		return fmt.Errorf("line %d: local_port must be a number or 'auto', got %q", val.Line, val.Value)
	}
	var n int
	if err := val.Decode(&n); err != nil {
		return fmt.Errorf("line %d: local_port must be a number or 'auto'", val.Line)
	}
	if n == 0 {
		p.Local, p.AutoRaw = LocalAuto, val.Value
		return nil
	}
	p.Local, p.LocalPort = LocalCustom, n
	return nil
}
