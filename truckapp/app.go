package truckapp

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/carlmjohnson/flagext"
	"github.com/djherbis/times"
	"github.com/peterbourgon/ff"
	"github.com/rwcarlsen/goexif/exif"
)

const AppName = "truck"

func CLI(args []string) error {
	a, err := parseArgs(args)
	if err != nil {
		return err
	}
	if err := a.exec(); err != nil {
		fmt.Fprintf(os.Stderr, "Runtime error: %v\n", err)
		return err
	}
	return nil
}

func parseArgs(args []string) (*app, error) {
	fl := flag.NewFlagSet(AppName, flag.ContinueOnError)
	// TODO: Copy files
	// todo: Overwrite mode
	// todo: relative to original directory?
	dryrun := fl.Bool("dryrun", false, `just output changes to stdout`)
	silent := fl.Bool("silent", false, `don't log what changed to stdout`)
	useNull := fl.Bool("0", false, `use null character as filename separator`)
	l := log.New(nil, AppName+" ", log.LstdFlags)
	fl.Var(
		flagext.Logger(l, flagext.LogVerbose),
		"verbose",
		`log debug output`,
	)

	fl.Usage = func() {
		fmt.Fprintf(fl.Output(), `Truck moves files from point A to point B.

Truck expects to receive a list of files to move from standard input, typically by piping "ls" or "find".

	truck [options] <mv-pattern>

<mv-pattern> is a Go text template describing how to transform the source file into the destination name.

Example:

$ find . -name '*.go' -print0 | truck -0 '{{.Dir}}/{{.BaseName}}_bak.go'
mv "./truckapp/app.go" "/src/truck/truckapp/app_bak.go"
mv "./main.go" "/src/truck/main_bak.go"

$ find . -name '*.go'|truck '{{.Dir}}/{{printf "%02d" .N}}-{{.Times.ModTime.Format "2006_01_02"}}-{{.BaseName}}_bak{{.Ext}}'
mv "./truckapp/app.go" "/src/truck/truckapp/01-2019_11_24-app_bak.go"
mv "./main.go" "/src/truck/02-2019_11_18-main_bak.go"

Options:
`)
		fl.PrintDefaults()
	}
	if err := ff.Parse(fl, args, ff.WithEnvVarPrefix("TRUCK")); err != nil {
		return nil, err
	}

	if fl.NArg() != 1 {
		fmt.Fprintf(fl.Output(), "wrong number of args: %d\n", fl.NArg())
		fl.Usage()
		return nil, flag.ErrHelp
	}

	t, err := template.New("mv-pattern").Parse(fl.Arg(0))
	if err != nil {
		return nil, err
	}

	sep := "\n"
	if *useNull {
		sep = "\x00"
	}
	a := app{
		t:      t,
		sep:    sep,
		dryrun: *dryrun,
		silent: *silent,
		Logger: l,
	}
	return &a, nil
}

type app struct {
	t              *template.Template
	sep            string
	dryrun, silent bool
	*log.Logger
}

func (a *app) exec() error {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(os.Stdin); err != nil {
		return err
	}
	paths := strings.Split(buf.String(), a.sep)
	// filter empty paths
	{
		n := 0
		for _, path := range paths {
			if path != "" {
				paths[n] = path
				n++
			}
		}
		paths = paths[:n]
	}
	a.Printf("got %d path(s)", len(paths))

	n := 0
	for _, oldPath := range paths {
		newPath, err := a.buildPath(n, oldPath)
		if err != nil {
			return err
		}
		if newPath == "" {
			a.Printf(`skipping %q == ""`, oldPath)
			continue
		}
		if !a.silent {
			a.Println("mv command equiv:")
			fmt.Printf("mv %q %q\n", oldPath, newPath)
		}
		n++
		if a.dryrun {
			continue
		}
		if err = a.move(newPath, oldPath); err != nil {
			return err
		}
	}
	var would string
	if a.dryrun {
		would = "would have "
	}
	a.Printf("%smoved %d file(s)", would, n)
	return nil
}

func (a *app) buildPath(i int, old string) (string, error) {
	a.Printf("building path for %q", old)

	var buf strings.Builder
	data, err := dataFor(i, old)
	if err != nil {
		return "", err
	}
	err = a.t.Execute(&buf, data)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func dataFor(i int, raw string) (interface{}, error) {
	abs, err := filepath.Abs(raw)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(abs)
	base := filepath.Base(abs)
	ext := filepath.Ext(base)
	basename := base[:len(base)-len(ext)]

	data := fileData{
		i, raw, abs, dir, base, ext, basename,
	}
	return data, nil
}

type fileData struct {
	i                                  int
	Raw, Abs, Dir, Base, Ext, BaseName string
}

func (fd fileData) I() int {
	return fd.i
}

func (fd fileData) N() int {
	return fd.i + 1
}

func (fd fileData) Times() (times.Timespec, error) {
	return times.Stat(fd.Raw)
}

func (fd fileData) Stat() (os.FileInfo, error) {
	return os.Stat(fd.Raw)
}

func (fd fileData) Exif() (*exif.Exif, error) {
	f, err := os.Open(fd.Raw)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	e, _ := exif.Decode(f)
	return e, nil
}

func (a *app) move(newPath, oldPath string) error {
	oldPath, err := filepath.Abs(oldPath)
	if err != nil {
		return err
	}
	newPath, err = filepath.Abs(newPath)
	if err != nil {
		return err
	}
	if newPath == oldPath {
		a.Printf("skipping %q == %q", oldPath, newPath)
		return nil
	}

	dir := filepath.Dir(newPath)
	if err = os.MkdirAll(dir, os.ModePerm); err != nil {
		a.Printf("could not make containing path %q", dir)
		// probably going to fail but go on anyway
	}

	// todo: overwrite mode
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		a.Printf("cannot rename %q → %q\n", oldPath, newPath)
		return nil
	}

	a.Printf("moving %q → %q", oldPath, newPath)
	return os.Rename(oldPath, newPath)
}
