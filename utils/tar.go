package utils

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MaxDecompressionSize established a high enough maximum tar size to decompres
// to prevent decompression bombs (8GB)
var MaxDecompressionSize int64 = 8 * 1024 * 1024 * 1024

// Maybe use this 	"github.com/mholt/archiver/v4" ?

func tarFile(tarWriter *tar.Writer, source string, relativePath string, info os.FileInfo) error {
	// Create a new tar header for the file
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = relativePath

	err = tarWriter.WriteHeader(header)
	if err != nil {
		return err
	}
	// If the file is a regular file, write its contents to the tar
	if !info.IsDir() {
		file, err := os.Open(source)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		if err != nil {
			return err
		}
	}
	return nil
}

// TarConfiguration defines the Tar char opts
type TarConfiguration struct {
	Prefix string
	Skip   func(f string) bool
}

// Tar calls TarContext with a Background context
func Tar(sourceDir string, filename string, cfg TarConfiguration) error {
	return TarContext(context.Background(), sourceDir, filename, cfg)
}

// TarContext compresses the provided sourceDir directory into the .tar.gz specified in filename,
// adding prefix to the added files.
func TarContext(parentCtx context.Context, sourceDir string, filename string, cfg TarConfiguration) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	prefix := cfg.Prefix
	skip := cfg.Skip
	if skip == nil {
		skip = func(f string) bool { return false }
	}
	dir := filepath.Dir(filename)
	if !FileExists(dir) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create destination directory %q: %w", dir, err)
		}
	}

	fh, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create tar.gz filename %q: %w", filename, err)
	}
	defer fh.Close()

	gzWriter := gzip.NewWriter(fh)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	// Walk through the directory and add files to the tar
	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled execution")
		default:
			if err != nil {
				return err
			}
			trimmedPath := strings.TrimPrefix(path, sourceDir)
			if trimmedPath == "" {
				// We are considering the sourceDir itself, just skip
				return nil
			}

			if skip(trimmedPath) {
				return nil
			}
			relPath := filepath.Join(prefix, trimmedPath)

			return tarFile(tarWriter, path, relPath, info)
		}
	})
	return err
}

func stripPathComponents(filename string, stripComponents int) string {
	if stripComponents <= 0 {
		return filepath.FromSlash(filename)
	}

	elemList := strings.Split(filepath.ToSlash(filename), "/")
	if len(elemList) <= stripComponents {
		return ""
	}
	return filepath.FromSlash(filepath.Join(elemList[stripComponents:]...))
}

func untarFile(tr *tar.Reader, dest string, header *tar.Header) error {
	fi := header.FileInfo()
	mode := fi.Mode()
	switch {
	case mode.IsRegular():
		dir := filepath.Dir(dest)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		wf, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode.Perm())
		if err != nil {
			return err
		}

		n, err := io.CopyN(wf, tr, MaxDecompressionSize)
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("error writing to %s: %v", dest, err)
		} else if n == MaxDecompressionSize {
			return fmt.Errorf("size of decoded data exceeds allowed size %d", MaxDecompressionSize)
		}

		if closeErr := wf.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("error writing to %s: %v", dest, err)
		}
		if n != header.Size {
			return fmt.Errorf("only wrote %d bytes to %s; expected %d", n, dest, header.Size)
		}
	case mode.IsDir():
		if err := os.MkdirAll(dest, 0755); err != nil {
			return err
		}
	default:
		return fmt.Errorf("tar file entry %s contained unsupported file type %v", header.Name, mode)
	}
	return nil
}

// Untar decompresses the provided filename into the outputDir
// Simplified implementation taken from: golang.org/x/build/internal/untar (BSD license)
func Untar(filename string, outputDir string, stripComponents int) error {
	return UntarContext(context.Background(), filename, outputDir, stripComponents)
}

// UntarContext decompresses the provided filename into the outputDir
// Simplified implementation taken from: golang.org/x/build/internal/untar (BSD license)
func UntarContext(ctx context.Context, filename string, outputDir string, stripComponents int) error {
	fh, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer fh.Close()
	gzr, err := gzip.NewReader(fh)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
Loop:
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled execution")
		default:
			f, err := tr.Next()

			if err == io.EOF {
				break Loop
			}
			if err != nil {
				return fmt.Errorf("failed to read tar file: %w", err)
			}
			rel := stripPathComponents(f.Name, stripComponents)
			// nothing left after stripping
			if rel == "" {
				continue Loop
			}

			abs := filepath.Join(outputDir, rel)

			if err := untarFile(tr, abs, f); err != nil {
				return err
			}
		}
	}
	return nil
}

// IsTarFile checks if the specified filename is a tar.gz file
func IsTarFile(filename string) (bool, error) {
	fi, err := os.Stat(filename)
	if err != nil {
		return false, fmt.Errorf("cannot check file type: %w", err)
	}
	if fi.Mode().IsDir() {
		return false, nil
	}
	fh, err := os.Open(filename)
	if err != nil {
		return false, fmt.Errorf("fail to open file: %w", err)
	}
	defer fh.Close()
	gzr, err := gzip.NewReader(fh)
	if err != nil {
		return false, nil
	}
	defer gzr.Close()
	return true, nil
}
