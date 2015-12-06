// gofloat converts code which uses ints to floats.
// It is written for svgo, but might work for other projects as well.

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
		n, err = pkgs.Fix(cfg.FromType, repo.ToType)
		if err != nil {
			logg.Printf("- Error during fixing type conflicts")
			return err
		}
		pkgsSnippets = pkgs.Snippets()
		count += n
	}
	if count == 0 {
		logg.Printf("- No type conflicts\n")
	} else {
		logg.Printf("- Fixed %d type conflicts ", count)
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
	if err := pkgs.Format(cfg.FromType, cfg.FormatFunc,
		cfg.Printf); err != nil {
		return err
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
	logg.Printf("- Copy non-go files of %q ...\n\n", toRepo)
	if err := copyFiles(fromDir, toDir, cfg.ReadMe); err != nil {
		return err
	}
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
