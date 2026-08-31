package tunnel

import (
	"strings"
	"testing"
)

func TestBuildSSHArgs(t *testing.T) {
	args := buildSSHArgs("/tmp/x.log", "3306:127.0.0.1:3306", "myhost")

	joined := strings.Join(args, " ")
	mustContain := []string{
		"-N",
		"-E /tmp/x.log",
		"-o BatchMode=yes",
		"-o StrictHostKeyChecking=accept-new",
		"-o ExitOnForwardFailure=yes",
		"-o ConnectTimeout=10",
		"-o ServerAliveInterval=15",
		"-o ServerAliveCountMax=3",
		"-L 3306:127.0.0.1:3306",
	}
	for _, m := range mustContain {
		if !strings.Contains(joined, m) {
			t.Errorf("missing %q in args: %v", m, args)
		}
	}

	// host musi być ostatnim argumentem
	if args[len(args)-1] != "myhost" {
		t.Errorf("host must be last arg, got %q", args[len(args)-1])
	}
	// forwardArg musi wystąpić bezpośrednio po -L
	for i, a := range args {
		if a == "-L" {
			if i+1 >= len(args) || args[i+1] != "3306:127.0.0.1:3306" {
				t.Errorf("-L must be followed by forward arg")
			}
		}
	}
}
