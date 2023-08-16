package testutil

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

func fileSplit(p string) []string {
	return strings.Split(filepath.Clean(p), "/")
}

func fileExists(f string) bool {
	if _, err := os.Stat(f); err == nil {
		return true
	}
	return false
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
