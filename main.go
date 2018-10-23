package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/rogpeppe/go-internal/semver"
	"gopkg.in/errgo.v2/fmt/errors"
)

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `gomodmerge gomodfile

Update the local module's dependencies by merging them
from the dependencies implied by the argument go.mod file.

Dependencies that are older than the current module's dependencies
will be ignored.
`)
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
	}
	if err := mergeMod(flag.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "%v (%s)\n", err, errors.Details(err))
		os.Exit(1)
	}
}

func mergeMod(modfile string) error {
	localVersions, err := moduleVersions("")
	if err != nil {
		return errors.Wrap(err)
	}
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return errors.Wrap(err)
	}
	defer os.RemoveAll(dir)
	goMod, err := ioutil.ReadFile(modfile)
	if err != nil {
		return errors.Wrap(err)
	}
	if err := ioutil.WriteFile(filepath.Join(dir, "go.mod"), goMod, 0666); err != nil {
		return errors.Wrap(err)
	}
	otherVersions, err := moduleVersions(dir)
	if err != nil {
		return errors.Wrap(err)
	}
	updates := make(map[string]string)
	for mod, version := range otherVersions {
		localVersion, ok := localVersions[mod]
		if !ok || semver.Compare(version, localVersion) > 0 {
			updates[mod] = version
		}
	}
	if len(updates) == 0 {
		fmt.Println("no updates required")
		return nil
	}
	updateMods := make([]string, 0, len(updates))
	for mod := range updates {
		updateMods = append(updateMods, mod)
	}
	sort.Strings(updateMods)
	editArgs := []string{"mod", "edit"}
	for _, mod := range updateMods {
		editArgs = append(editArgs, "-require="+mod+"@"+updates[mod])
	}
	c := exec.Command("go", editArgs...)
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return errors.Wrap(err)
	}
	for _, mod := range updateMods {
		fmt.Printf("%s %s\n", mod, updates[mod])
	}
	return nil
}

type goMod struct {
	Path    string
	Version string
}

func moduleVersions(dir string) (map[string]string, error) {
	var out bytes.Buffer
	c := exec.Command("go", "list", "-m", "-json", "all")
	c.Dir = dir
	c.Stderr = os.Stderr
	c.Stdout = &out
	if err := c.Run(); err != nil {
		return nil, errors.Wrap(err)
	}
	dec := json.NewDecoder(&out)
	mods := make(map[string]string)
	for {
		var m goMod
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			return nil, errors.Wrap(err)
		}
		if m.Version != "" {
			mods[m.Path] = m.Version
		}
	}
	return mods, nil
}
