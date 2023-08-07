package testutil

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	tu "github.com/bitnami/gonit/testutils"
)

var (
	tmplExtension    = ".tmpl"
	partialExtension = ".partial" + tmplExtension
)

var fns = template.FuncMap{
	"isLast": func(index int, len int) bool {
		return index+1 == len
	},
}

// Sandbox defines a filesystem container where to write files
// that can be easily cleaned up
type Sandbox struct {
	*tu.Sandbox
}

// NewSandbox returns a new Sandbox
func NewSandbox() *Sandbox {
	return &Sandbox{tu.NewSandbox()}
}

// RenderTemplateString renders a golang template defined in str with the provided tplData.
// It can receive an optional list of files to parse, including templates
func RenderTemplateString(str string, tplData interface{}, files ...string) (string, error) {
	tmpl := template.New("test")
	localFns := template.FuncMap{"include": func(name string, data interface{}) (string, error) {
		buf := bytes.NewBuffer(nil)
		if err := tmpl.ExecuteTemplate(buf, name, data); err != nil {
			return "", err
		}
		return buf.String(), nil
	},
	}

	tmpl, err := tmpl.Funcs(fns).Funcs(sprig.FuncMap()).Funcs(localFns).Parse(str)
	if err != nil {
		return "", err
	}
	if len(files) > 0 {
		if _, err := tmpl.ParseFiles(files...); err != nil {
			return "", err
		}
	}
	b := &bytes.Buffer{}

	if err := tmpl.Execute(b, tplData); err != nil {
		return "", err
	}
	return strings.TrimSpace(b.String()), nil
}

// RenderTemplateFile renders the golang template specified in file with the provided tplData.
// It can receive an optional list of files to parse, including templates
func RenderTemplateFile(file string, tplData interface{}, files ...string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return RenderTemplateString(string(data), tplData, files...)
}

// RenderScenario renders a full directory specified by origin in the destDir directory with
// the specified data
func RenderScenario(origin string, destDir string, data map[string]interface{}) error {
	matches, err := filepath.Glob(origin)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("cannot find any files at %q", origin)
	}
	templateFiles, err := filepath.Glob(filepath.Join(origin, fmt.Sprintf("*%s", partialExtension)))
	_ = templateFiles
	if err != nil {
		return fmt.Errorf("faled to list template partials")
	}
	for _, p := range matches {
		rootDir := filepath.Dir(filepath.Clean(p))
		err := filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
			if strings.HasSuffix(path, partialExtension) {
				return nil
			}
			relative, _ := filepath.Rel(rootDir, path)
			destFile := filepath.Join(destDir, relative)

			if info.Mode().IsRegular() {
				if strings.HasSuffix(path, tmplExtension) {
					destFile = strings.TrimSuffix(destFile, tmplExtension)
					rendered, err := RenderTemplateFile(path, data, templateFiles...)
					if err != nil {
						return fmt.Errorf("failed to render template %q: %v", path, err)
					}

					if err := os.WriteFile(destFile, []byte(rendered), 0644); err != nil {
						return err
					}
				} else {
					err := copyFile(path, destFile)
					if err != nil {
						return fmt.Errorf("failed to copy %q: %v", path, err)
					}
				}
			} else if info.IsDir() {
				if err := os.MkdirAll(destFile, info.Mode()); err != nil {
					return fmt.Errorf("failed to create directory: %v", err)
				}
			} else {
				return fmt.Errorf("unknown file type (%s)", path)
			}
			if err := os.Chmod(destFile, info.Mode().Perm()); err != nil {
				log.Printf("DEBUG: failed to change file %q permissions: %v", destFile, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func copyFile(srcFile string, destFile string) error {
	src, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer src.Close()

	dest, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, src)
	if err != nil {
		return err
	}

	return dest.Sync()
}
