package cmd

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/pcpl2lab/sshforward/internal/configedit"
)

// screenDriver feeds keys to a service screen the way bubbletea would,
// running the commands huh returns (moving between groups is one).
type screenDriver struct {
	t   *testing.T
	scr *serviceScreen
	m   huh.Model
}

func driveScreen(t *testing.T, s configedit.Service, isNew bool) *screenDriver {
	t.Helper()
	scr := newServiceScreen(s, func(string) bool { return false }, isNew)
	d := &screenDriver{t: t, scr: scr, m: scr.form}
	d.run(scr.form.Init())
	d.update(tea.WindowSizeMsg{Width: 100, Height: 60})
	return d
}

func (d *screenDriver) update(msg tea.Msg) {
	m, cmd := d.m.Update(msg)
	d.m = m
	d.run(cmd)
}

// run executes cmd and feeds its messages back. Commands that do not answer
// at once (cursor blink ticks) are dropped.
func (d *screenDriver) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(20 * time.Millisecond):
		return
	}
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			d.run(c)
		}
	case tea.QuitMsg:
	default:
		d.update(msg)
	}
}

func (d *screenDriver) key(code rune, mod tea.KeyMod) {
	d.update(tea.KeyPressMsg{Code: code, Mod: mod})
}

func (d *screenDriver) tab()     { d.key(tea.KeyTab, 0) }
func (d *screenDriver) backTab() { d.key(tea.KeyTab, tea.ModShift) }
func (d *screenDriver) enter()   { d.key(tea.KeyEnter, 0) }
func (d *screenDriver) right()   { d.key(tea.KeyRight, 0) }
func (d *screenDriver) backspace(n int) {
	for range n {
		d.key(tea.KeyBackspace, 0)
	}
}

func (d *screenDriver) typeText(s string) {
	for _, r := range s {
		d.update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func (d *screenDriver) view() string {
	return ansiSeq.ReplaceAllString(d.scr.form.View(), "")
}

func (d *screenDriver) done() bool {
	return d.scr.form.State == huh.StateCompleted
}

func twoPortService() configedit.Service {
	return configedit.Service{
		Name:       "gitea",
		RemoteHost: "10.0.0.5",
		Ports: []configedit.Port{
			{Name: "web", RemotePort: 3000, Local: configedit.LocalSame},
			{Name: "ssh", RemotePort: 2222, Local: configedit.LocalCustom, LocalPort: 2022},
		},
	}
}

func TestServiceScreen_ShowsEveryCurrentValue(t *testing.T) {
	d := driveScreen(t, twoPortService(), false)
	v := d.view()
	for _, want := range []string{"10.0.0.5", "Port 1", "web", "3000", "Port 2", "ssh", "2222", "2022", "Next"} {
		if !strings.Contains(v, want) {
			t.Errorf("screen lacks %q:\n%s", want, v)
		}
	}
}

func TestServiceScreen_CustomLocalPortOnlyWhenCustom(t *testing.T) {
	s := configedit.Service{Name: "mysql", Ports: []configedit.Port{{RemotePort: 3306}}}
	d := driveScreen(t, s, false)
	if strings.Contains(d.view(), "Custom local port") {
		t.Fatalf("custom field shown for 'same as remote':\n%s", d.view())
	}
	d.tab()   // host -> remote port
	d.tab()   // remote port -> local mode
	d.right() // random
	d.right() // custom
	if !strings.Contains(d.view(), "Custom local port") {
		t.Fatalf("custom field not shown after choosing custom:\n%s", d.view())
	}
}

// The complaint this screen answers: a value is corrected in place, and
// going back to an earlier field keeps everything typed after it.
func TestServiceScreen_GoBackAndCorrect(t *testing.T) {
	d := driveScreen(t, twoPortService(), false)
	d.tab()         // host -> port 1 name
	d.tab()         // -> port 1 remote port
	d.backspace(1)  // 3000 -> 300
	d.typeText("1") // -> 3001
	d.backTab()     // back to port 1 name
	d.backTab()     // back to host
	d.backspace(1)  // 10.0.0.5 -> 10.0.0.
	d.typeText("9") // -> 10.0.0.9
	for i := 0; i < 20 && !d.done(); i++ {
		d.enter()
	}
	if !d.done() {
		t.Fatalf("form not submitted:\n%s", d.view())
	}
	if d.scr.next != actDone {
		t.Fatalf("next = %d, want Done", d.scr.next)
	}
	got := twoPortService()
	if err := d.scr.collect(&got); err != nil {
		t.Fatal(err)
	}
	want := twoPortService()
	want.RemoteHost = "10.0.0.9"
	want.Ports[0].RemotePort = 3001
	if got.RemoteHost != want.RemoteHost || got.Ports[0] != want.Ports[0] || got.Ports[1] != want.Ports[1] {
		t.Fatalf("collected %+v, want %+v", got, want)
	}
}

func TestServiceScreen_ValidateCatchesRepeatedPortName(t *testing.T) {
	scr := newServiceScreen(twoPortService(), func(string) bool { return false }, false)
	scr.ports[1].name = "web"
	if err := scr.validate(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("validate() = %v, want repeated-name error", err)
	}
}

func TestServiceScreen_NewPortNeedsValues(t *testing.T) {
	s := twoPortService()
	s.Ports = append(s.Ports, configedit.Port{})
	scr := newServiceScreen(s, func(string) bool { return false }, false)
	if err := scr.validate(); err == nil || !strings.Contains(err.Error(), "port 3") {
		t.Fatalf("validate() = %v, want an error for port 3", err)
	}
}

func TestServiceScreen_NewServiceNameChecked(t *testing.T) {
	scr := newServiceScreen(configedit.Service{Ports: []configedit.Port{{RemotePort: 80}}},
		func(n string) bool { return n == "taken" }, true)
	scr.name = "taken"
	if err := scr.validate(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("validate() = %v, want taken-name error", err)
	}
}

func fourPortService() configedit.Service {
	s := twoPortService()
	s.Ports = append(s.Ports,
		configedit.Port{Name: "api", RemotePort: 8080},
		configedit.Port{Name: "metrics", RemotePort: 9090})
	return s
}

func TestServiceScreen_FitsShortTerminal(t *testing.T) {
	const height = 20
	d := driveScreen(t, fourPortService(), false)
	d.scr.layout.height = func() int { return height }
	if n := strings.Count(d.view(), "\n") + 1; n >= height {
		t.Fatalf("view has %d lines, terminal has %d:\n%s", n, height, d.view())
	}
	v := d.view()
	if !strings.Contains(v, "10.0.0.5") || !strings.Contains(v, "more below") || strings.Contains(v, "more above") {
		t.Fatalf("top of the screen not shown as expected:\n%s", v)
	}
}

func TestServiceScreen_ScrollsToFocusedField(t *testing.T) {
	const height = 20
	d := driveScreen(t, fourPortService(), false)
	d.scr.layout.height = func() int { return height }
	// Walk to "Next" at the very bottom.
	for i := 0; i < 30 && !strings.Contains(d.view(), "> Done"); i++ {
		d.tab()
	}
	v := d.view()
	if n := strings.Count(v, "\n") + 1; n >= height {
		t.Fatalf("view has %d lines, terminal has %d:\n%s", n, height, v)
	}
	if !strings.Contains(v, "Next") || !strings.Contains(v, "more above") || strings.Contains(v, "10.0.0.5") {
		t.Fatalf("bottom of the screen not shown as expected:\n%s", v)
	}
	// And back up again.
	for i := 0; i < 30 && !strings.Contains(d.view(), "10.0.0.5"); i++ {
		d.backTab()
	}
	if !strings.Contains(d.view(), "10.0.0.5") {
		t.Fatalf("did not scroll back to the top:\n%s", d.view())
	}
}

func TestServiceScreen_NoScrollWhenItFits(t *testing.T) {
	d := driveScreen(t, twoPortService(), false)
	d.scr.layout.height = func() int { return 200 }
	if v := d.view(); strings.Contains(v, "more above") || strings.Contains(v, "more below") {
		t.Fatalf("markers shown though the screen fits:\n%s", v)
	}
}

func TestMenuForm_QuitsOnQ(t *testing.T) {
	pick := 0
	f := newMenuForm(huh.NewGroup(huh.NewSelect[int]().
		Options(huh.NewOption("gitea", 0), huh.NewOption("Quit", -2)).
		Value(&pick)))
	f.Init()
	f.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if f.State != huh.StateAborted {
		t.Fatalf("state after q = %v, want aborted", f.State)
	}
}

func TestServiceForm_QIsTypedNotQuit(t *testing.T) {
	d := driveScreen(t, twoPortService(), false)
	d.typeText("q")
	if d.scr.form.State == huh.StateAborted || !strings.Contains(d.view(), "10.0.0.5q") {
		t.Fatalf("q did not reach the field:\n%s", d.view())
	}
}

func TestNoteCtrlC(t *testing.T) {
	interrupted = false
	t.Cleanup(func() { interrupted = false })
	noteCtrlC(nil, tea.KeyPressMsg{Code: tea.KeyEscape})
	if interrupted {
		t.Fatal("Esc counted as Ctrl+C")
	}
	noteCtrlC(nil, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !interrupted {
		t.Fatal("Ctrl+C not noted")
	}
}
