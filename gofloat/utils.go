package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// empty folder before converting (remove anything except hidden '.')
func empty(dirname string) error {
	_, err := os.Stat(dirname)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return context(err)
	}
	files, err := ioutil.ReadDir(dirname)
	if err != nil {
		return context(err)
	}
	for _, f := range files {
		name := f.Name()
		if name[0] == '.' { // skip .git etc ...
			continue
		}
		name = filepath.Join(dirname, name)
		if f.IsDir() {
			if err := os.RemoveAll(name); err != nil {
				return context(err)
			}
			continue
		}
		if err := os.Remove(name); err != nil {
			return context(err)
		}
	}
	return nil
}

// patch prepends the header, applies patches to the source and
// appends footer.
func patch(dirname string, header []byte, patches map[string][]Patch,
	footer map[string][]byte) (int, error) {
	if patches == nil {
		return 0, nil
	}
	infos, err := ioutil.ReadDir(dirname)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, info := range infos {
		if info.IsDir() {
			continue
		}
		base := info.Name()
		if strings.ToLower(filepath.Ext(base)) != ".go" {
			continue
		}
		filename := filepath.Join(dirname, base)
		buf, err := ioutil.ReadFile(filename)
		if err != nil {
			return 0, context(err)
		}
		source := string(buf)
		fmt.Println(base)
		ps, ok := patches[base]
		if ok {
			for _, patch := range ps {
				n := -1
				if patch.N > 0 {
					n = patch.N
				}
				source = source[:patch.Start] + strings.Replace(
					source[patch.Start:], patch.Old, patch.New, n)
			}
			count += len(ps)
		}
		fo, err := os.Create(filename)
		if err != nil {
			return 0, context(err)
		}
		defer fo.Close()
		_, err = fo.Write(header)
		if err != nil {
			return 0, context(err)
		}
		_, err = fo.WriteString(source)
		if err != nil {
			return 0, context(err)
		}
		ft, ok := footer[base]
		if ok {
			_, err = fo.Write(ft)
			if err != nil {
				return 0, context(err)
			}
		}
	}
	return count, nil
}

func hasSuffix(s string, suffix []string) bool {
	for _, sf := range suffix {
		if strings.HasSuffix(s, sf) {
			return true
		}
	}
	return false
}

// copyFiles copies all non go files from a folder using os.Link
func copyFiles(fromDir, toDir string, readme []byte) error {
	fs, err := ioutil.ReadDir(fromDir)
	if err != nil {
		return context(err)
	}
	for _, f := range fs {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if name[0] == '.' || hasSuffix(name, []string{".go", "~"}) {
			continue
		}
		// copy with os.Link
		if !strings.HasPrefix(strings.ToLower(name), "readme") {
			if err := os.Link(filepath.Join(fromDir, name),
				filepath.Join(toDir, name)); err != nil {
				return context(err)
			}
			continue
		}
		// add header to readme
		buf, err := ioutil.ReadFile(filepath.Join(fromDir, name))
		if err != nil {
			return context(err)
		}
		fo, err := os.Create(filepath.Join(toDir, name))
		if err != nil {
			return context(err)
		}
		defer fo.Close()
		_, err = fo.Write(readme)
		if err != nil {
			return context(err)
		}
		_, err = fo.Write(buf)
		if err != nil {
			return context(err)
		}
	}
	return nil
}
