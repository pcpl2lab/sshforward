package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/pcpl2lab/sshforward/internal/config"
	"github.com/pcpl2lab/sshforward/internal/configedit"
	"golang.org/x/term"
)

// The interactive forms below are built only inside RunE, after
// requireTerminal: nothing here may run at package init, so commands that
// never needed a terminal behave exactly as before without one.

// requireTerminal fails fast when the form could not be shown, instead of
// waiting on a pipe or a CI job's empty stdin.
func requireTerminal() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return errors.New("interactive form needs a terminal; edit the file with 'sshforward edit'")
	}
	return nil
}

// loadConfigFile is config.LoadFromPath; tests replace it.
var loadConfigFile = config.LoadFromPath

// openDocument creates the file from the template when createIfMissing, then
// returns its path, raw bytes, the parsed Document and the loaded config.
// A config that does not load returns the loader's error unchanged.
func openDocument(createIfMissing bool) (path string, src []byte, doc *configedit.Document, cfg *config.Config, err error) {
	path, err = config.DefaultConfigPath()
	if err != nil {
		return "", nil, nil, nil, err
	}
	if createIfMissing {
		if err := ensureConfigFile(path); err != nil {
			return "", nil, nil, nil, err
		}
	}
	src, err = os.ReadFile(path)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("cannot read config: %w", err)
	}
	// A file that does not load is never edited: the user fixes it first.
	cfg, err = loadConfigFile(path)
	if err != nil {
		return "", nil, nil, nil, err
	}
	// The loader reads the file itself; reading it again proves cfg was
	// loaded from the same bytes as src.
	again, err := os.ReadFile(path)
	if err != nil {
		return "", nil, nil, nil, fmt.Errorf("cannot read config: %w", err)
	}
	if !bytes.Equal(again, src) {
		return "", nil, nil, nil, configedit.ErrChanged
	}
	doc, err = configedit.Parse(src)
	if err != nil {
		return "", nil, nil, nil, err
	}
	return path, src, doc, cfg, nil
}

// newForm builds every form of add/manage. huh's default keymap quits only on
// Ctrl+C; Esc must cancel too, and both surface as huh.ErrUserAborted.
func newForm(groups ...*huh.Group) *huh.Form {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(key.WithKeys("ctrl+c", "esc"), key.WithHelp("esc", "cancel"))
	return withKeys(km, groups...)
}

// newMenuForm is newForm for manage's service list, which q also quits. The
// list has no filter, so q is never typed into one.
func newMenuForm(groups ...*huh.Group) *huh.Form {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(key.WithKeys("q", "ctrl+c", "esc"), key.WithHelp("q", "quit"))
	km.Select.Filter = key.NewBinding(key.WithDisabled())
	return withKeys(km, groups...)
}

func withKeys(km *huh.KeyMap, groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).
		WithKeyMap(km).
		// Replaces huh's options, so its default output is repeated.
		WithProgramOptions(tea.WithOutput(os.Stderr), tea.WithFilter(noteCtrlC))
}

// interrupted is set when Ctrl+C ends a form. huh reports Esc and Ctrl+C as
// the same huh.ErrUserAborted; manage tells them apart: Esc goes back to its
// service list, Ctrl+C leaves.
var interrupted bool

func noteCtrlC(_ tea.Model, msg tea.Msg) tea.Msg {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "ctrl+c" {
		interrupted = true
	}
	return msg
}

// isAbort reports whether the user cancelled a form (Esc / Ctrl+C).
func isAbort(err error) bool {
	return errors.Is(err, huh.ErrUserAborted)
}

// keepOr returns a validator that runs check on the trimmed value, except that
// a non-empty prefill left unchanged passes: values the loader already
// accepted are only judged again when the user edits them.
func keepOr(prefill string, check func(string) error) func(string) error {
	return func(v string) error {
		v = strings.TrimSpace(v)
		if prefill != "" && v == prefill {
			return nil
		}
		return check(v)
	}
}

func checkPort(v string) error {
	_, err := configedit.ParsePort(v)
	return err
}

// Actions offered at the end of the service screen.
const (
	actDone   = -1
	actAdd    = -2
	actRemove = -3
)

// runServiceForm fills s; returns huh.ErrUserAborted on cancel. The name is
// asked only when isNew and must not be taken. s is changed only on success.
//
// Every field of the service is on one screen, so any value can be corrected
// in place. Adding or removing a port redraws the screen with what was typed
// so far.
func runServiceForm(s *configedit.Service, taken func(name string) bool, isNew bool) error {
	work := *s
	work.Ports = append([]configedit.Port(nil), s.Ports...)
	if len(work.Ports) == 0 {
		work.Ports = append(work.Ports, configedit.Port{})
	}
	for {
		scr := newServiceScreen(work, taken, isNew)
		if err := scr.form.Run(); err != nil {
			return err
		}
		if err := scr.collect(&work); err != nil {
			return err
		}
		switch scr.next {
		case actDone:
			*s = work
			return nil
		case actAdd:
			work.Ports = append(work.Ports, configedit.Port{})
		case actRemove:
			if err := removePort(&work); err != nil {
				return err
			}
		}
	}
}

// portVars are one port's values as typed, next to the values they started
// from: an unchanged prefill is not validated again (see keepOr).
type portVars struct {
	name, remote, local                      string
	mode                                     configedit.LocalMode
	namePrefill, remotePrefill, localPrefill string
}

// serviceScreen is the one-screen form for a service and its ports.
type serviceScreen struct {
	form             *huh.Form
	isNew            bool
	taken            func(string) bool
	name, host       string
	namePre, hostPre string
	ports            []*portVars
	layout           *stackLayout
	next             int
}

func newServiceScreen(s configedit.Service, taken func(string) bool, isNew bool) *serviceScreen {
	scr := &serviceScreen{
		isNew: isNew, taken: taken,
		name: s.Name, namePre: s.Name,
		host: s.RemoteHost, hostPre: s.RemoteHost,
		next: actDone,
	}
	multi := len(s.Ports) > 1
	layout := &stackLayout{height: terminalHeight}
	scr.layout = layout

	var head []huh.Field
	if isNew {
		head = append(head, huh.NewInput().
			Title("Service name").
			Value(&scr.name).
			Validate(scr.checkName))
	}
	head = append(head, huh.NewInput().
		Title("Remote host").
		Description("Optional; empty means 127.0.0.1 on the SSH server").
		Value(&scr.host).
		Validate(keepOr(scr.hostPre, configedit.ValidateHost)))
	layout.add("", nil, head...)

	for i, p := range s.Ports {
		v := &portVars{name: p.Name, namePrefill: p.Name, mode: p.Local}
		if p.RemotePort > 0 {
			v.remotePrefill = strconv.Itoa(p.RemotePort)
		}
		if p.Local == configedit.LocalCustom {
			v.localPrefill = strconv.Itoa(p.LocalPort)
		}
		v.remote, v.local = v.remotePrefill, v.localPrefill
		scr.ports = append(scr.ports, v)

		title := "Port"
		var fields []huh.Field
		if multi {
			title = fmt.Sprintf("Port %d", i+1)
			fields = append(fields, huh.NewInput().
				Title("Port name").
				Description("For example web or ssh").
				Value(&v.name).
				Validate(scr.portNameCheck(i)))
		}
		fields = append(fields,
			huh.NewInput().
				Title("Remote port").
				Value(&v.remote).
				Validate(keepOr(v.remotePrefill, checkPort)),
			huh.NewSelect[configedit.LocalMode]().
				Title("Local port").
				Options(
					huh.NewOption("same as remote", configedit.LocalSame),
					huh.NewOption("random free port", configedit.LocalAuto),
					huh.NewOption("custom", configedit.LocalCustom),
				).
				Inline(true).
				Value(&v.mode),
		)
		layout.add(title, nil, fields...)
		layout.add("", func() bool { return v.mode != configedit.LocalCustom },
			huh.NewInput().
				Title("Custom local port").
				Value(&v.local).
				Validate(keepOr(v.localPrefill, checkPort)))
	}

	opts := []huh.Option[int]{huh.NewOption("Done", actDone), huh.NewOption("Add port", actAdd)}
	if multi {
		opts = append(opts, huh.NewOption("Remove port", actRemove))
	}
	layout.add("", nil, huh.NewSelect[int]().
		Title("Next").
		Options(opts...).
		Value(&scr.next).
		// Each field is checked when it is left; this also catches what a
		// later edit made wrong, e.g. a port name that now repeats.
		Validate(func(int) error { return scr.validate() }))

	scr.form = newForm(layout.groups...).WithLayout(layout)
	return scr
}

func (scr *serviceScreen) checkName(v string) error {
	return keepOr(scr.namePre, func(v string) error {
		if err := configedit.ValidateName(v); err != nil {
			return err
		}
		if scr.taken(v) {
			return fmt.Errorf("service %q already exists", v)
		}
		return nil
	})(v)
}

// portNameCheck validates port i's name against the names currently typed
// for the other ports.
func (scr *serviceScreen) portNameCheck(i int) func(string) error {
	return func(v string) error {
		var others []string
		for j, o := range scr.ports {
			if j != i {
				others = append(others, strings.TrimSpace(o.name))
			}
		}
		return keepOr(scr.ports[i].namePrefill, portNameCheck(others))(v)
	}
}

// validate runs every field's check, so the screen cannot be left with a
// value that was never checked or no longer fits.
func (scr *serviceScreen) validate() error {
	if scr.isNew {
		if err := scr.checkName(scr.name); err != nil {
			return fmt.Errorf("service name: %w", err)
		}
	}
	if err := keepOr(scr.hostPre, configedit.ValidateHost)(scr.host); err != nil {
		return fmt.Errorf("remote host: %w", err)
	}
	multi := len(scr.ports) > 1
	for i, v := range scr.ports {
		label := "port"
		if multi {
			label = fmt.Sprintf("port %d", i+1)
			if err := scr.portNameCheck(i)(v.name); err != nil {
				return fmt.Errorf("%s name: %w", label, err)
			}
		}
		if err := keepOr(v.remotePrefill, checkPort)(v.remote); err != nil {
			return fmt.Errorf("%s remote port: %w", label, err)
		}
		if v.mode == configedit.LocalCustom {
			if err := keepOr(v.localPrefill, checkPort)(v.local); err != nil {
				return fmt.Errorf("%s custom local port: %w", label, err)
			}
		}
	}
	return nil
}

// collect writes the screen's values into s, whose ports match the screen's.
func (scr *serviceScreen) collect(s *configedit.Service) error {
	if scr.isNew {
		s.Name = strings.TrimSpace(scr.name)
	}
	s.RemoteHost = strings.TrimSpace(scr.host)
	multi := len(scr.ports) > 1
	for i, v := range scr.ports {
		p := &s.Ports[i]
		if multi {
			p.Name = strings.TrimSpace(v.name)
		}
		var err error
		if r := strings.TrimSpace(v.remote); r != v.remotePrefill {
			if p.RemotePort, err = configedit.ParsePort(r); err != nil {
				return err
			}
		}
		p.Local = v.mode
		if v.mode == configedit.LocalCustom {
			if l := strings.TrimSpace(v.local); l != v.localPrefill {
				if p.LocalPort, err = configedit.ParsePort(l); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// stackLayout shows every visible group at once, so all values stay in view
// and Shift+Tab reaches any earlier field. huh's LayoutStack draws hidden
// groups too, hence this one. A screen taller than the terminal scrolls to
// keep the focused group in view.
type stackLayout struct {
	groups []*huh.Group
	hidden []func() bool
	fields [][]huh.Field
	// height reports the terminal's height, 0 when unknown.
	height func() int
	// top is the first line shown while the screen is scrolled.
	top int
}

// add appends a group; hide, when set, keeps it off screen and out of the
// tab order while it returns true.
func (l *stackLayout) add(title string, hide func() bool, fields ...huh.Field) {
	g := huh.NewGroup(fields...).Title(title)
	if hide != nil {
		g = g.WithHideFunc(hide)
	}
	l.groups = append(l.groups, g)
	l.hidden = append(l.hidden, hide)
	l.fields = append(l.fields, fields)
}

func (l *stackLayout) View(f *huh.Form) string {
	focused := f.GetFocusedField()
	var lines []string
	start, end := 0, 0
	footer := ""
	for i, g := range l.groups {
		if l.hidden[i] != nil && l.hidden[i]() {
			continue
		}
		first := len(lines)
		if h := g.Header(); h != "" {
			lines = append(lines, strings.Split(h, "\n")...)
		}
		lines = append(lines, strings.Split(g.Content(), "\n")...)
		if slices.Contains(l.fields[i], focused) {
			start, end = first, len(lines)
			footer = g.Footer()
		}
		lines = append(lines, "")
	}
	var foot []string
	if footer != "" {
		foot = strings.Split(footer, "\n")
	}
	return strings.Join(append(l.scroll(lines, start, end, len(foot)), foot...), "\n")
}

// scroll returns the part of lines that fits the terminal above a footer of
// footer lines, keeping lines [start, end) - the focused group - in view.
// Marker lines say when there is more above or below.
func (l *stackLayout) scroll(lines []string, start, end, footer int) []string {
	h := 0
	if l.height != nil {
		h = l.height()
	}
	// One row stays free: a view as tall as the terminal pushes its first
	// line off screen.
	avail := h - footer - 1
	if h <= 0 || len(lines) <= avail {
		l.top = 0
		return lines
	}
	room := max(avail-2, 1) // two rows go to the markers
	if end-start > room {
		end = start + room
	}
	if start < l.top {
		l.top = start
	}
	if end > l.top+room {
		l.top = end - room
	}
	l.top = max(min(l.top, len(lines)-room), 0)

	out := make([]string, 0, room+2)
	out = append(out, moreMarker(l.top > 0, "↑ more above"))
	out = append(out, lines[l.top:l.top+room]...)
	return append(out, moreMarker(l.top+room < len(lines), "↓ more below"))
}

func moreMarker(show bool, text string) string {
	if !show {
		return ""
	}
	return "  " + text
}

// terminalHeight is the stdout terminal's height, or 0 when there is none.
func terminalHeight() int {
	_, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return h
}

func (l *stackLayout) GroupWidth(_ *huh.Form, _ *huh.Group, w int) int {
	return w
}

// removePort asks which port to drop; it is offered only with two or more.
func removePort(s *configedit.Service) error {
	opts := make([]huh.Option[int], 0, len(s.Ports))
	for i, p := range s.Ports {
		opts = append(opts, huh.NewOption(portLabel(p, true), i))
	}
	var idx int
	err := newForm(huh.NewGroup(huh.NewSelect[int]().
		Title("Remove which port?").
		Options(opts...).
		Value(&idx))).Run()
	if err != nil {
		return err
	}
	s.Ports = append(s.Ports[:idx], s.Ports[idx+1:]...)
	return nil
}

// portNameCheck validates a port name and keeps it unique within the service.
func portNameCheck(others []string) func(string) error {
	return func(v string) error {
		if err := configedit.ValidateName(v); err != nil {
			return err
		}
		for _, o := range others {
			if o == v {
				return fmt.Errorf("port %q already exists in this service", v)
			}
		}
		return nil
	}
}

// portLabel describes a port in a select, e.g. "web: remote 3000, local same".
func portLabel(p configedit.Port, withName bool) string {
	var local string
	switch p.Local {
	case configedit.LocalAuto:
		local = "random"
	case configedit.LocalCustom:
		local = strconv.Itoa(p.LocalPort)
	default:
		local = "same"
	}
	label := fmt.Sprintf("remote %d, local %s", p.RemotePort, local)
	if withName && p.Name != "" {
		label = p.Name + ": " + label
	}
	return label
}

// printPreview prints the lines a change writes or removes, then any
// warnings. It is plain output rather than part of a form, so a long preview
// scrolls with the terminal instead of being cut off.
func printPreview(lines []string, warnings []string) {
	fmt.Println()
	for _, l := range lines {
		fmt.Println("  " + l)
	}
	if len(warnings) > 0 {
		fmt.Println()
		for _, w := range warnings {
			fmt.Println(w)
		}
	}
	fmt.Println()
}

// confirmPreview prints the preview and asks whether to save. The default
// answer is yes, or no when there are warnings.
func confirmPreview(title string, lines []string, warnings []string) (bool, error) {
	printPreview(lines, warnings)
	ok := len(warnings) == 0
	err := newForm(huh.NewGroup(huh.NewConfirm().
		Title(title).
		Affirmative("Save").
		Negative("Cancel").
		Value(&ok))).Run()
	return ok, err
}
