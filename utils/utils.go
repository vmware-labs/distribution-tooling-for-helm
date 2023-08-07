// Package utils implements helper functions
package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/vmware-labs/yaml-jsonpath/pkg/yamlpath"
	"gopkg.in/yaml.v3"
)

// FileExists checks if filename exists
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func rawYamlSet(n *yaml.Node, path string, value string) error {
	p, err := yamlpath.NewPath(path)

	if err != nil {
		return fmt.Errorf("cannot create YAML path: %v", err)
	}
	q, err := p.Find(n)
	if err != nil {
		return fmt.Errorf("cannot find YAML path %q: %v", path, err)
	}
	if len(q) == 0 {
		return fmt.Errorf("cannot find YAML path %q", path)
	}
	if len(q) > 1 {
		return fmt.Errorf("expected single result replacing image but found %d", len(q))
	}
	yamlElement := q[0]

	yamlElement.Value = value
	return nil

}

// YamlFileSet sets the list of key-value specified in values in the YAML file.
// The keys are in jsonpath format
func YamlFileSet(file string, values map[string]string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to set YAML file %q: %v", file, err)
	}
	data, err = YamlSet(data, values)
	if err != nil {
		return fmt.Errorf("failed to set YAML file %q: %v", file, err)
	}
	return SafeWriteFile(file, data, 0644)
}

// YamlSet sets the list of key-value specified in values in the YAML data.
// The keys are in jsonpath format
func YamlSet(data []byte, values map[string]string) ([]byte, error) {
	var allErrors error
	var n yaml.Node

	err := yaml.Unmarshal(data, &n)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal YAML data: %v", err)
	}
	for path, value := range values {

		if err := rawYamlSet(&n, path, value); err != nil {
			allErrors = errors.Join(allErrors, err)
		}
	}
	if allErrors != nil {
		return nil, allErrors
	}

	var buf bytes.Buffer
	e := yaml.NewEncoder(&buf)
	e.SetIndent(2)

	err = e.Encode(&n)
	if err != nil {
		return nil, fmt.Errorf("failed to format YAML: %v", err)
	}
	if err := e.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize YAML: %v", err)
	}
	return buf.Bytes(), nil
}

// SafeWriteFile writes data into the specified filename by first creating it, and then renaming
// to the final destination to minimize breaking the file
func SafeWriteFile(filename string, data []byte, perm os.FileMode) error {

	f, err := os.CreateTemp(filepath.Dir(filename), "tmp")
	if err != nil {
		return err
	}
	err = f.Chmod(perm)
	if err != nil {
		return err
	}
	tmpname := f.Name()

	// write data to temp file
	n, err := f.Write(data)
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	}
	if err1 := f.Close(); err == nil {
		err = err1
	}
	if err != nil {
		return err
	}

	return os.Rename(tmpname, filename)
}

// RelocateImageURL rewrites the provided image url by replacing its prefix
func RelocateImageURL(url string, prefix string, includeIndentifier bool) (string, error) {
	ref, err := name.ParseReference(url)
	if err != nil {
		return "", fmt.Errorf("failed to relocate url: %v", err)
	}
	normalizedURL := ref.Context().Name()

	// We will preserve the last past of the repository
	re := regexp.MustCompile("^.*/([^/]+/[^/]+)$")
	match := re.FindStringSubmatch(normalizedURL)
	if match == nil {
		return "", fmt.Errorf("failed to parse normalized URL")
	}
	newURL := fmt.Sprintf("%s/%s", strings.TrimRight(prefix, "/"), match[1])
	if includeIndentifier && ref.Identifier() != "" {
		separator := ":"
		if _, ok := ref.(name.Digest); ok {
			separator = "@"
		}
		newURL = fmt.Sprintf("%s%s%s", newURL, separator, ref.Identifier())
	}
	return newURL, nil
}

// ExecuteWithRetry executes a function retrying until it succeeds or the number of retries is reached
func ExecuteWithRetry(retries int, cb func(try int, prevErr error) error) error {
	retry := 0
	var err error
	for {
		err = cb(retry, err)
		if err == nil {
			break
		}
		if retry < retries {
			retry++
			continue
		}
		return err
	}
	return nil
}
