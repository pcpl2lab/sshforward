package cmd

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/huh/v2"
	"github.com/pcpl2lab/sshforward/internal/config"
	"github.com/pcpl2lab/sshforward/internal/configedit"
	"github.com/pcpl2lab/sshforward/internal/tunnel"
	"github.com/spf13/cobra"
)

var manageCmd = &cobra.Command{
	Use:   "manage",
	Short: "Add, edit, rename or delete services with an interactive form",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireTerminal(); err != nil {
			return err
		}
		// Every change, saved or cancelled, comes back to the service list
		// until the user quits there (or presses Ctrl+C anywhere). The file is
		// read again each time, so the list and the next change start from
		// what was just written.
		last := ""
		for first := true; ; first = false {
			interrupted = false
			path, src, doc, cfg, err := openDocument(true)
			if err != nil {
				return err
			}
			m := &manager{path: path, src: src, doc: doc, cfg: cfg}
			quit := false
			if first && len(doc.Names()) == 0 {
				err = runAdd(path, src, doc, cfg)
			} else {
				quit, err = m.menu(&last)
			}
			if err != nil || quit || interrupted {
				return err
			}
			fmt.Println()
		}
	},
}

// menu lists the services and runs the change picked for one of them. last
// is the service picked the time before: the cursor starts there, and it is
// updated to this pick. quit is set when the user leaves the list.
func (m *manager) menu(last *string) (quit bool, err error) {
	names := m.doc.Names()
	// Options carry indexes, not names: the loader accepts any service
	// name, so no name can double as a marker.
	const (
		newService = -1
		quitMenu   = -2
	)
	opts := make([]huh.Option[int], 0, len(names)+2)
	pick := 0
	for i, name := range names {
		opts = append(opts, huh.NewOption(name, i))
		if name == *last {
			pick = i
		}
	}
	if len(names) == 0 {
		pick = newService
	}
	opts = append(opts,
		huh.NewOption("+ New service", newService),
		huh.NewOption("Quit", quitMenu))
	err = newMenuForm(huh.NewGroup(huh.NewSelect[int]().
		Title("Service").
		Options(opts...).
		Value(&pick))).Run()
	if isAbort(err) || pick == quitMenu {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if pick == newService {
		return false, runAdd(m.path, m.src, m.doc, m.cfg)
	}
	*last = names[pick]
	return false, m.run(names[pick])
}

// manager holds what the file looked like when manage read it.
type manager struct {
	path string
	src  []byte
	doc  *configedit.Document
	cfg  *config.Config
}

func (m *manager) run(name string) error {
	const (
		actEdit   = "edit"
		actRename = "rename"
		actDelete = "delete"
	)
	action := actEdit
	err := newForm(huh.NewGroup(huh.NewSelect[string]().
		Title(fmt.Sprintf("Service %q", name)).
		Options(
			huh.NewOption("Edit", actEdit),
			huh.NewOption("Rename", actRename),
			huh.NewOption("Delete", actDelete),
		).
		Value(&action))).Run()
	if err != nil {
		return nothingChangedOn(err)
	}
	switch action {
	case actRename:
		return m.rename(name)
	case actDelete:
		return m.delete(name)
	default:
		return m.edit(name)
	}
}

// edit runs the service form prefilled with name and replaces its block.
func (m *manager) edit(name string) error {
	orig, err := m.doc.Service(name)
	if err != nil {
		return err
	}
	s := orig
	s.Ports = append([]configedit.Port(nil), orig.Ports...)
	if err := runServiceForm(&s, m.takenExcept(name), false); err != nil {
		return nothingChangedOn(err)
	}
	if sameRendering(orig, s) {
		// Rewriting an unchanged block would only drop its comments.
		return nothingChangedOn(nil)
	}

	var warnings []string
	if dropped := m.doc.DroppedComments(name); len(dropped) > 0 {
		warnings = append(warnings, "These comments in the service will be dropped:")
		for _, c := range dropped {
			warnings = append(warnings, "  "+c)
		}
	}
	if w := collapseWarning(orig, s); w != "" {
		warnings = append(warnings, w)
	}
	warnings = append(warnings, liveWarnings(name)...)

	ok, err := confirmPreview(fmt.Sprintf("Save service %q?", name), m.doc.Rendered(s), warnings)
	if err != nil || !ok {
		return nothingChangedOn(err)
	}
	out, err := m.doc.Replace(name, s)
	if err != nil {
		return err
	}
	if err := configedit.Commit(m.path, m.src, out, configedit.Expected(m.cfg, name, &s)); err != nil {
		return err
	}
	fmt.Printf("Updated service %q.\n", name)
	return nil
}

// rename asks for a new name and rewrites only the service's key.
func (m *manager) rename(oldName string) error {
	newName := oldName
	taken := m.takenExcept(oldName)
	err := newForm(huh.NewGroup(huh.NewInput().
		Title(fmt.Sprintf("New name for %q", oldName)).
		Value(&newName).
		Validate(func(v string) error {
			v = strings.TrimSpace(v)
			if err := configedit.ValidateName(v); err != nil {
				return err
			}
			if taken(v) {
				return fmt.Errorf("service %q already exists", v)
			}
			return nil
		}))).Run()
	if err != nil {
		return nothingChangedOn(err)
	}
	newName = strings.TrimSpace(newName)
	if newName == oldName {
		return nothingChangedOn(nil)
	}

	out, err := m.doc.Rename(oldName, newName)
	if err != nil {
		return err
	}
	before, after := changedLine(m.src, out)
	ok, err := confirmPreview(
		fmt.Sprintf("Rename service %q to %q?", oldName, newName),
		[]string{"- " + before, "+ " + after},
		liveWarnings(oldName))
	if err != nil || !ok {
		return nothingChangedOn(err)
	}
	if err := configedit.Commit(m.path, m.src, out, configedit.ExpectedRename(m.cfg, oldName, newName)); err != nil {
		return err
	}
	fmt.Printf("Renamed service %q to %q.\n", oldName, newName)
	return nil
}

// delete removes the service and its head comment after a confirmation that
// defaults to Cancel.
func (m *manager) delete(name string) error {
	out, removed, err := m.doc.Delete(name)
	if err != nil {
		return err
	}
	fmt.Printf("These lines of service %q will be removed:\n", name)
	printPreview(removed, liveWarnings(name))
	ok := false
	err = newForm(huh.NewGroup(huh.NewConfirm().
		Title(fmt.Sprintf("Delete service %q?", name)).
		Affirmative("Delete").
		Negative("Cancel").
		Value(&ok))).Run()
	if err != nil || !ok {
		return nothingChangedOn(err)
	}
	if err := configedit.Commit(m.path, m.src, out, configedit.Expected(m.cfg, name, nil)); err != nil {
		return err
	}
	fmt.Printf("Deleted service %q.\n", name)
	return nil
}

// takenExcept reports whether a service other than name already uses a name.
func (m *manager) takenExcept(name string) func(string) bool {
	return func(v string) bool {
		if v == name {
			return false
		}
		_, ok := m.cfg.Services[v]
		return ok
	}
}

// sameRendering reports whether s would be written exactly as orig. Values
// that are not written (the name of a lone port) do not count.
func sameRendering(orig, s configedit.Service) bool {
	return slices.Equal(configedit.Render(orig, "", "  "), configedit.Render(s, "", "  "))
}

// collapseWarning warns when a multi-port service collapses to one port whose
// own remote_host replaces a different service-level one, or returns "".
func collapseWarning(orig, s configedit.Service) string {
	if len(orig.Ports) > 1 && len(s.Ports) == 1 {
		// Render writes the port's own host in the short form (see the spec).
		if p := s.Ports[0]; p.RemoteHost != "" && s.RemoteHost != "" && p.RemoteHost != s.RemoteHost {
			return fmt.Sprintf(
				"remote_host %s from port %s is kept; the service-level remote_host is not used",
				p.RemoteHost, p.Name)
		}
	}
	return ""
}

// changedLine returns the first line that differs between a and b, without
// its line ending. Rename changes exactly one line.
func changedLine(a, b []byte) (before, after string) {
	al := strings.Split(string(a), "\n")
	bl := strings.Split(string(b), "\n")
	for i := 0; i < len(al) && i < len(bl); i++ {
		if al[i] != bl[i] {
			return strings.TrimRight(al[i], "\r"), strings.TrimRight(bl[i], "\r")
		}
	}
	return "", ""
}

// liveWarnings lists the live tunnels of service. Tunnel state that cannot be
// read counts as no live tunnels: it must not block editing the config.
func liveWarnings(service string) []string {
	dir, err := tunnel.TunnelsDir()
	if err != nil {
		return nil
	}
	states, err := tunnel.ListStates(dir)
	if err != nil {
		return nil
	}
	return liveTunnelWarnings(states, service, (*tunnel.TunnelState).IsAlive)
}

// liveTunnelWarnings returns one line per live tunnel of service, e.g.
// "tunnel prod/gitea (web) is running with the old settings; stop it with: sshforward stop prod gitea".
// alive is (*tunnel.TunnelState).IsAlive in production; a stub in tests,
// because IsAlive also requires the process image to be ssh.
func liveTunnelWarnings(states []*tunnel.TunnelState, service string, alive func(*tunnel.TunnelState) bool) []string {
	var lines []string
	for _, s := range states {
		if s.Service != service || !alive(s) {
			continue
		}
		label := s.Host + "/" + s.Service
		if s.PortName != "" {
			label += " (" + s.PortName + ")"
		}
		lines = append(lines, fmt.Sprintf(
			"tunnel %s is running with the old settings; stop it with: sshforward stop %s %s",
			label, s.Host, s.Service))
	}
	return lines
}

func init() {
	rootCmd.AddCommand(manageCmd)
}
