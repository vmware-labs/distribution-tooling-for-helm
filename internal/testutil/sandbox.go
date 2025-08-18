package testutil

import (
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

var (
	tempFileIndex = 0
	mutex         = &sync.Mutex{}
)

// Sandbox allows manipulating files and directories with paths sandboxed into
// the Root directory
type Sandbox struct {
	sync.RWMutex
	// Root of the sandbox
	Root               string
	temporaryResources []string
}

// NewSandbox returns a new sandbox with the configured root or a random
// temporary one if none is provided
func NewSandbox(args ...string) *Sandbox {
	var root string
	var err error
	if len(args) > 0 {
		root = args[0]
	} else {
		root, err = os.MkdirTemp("", "sandbox")
		if err != nil {
			log.Fatal("Error creating temporary directory for sandbox")
		}
	}
	sb := &Sandbox{Root: root}
	sb.temporaryResources = make([]string, 0)
	return sb
}

// Track registers a path as a temporary one to be deleted on cleanup
func (sb *Sandbox) Track(p string) {
	defer sb.Unlock()
	sb.Lock()
	sb.temporaryResources = append(sb.temporaryResources, sb.Normalize(p))
}

// Touch touches a file inside the sandbox
func (sb *Sandbox) Touch(file string) string {
	f := sb.Normalize(file)
	if fileExists(f) {
		os.Chtimes(f, time.Now(), time.Now())
	} else {
		sb.WriteFile(f, []byte{}, os.FileMode(0766))
	}
	return f
}

// TempFile returns a temporary non-existent file.
// An optional file tail can be provided
func (sb *Sandbox) TempFile(args ...string) string {
	tail := ""
	if len(args) > 0 {
		tail = args[0]
	} else {
		tail = strconv.Itoa(rand.Int()) //nolint:gosec
		// Too long paths in osx result in errors creating sockets (make the daemon tests break)
		// https://github.com/golang/go/issues/6895
		if len(tail) > 10 {
			tail = tail[0:10]
		}
	}
	mutex.Lock()
	tail += strconv.Itoa(tempFileIndex)
	tempFileIndex++
	mutex.Unlock()

	f := sb.Normalize(tail)
	if fileExists(f) {
		suffix := 0
		for fileExists(f + strconv.Itoa(suffix)) {
			suffix++
		}
		f = f + strconv.Itoa(suffix)
	}

	sb.Track(f)
	return f
}

// Mkdir creates a directory inside the sandbox
func (sb *Sandbox) Mkdir(p string, mode os.FileMode) (string, error) {
	f := sb.Normalize(p)
	sb.Track(f)
	return f, os.MkdirAll(f, mode)
}

// Symlink creates a symlink inside the sandbox
func (sb *Sandbox) Symlink(oldname, newname string) (string, error) {
	dest := sb.Normalize(newname)
	sb.Track(dest)
	return dest, os.Symlink(oldname, dest)
}

// Write writes data into the file pointed by path.
// This is a convenience wrapper around WriteFile
func (sb *Sandbox) Write(path string, data string) (string, error) {
	return sb.WriteFile(path, []byte(data), os.FileMode(0644))
}

// WriteFile writes a set of bytes (data) into the file pointed by path and with the specified mode
func (sb *Sandbox) WriteFile(path string, data []byte, mode os.FileMode) (string, error) {
	f := sb.Normalize(path)
	sb.Track(f)
	return f, os.WriteFile(f, data, mode)
}

// Cleanup removes all the resources created by the sandbox
func (sb *Sandbox) Cleanup() error {
	sb.RLock()
	resources := sb.temporaryResources
	sb.RUnlock()
	for _, p := range resources {
		os.RemoveAll(p)
	}
	return os.RemoveAll(sb.Root)
}

// ContainsPath returns true if path is contained inside the sandbox and false otherwise.
// This function does not check for the existence of the file, just checks if the
// path is contained in the sanbox root
func (sb *Sandbox) ContainsPath(path string) bool {
	splitted := fileSplit(path)
	for idx, comp := range fileSplit(sb.Root) {
		if idx >= len(splitted) || comp != splitted[idx] {
			return false
		}
	}
	return true
}

// Normalize returns the fully normalized version of path, including the Root prefix
func (sb *Sandbox) Normalize(path string) string {
	if sb.ContainsPath(path) {
		return path
	}
	return filepath.Join(sb.Root, path)
}
