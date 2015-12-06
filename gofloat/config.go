package main

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/stanim/typewriter/packages"
)

// Repository refers to a destination repository, which contains the
// automatically translated code..
type Repository struct {
	Name     string // repo name:  svgof, svgo2f
	ToType   string // "float64"
	Disabled bool
	Recurse  bool // also convert subfolders?
}

// Patch uses basically strings.Replace to apply patches to a file.
type Patch struct {
	Old   string
	New   string
	N     int
	Start int
}

// Config can be applied to multiple destination repositories.
type Config struct {
	Footer     map[string][]byte
	FormatFunc map[string]string
	From       string
	FromType   string
	Header     []byte
	Patches    map[string][]Patch // patches by filename
	Printf     map[string]int
	ReadMe     []byte
	Skip       packages.Skip
	To         string
}

type configData struct {
	Footer     map[string][]string
	FormatFunc map[string]string
	From       string
	FromType   string
	Header     []string
	Patches    map[string][]Patch
	Printf     map[string]int
	ReadMe     []string
	Skip       map[string][]string
	To         string
	ToType     string
}

type data struct {
	Repos  []Repository
	Config configData
}

// Open opens a JSON config file and returns destination repositories
// with a common config file.
func Open(path string) ([]Repository, Config, error) {
	cfg := Config{}
	f, err := os.Open(path)
	if err != nil {
		return nil, cfg, context(err)
	}
	var d data
	if err = json.NewDecoder(f).Decode(&d); err != nil {
		return nil, cfg, context(err)
	}
	cfgd := d.Config
	if cfgd.From == "" {
		return nil, cfg, contextErr("config 'From' repo is unknown")
	}
	if cfgd.To == "" {
		return nil, cfg, contextErr("config 'To' repo is unknown")
	}
	for i := range d.Repos {
		if d.Repos[i].ToType == "" {
			d.Repos[i].ToType = "float64"
		}
	}
	cfg.FormatFunc = cfgd.FormatFunc
	cfg.From = cfgd.From
	if cfgd.FromType == "" {
		cfg.FromType = "int"
	} else {
		cfg.FromType = cfgd.FromType
	}
	cfg.Footer = map[string][]byte{}
	for fn, lines := range cfgd.Footer {
		cfg.Footer[fn] = []byte(
			"\n// Automatically appended by gofloat\n\n" +
				strings.Join(lines, "\n") + "\n")
	}
	cfg.Header = []byte(strings.Join(cfgd.Header, "\n"))
	cfg.Patches = cfgd.Patches
	if len(cfgd.Printf) == 0 {
		cfg.Printf = map[string]int{
			"errorf":  0,
			"fatalf":  0,
			"fprintf": 1,
			"logf":    0,
			"panicf":  0,
			"printf":  0,
			"sprintf": 0,
		}
	} else {
		cfg.Printf = cfgd.Printf
	}
	cfg.ReadMe = []byte(strings.Join(cfgd.ReadMe, "\n"))
	star := cfgd.Skip["*"]
	for base, lst := range cfgd.Skip {
		cfgd.Skip[base] = append(lst, star...)
	}
	cfg.Skip = packages.NewSkip(cfgd.Skip)
	cfg.To = cfgd.To
	return d.Repos, cfg, nil
}
