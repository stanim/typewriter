/*
The gofloat command translates any go source code from ints to floats.
It is written with svgo in mind, but might work for other projects as
well.

Usage

Install it first:

  $ cd gofloat
  $ go install

Apply it it to a configuration file. For example to convert svgo from
ints to floats:

  $ gofloat svgo.json
  Open "svgo.json" ...
  github.com/ajstarks/svgo -> github.com/stanim/svgotest:
  - Empty "github.com/stanim/svgotest" ...
  - Convert type from "int" to "float64" ...
  - Fix type conflicts ...
	... no type conflicts found.
  - Format "github.com/stanim/svgotest" ...
  - Applied 13 patches to "github.com/stanim/svgotest" ...
  - Copy non-go files of "github.com/stanim/svgotest" ...
  - OK
  ...
  github.com/ajstarks/svgo/imfade -> github.com/stanim/svgotest/imfade:
  - Empty "github.com/stanim/svgotest/imfade" ...
  - Convert type from "int" to "float64" ...
  - Fix type conflicts ...
	+ /home/stani/Labo/go/src/github.com/stanim/svgotest/imfade/imfade.go:25:14: cannot compare i < width - 128 (mismatched types int and float64)
	+ /home/stani/Labo/go/src/github.com/stanim/svgotest/imfade/imfade.go:26:16: cannot pass argument i (variable of type int) to parameter of type float64
	... fixed 2 type conflicts.
  - Format "github.com/stanim/svgotest/imfade" ...
  - Copy non-go files of "github.com/stanim/svgotest/imfade" ...
  - OK
  ...
  github.com/ajstarks/svgo/planets -> github.com/stanim/svgotest/planets:
  - Empty "github.com/stanim/svgotest/planets" ...
  - Convert type from "int" to "float64" ...
  - Fix type conflicts ...
	... no type conflicts found.
  - Format "github.com/stanim/svgotest/planets" ...
	Please fix: /home/stani/Labo/go/src/github.com/stanim/svgotest/planets/planets.go:122:65: can't check non-constant format "tfmt" in call to Sprintf.
  - SKIP
  ...

Output

If a package is succesfully converted it will finish with an 'OK'.
Process

It uses 4 phases:

1) Convert types: all int types are converted to floats.

2) Fix all type conflicts:
for example make sure that all slice indices are integers.

3) Fix format arguments in printf functions. ("%d" becomes "%f")

4) Apply patches and add header/footer if necessary.

See the package 'packages' for more information.

Limitations

1) In phase 3 (format verbs), non-constant arguments can not be checked
in call to printf functions. These packages will be skipped and should
be manually fixed first. Let's take svgo planets as an example:

  github.com/ajstarks/svgo/planets -> github.com/stanim/svgotest/planets:
  - Empty "github.com/stanim/svgotest/planets" ...
  - Convert type from "int" to "float64" ...
  - Fix type conflicts ...
	... no type conflicts found.
  - Format "github.com/stanim/svgotest/planets" ...
	Please fix: /home/stani/Labo/go/src/github.com/stanim/svgotest/planets/planets.go:122:65: can't check non-constant format "tfmt" in call to Sprintf.
  - SKIP

The check uses the same internal code of 'go vet', which can be used
to find all lines which need to be fixed.

  go tool vet -v planets.go

This gives the following output:

  Checking file planets.go
  planets.go:122: can't check non-constant format in call to Sprintf

The following code blocks the conversion and needs to be fixed:

  tfmt := "fill:white; font-size:%dpx; font-family:Calibri,sans; text-anchor:middle"
  ...
  canvas.Text(px+po, y-labeloc-10, "You are here", fmt.Sprintf(tfmt, fontsize))

A fix is to declare 'tfmt' not as a variable, but as a constant.
This also appears more correct anyhow:

  const tfmt = "fill:white; font-size:%dpx; font-family:Calibri,sans; text-anchor:middle"
  ...
  canvas.Text(px+po, y-labeloc-10, "You are here", fmt.Sprintf(tfmt, fontsize))

Often these files are easy to convert by hand. However if you want
automatic conversion with the gofloat command by hand, it is important
that all formats in printf functions are constants.

If the format does not contain '%d' (int), which needs to be converted
to '%f' (float), it is safe to ignore this issue. By adding the
filename to the FormatVar list in the json configuration file, gofloat
will not perform this check.

2) Converting to float32 is not supported.
*/
package main

//TODO: check do the visitors need all their fields
import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/stanim/typewriter/packages"
)

const maxInt = int(^uint(0) >> 1)

var (
	verbose   = flag.Bool("v", false, "verbose")
	stdout    = log.New(os.Stdout, "", 0)
	null      = log.New(ioutil.Discard, "", 0)
	logg      = stdout
	goSrcPath = packages.GoPathSrc("")
)

func fatal(err error) {
	logg.Fatal(err)
}

func dir(fromDir string, cfg Config, repo Repository,
	imports map[string]string) error {

	toRepo := filepath.Join(cfg.To, repo.Name)
	toDir := strings.Replace(fromDir, cfg.From, toRepo, 1)
	fromRepo := strings.Replace(fromDir, goSrcPath, "", 1)[1:]
	toRepo = strings.Replace(toDir, goSrcPath, "", 1)[1:]
	logg.Printf("%s -> %s:\n", fromRepo, toRepo)
	// phase 0: make repo empty
	logg.Printf("- Empty %q ...\n", toRepo)
	os.MkdirAll(toDir, 0777)
	empty(toDir)
	// phase 1: convert types
	logg.Printf("- Convert type from %q to %q ...\n",
		cfg.FromType, repo.ToType)
	pkgs, err := packages.New(fromDir)
	if err != nil {
		return err
	}
	if err := pkgs.Error(); err != nil { // no type error allowed
		return err
	}
	pkgs.Convert(cfg.FromType, repo.ToType, toDir, cfg.Skip, imports)
	if err := pkgs.Save(toDir); err != nil {
		return err
	}
	// phase 2: fix type conflicts
	logg.Printf("- Fix type conflicts ...\n")
	prevN := maxInt
	n := prevN - 1
	count := 0
	var pkgsSnippets map[string]packages.Set
	for n != 0 && n < prevN {
		prevN = n
		pkgs, err = packages.New(toDir)
		if err != nil {
			return err
		}
		pkgs.SetSnippets(pkgsSnippets)
		n, err = pkgs.Fix(cfg.FromType, repo.ToType, cfg.LogConflicts)
		if err != nil {
			logg.Printf("- Error during fixing type conflicts")
			return err
		}
		pkgsSnippets = pkgs.Snippets()
		count += n
	}
	if count == 0 {
		logg.Printf("  ... no type conflicts found.\n")
	} else {
		logg.Printf("  ... fixed %d type conflicts.\n", count)
	}
	// phase 3: fix format verbs
	logg.Printf("- Format %q ...\n", toRepo)
	pkgs, err = packages.New(toDir)
	if err != nil {
		return err
	}
	if err := pkgs.Error(); err != nil { // no type error allowed
		return err
	}
	if err := pkgs.Format(cfg.FromType, cfg.FormatVar, cfg.FormatFunc,
		cfg.Printf); err != nil {
		logg.Printf("  Please fix: %s\n- SKIP\n\n", err)
		_ = os.RemoveAll(toDir) // discard error
		// allow these errors to be fixed
		return nil
	}
	if err := pkgs.Save(toDir); err != nil {
		return err
	}
	// phase 4: header, patches and footer (utils.go)
	count, err = patch(toDir, cfg.Header, cfg.Patches, cfg.Footer)
	if err != nil {
		return err
	}
	if count > 1 {
		logg.Printf("- Applied %d patches to %q ...\n", count, toRepo)
	} else if count == 1 {
		logg.Printf("- Applied one patch to %q ...\n", toRepo)
	}
	// phase 5: copy non-go files (utils.go)
	logg.Printf("- Copy non-go files of %q ...\n", toRepo)
	if err := copyFiles(fromDir, toDir, cfg.ReadMe); err != nil {
		return err
	}
	logg.Printf("- OK\n\n")
	return nil
}

// recurse converts all files in a dir and all subdirs
func recurse(fromDir string, config Config, repo Repository,
	imports map[string]string) error {

	walk := func(sub string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			return nil
		}
		if info.Name()[0] == '.' {
			return filepath.SkipDir
		}
		return dir(sub, config, repo, imports)
	}
	return filepath.Walk(fromDir, walk)
}

func run() error {
	var err error
	cfgJson := "svgo.json"
	n := len(os.Args)
	if n > 1 && os.Args[n-1][0] != '-' {
		cfgJson = os.Args[n-1]
	}
	logg.Printf("Open %q ...", cfgJson)
	repos, cfg, err := Open(cfgJson)
	if err != nil {
		return context(err)
	}
	for _, repo := range repos {
		if repo.Disabled {
			break
		}
		fromDir := packages.GoPathSrc(cfg.From)
		toRepo := fmt.Sprintf("%s/%s", cfg.To, repo.Name)
		imports := map[string]string{cfg.From: toRepo}
		if repo.Recurse {
			if err := recurse(fromDir, cfg, repo, imports); err != nil {
				return err
			}
		} else {
			if err := dir(fromDir, cfg, repo, imports); err != nil {
				return err
			}
		}
	}
	return nil
}

func main() {
	flag.Parse()
	logg = stdout
	if *verbose {
		logg = stdout
	}
	if err := run(); err != nil {
		stdout.Fatalf("Error: %s\n", err)
	}
	logg.Println("Done without errors.")
}
