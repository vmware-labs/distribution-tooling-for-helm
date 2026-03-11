// Package utils implements helper functions
package utils

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
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

	if err := yaml.Unmarshal(data, &n); err != nil {
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

	if err := e.Encode(&n); err != nil {
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

// relocateRepoRe matches the last one or two path components of a normalized image URL.
var relocateRepoRe = regexp.MustCompile("^.*?/(([^/]+/)?[^/]+)$")

// RelocateImageURL rewrites the provided image url by replacing its prefix.
// includeIdentifier controls whether the tag or digest is appended to the result.
// preserveRepository controls whether the last two path components of the source repository
// are preserved in the relocated URL (e.g. "myregistry/bitnami/wordpress" keeps "bitnami/wordpress").
// When false, only the image base name is kept (e.g. "myregistry/wordpress").
// Use true for Helm chart wraps where the repository structure is meaningful, and false
// for standalone container image wraps where only the image name matters.
func RelocateImageURL(url string, prefix string, includeIdentifier, preserveRepository bool) (string, error) {
	ref, err := name.ParseReference(url)
	if err != nil {
		return "", fmt.Errorf("failed to relocate url: %v", err)
	}
	normalizedURL := ref.Context().Name()

	app := path.Base(normalizedURL)
	newURL := fmt.Sprintf("%s/%s", strings.TrimRight(prefix, "/"), app)
	if preserveRepository {
		// We will preserve the last part of the repository
		match := relocateRepoRe.FindStringSubmatch(normalizedURL)
		if match == nil {
			return "", fmt.Errorf("failed to parse normalized URL")
		}
		newURL = fmt.Sprintf("%s/%s", strings.TrimRight(prefix, "/"), match[1])
	}

	if includeIdentifier && ref.Identifier() != "" {
		separator := ":"
		if _, ok := ref.(name.Digest); ok {
			separator = "@"
		}
		newURL = fmt.Sprintf("%s%s%s", newURL, separator, ref.Identifier())
	}
	return newURL, nil
}

// ParseImageReference parses an OCI image reference and returns a filesystem-safe base name,
// a tag, and a digest. Exactly one of tag or digest will be non-empty:
//   - If the reference contains a digest (@sha256:...), digest is set and tag is empty.
//   - If the reference contains a tag, tag is set and digest is empty.
//   - If neither is present, tag defaults to "latest" and digest is empty.
//
// The oci:// scheme prefix is stripped if present.
func ParseImageReference(imageRef string) (name, tag, digest string) {
	ref := strings.TrimPrefix(imageRef, "oci://")

	// Extract tag or digest before stripping path separators
	if idx := strings.Index(ref, "@"); idx != -1 {
		digest = ref[idx+1:]
	} else if idx := strings.LastIndex(ref, ":"); idx != -1 {
		tag = ref[idx+1:]
	} else {
		tag = "latest"
	}

	// Use only the last path segment of the repository
	base := ref
	if idx := strings.LastIndex(base, "/"); idx != -1 {
		base = base[idx+1:]
	}
	// Strip tag or digest suffix
	base = strings.SplitN(base, "@", 2)[0]
	base = strings.SplitN(base, ":", 2)[0]
	name = base
	return name, tag, digest
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

// TruncateStringWithEllipsis returns a truncated version of text
func TruncateStringWithEllipsis(text string, maxLength int) string {
	if len(text) <= maxLength {
		return text
	}
	if maxLength <= 0 {
		return ""
	}

	ellipsis := "[...]"

	// If the maxLength is so small the ellipsis does not fit, just return the prefix
	if maxLength <= len(ellipsis) {
		return text[0:maxLength]
	}
	startSplit := (maxLength - len(ellipsis)) / 2
	endSplit := len(text) - (maxLength - startSplit - len(ellipsis))
	return text[0:startSplit] + ellipsis + text[endSplit:]
}
