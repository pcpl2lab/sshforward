package cmd

import (
	"fmt"

	"github.com/pcpl2lab/sshforward/internal/config"
	"github.com/pcpl2lab/sshforward/internal/configedit"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a service to the config with an interactive form",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireTerminal(); err != nil {
			return err
		}
		path, src, doc, cfg, err := openDocument(true)
		if err != nil {
			return err
		}
		return runAdd(path, src, doc, cfg)
	},
}

// runAdd asks for a new service, previews it and appends it to the file at
// path, which held src when doc and cfg were read from it. Cancelling at any
// point writes nothing and is not an error.
func runAdd(path string, src []byte, doc *configedit.Document, cfg *config.Config) error {
	taken := func(name string) bool {
		_, ok := cfg.Services[name]
		return ok
	}

	var s configedit.Service
	if err := runServiceForm(&s, taken, true); err != nil {
		return nothingChangedOn(err)
	}
	ok, err := confirmPreview(fmt.Sprintf("Add service %q?", s.Name), doc.Rendered(s), nil)
	if err != nil {
		return nothingChangedOn(err)
	}
	if !ok {
		return nothingChangedOn(nil)
	}

	out := doc.Add(s)
	if err := configedit.Commit(path, src, out, configedit.Expected(cfg, "", &s)); err != nil {
		return err
	}
	fmt.Printf("Added service %q. Start it with: sshforward start <host> %s\n", s.Name, s.Name)
	return nil
}

// nothingChangedOn turns a cancel (or a declined preview, err == nil) into the
// "Nothing changed." message and a clean exit; other errors pass through.
func nothingChangedOn(err error) error {
	if err != nil && !isAbort(err) {
		return err
	}
	fmt.Println("Nothing changed.")
	return nil
}

func init() {
	rootCmd.AddCommand(addCmd)
}
