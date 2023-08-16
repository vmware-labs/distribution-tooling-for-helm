package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSandbox(t *testing.T) {
	root, err := os.MkdirTemp("", "sandbox")
	require.NoError(t, err)

	sb1 := NewSandbox(root)
	defer sb1.Cleanup()
	assert.Equal(t, root, sb1.Root)

	sb2 := NewSandbox()
	defer sb2.Cleanup()

	assert.Regexp(t,
		regexp.MustCompile(fmt.Sprintf(`^%s/+sandbox.*$`, filepath.Clean(os.TempDir()))),
		sb2.Root)

	for _, s := range []*Sandbox{sb1, sb2} {
		if !fileExists(s.Root) {
			assert.Fail(t, "Expected %s to exists", s.Root)
		} else {
			s.Cleanup()
			if fileExists(s.Root) {
				assert.Fail(t, "Expected %s to not exist", s.Root)
			}
		}
	}
}

func TestCleanup(t *testing.T) {
	sb := NewSandbox()
	defer sb.Cleanup()

	tmpFile := filepath.Join(sb.Root, "sample.txt")
	err := os.WriteFile(tmpFile, []byte{}, os.FileMode(0644))
	require.NoError(t, err)

	assert.NoError(t, sb.Cleanup())
	// Even if not created by the sandbox, we delete stuff if contained inside
	assert.False(t, fileExists(tmpFile), "Expected %s to exists", tmpFile)

	sb.Track(tmpFile)

	assert.NoError(t, sb.Cleanup())
	sb.Mkdir(sb.Root, os.FileMode(0755))

	assert.False(t, fileExists(tmpFile), "Expected %s to not exist", tmpFile)

	p1 := sb.Touch("foo.txt")
	assert.True(t, fileExists(p1), "Expected %s to exists", p1)

	p2, err := sb.Mkdir("some/dir/bar", os.FileMode(0755))
	assert.NoError(t, err)
	assert.True(t, fileExists(p2), "Expected %s to exists", p2)

	assert.NoError(t, sb.Cleanup())

	assert.False(t, fileExists(sb.Root))
}

func TestNormalize(t *testing.T) {
	root, err := os.MkdirTemp("", "sandbox")
	require.NoError(t, err)

	sb := NewSandbox(root)
	defer sb.Cleanup()

	for _, path := range []string{
		"/tmp/foo/bar", "/", "a.txt", "a/b/c/d", "",
	} {
		assert.Equal(t, sb.Normalize(path), filepath.Join(root, path))
	}
}

func TestContainsPath(t *testing.T) {

	root, err := os.MkdirTemp("", "sandbox")
	require.NoError(t, err)

	sb := NewSandbox(root)
	defer sb.Cleanup()

	for path, isContained := range map[string]bool{
		"/tmp": false,
		root:   true,
		"/":    false,
		// Relative paths are dangerous in this context, require full paths
		"var/foo.txt":                        false,
		"/var/foo.txt":                       false,
		filepath.Join(root, "sample.txt"):    true,
		filepath.Join(root, "../sample.txt"): false,
		"../sample.txt":                      false,
	} {
		assert.Equal(t, isContained,
			sb.ContainsPath(path),
			"Path %s fails is contained check in %s", path, root)
	}
}

func TestWriteFile(t *testing.T) {
	root, err := os.MkdirTemp("", "sandbox")
	require.NoError(t, err)

	sb := NewSandbox(root)
	defer sb.Cleanup()

	data := "hello worlds!"
	tail := "sample.txt"
	f, err := sb.WriteFile(tail, []byte(data), os.FileMode(0644))
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(root, tail), f)
	read, err := os.ReadFile(f)
	assert.NoError(t, err)

	assert.Equal(t, data, string(read))
}

func TestWrite(t *testing.T) {

	root, err := os.MkdirTemp("", "sandbox")
	require.NoError(t, err)

	sb := NewSandbox(root)
	defer sb.Cleanup()

	data := "hello worlds!"
	tail := "sample.txt"
	f, err := sb.Write(tail, data)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(root, tail), f)
	read, err := os.ReadFile(f)
	assert.NoError(t, err)

	assert.Equal(t, data, string(read))
}

func TestSymlink(t *testing.T) {
	root, err := os.MkdirTemp("", "sandbox")
	require.NoError(t, err)

	sb := NewSandbox(root)
	defer sb.Cleanup()

	f, err := sb.Symlink("../a", "b")
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "b"), f)
	d, _ := os.Readlink(f)
	assert.Equal(t, d, "../a")
}

func TestMkdir(t *testing.T) {
	root, err := os.MkdirTemp("", "sandbox")
	require.NoError(t, err)

	sb := NewSandbox(root)
	defer sb.Cleanup()

	tail := "sample_dir"
	fullPath := filepath.Join(root, tail)
	assert.False(t, fileExists(fullPath), "Expected %s to not exist", fullPath)
	f, err := sb.Mkdir(tail, os.FileMode(0755))
	assert.NoError(t, err)
	assert.Equal(t, fullPath, f)
	require.True(t, fileExists(fullPath), "Expected %s to exists", fullPath)

	s, _ := os.Stat(fullPath)
	assert.True(t, s.IsDir(), "Expected %s to be a directory", fullPath)
}
func TestTouch(t *testing.T) {
	root, err := os.MkdirTemp("", "sandbox")
	require.NoError(t, err)

	sb := NewSandbox(root)
	defer sb.Cleanup()

	tail := "sample.txt"
	fullPath := filepath.Join(root, tail)
	assert.False(t, fileExists(fullPath), "Expected %s to not exist", fullPath)
	assert.Equal(t, fullPath, sb.Touch(tail))
	require.True(t, fileExists(fullPath), "Expected %s to exists", fullPath)

	s1, _ := os.Stat(fullPath)
	mt1 := s1.ModTime()

	// To avoid issues on OS X, we have to wait more then 1sec (HFS+ only stores timestamps to a granularity of one second)
	// http://stackoverflow.com/questions/18403588/how-to-return-millisecond-information-for-file-access-on-mac-os-x-in-java/18404059#18404059
	time.Sleep(1500 * time.Millisecond)
	sb.Touch(tail)
	s2, _ := os.Stat(fullPath)
	mt2 := s2.ModTime()
	assert.NotEqual(t, mt1, mt2)
}

func TestTempFile(t *testing.T) {
	root, err := os.MkdirTemp("", "sandbox")
	require.NoError(t, err)

	sb := NewSandbox(root)
	defer sb.Cleanup()

	tail := "sample.txt"
	f1 := sb.TempFile(tail)
	f2 := sb.TempFile(tail)
	assert.NotEqual(t, f1, f2)
	for _, f := range []string{f1, f2} {
		assert.False(t, fileExists(f))
		assert.Regexp(t,
			regexp.MustCompile(fmt.Sprintf("^%s/%s[0-9]+$", root, tail)),
			f)
	}

	// tempFileIndex is index incremented each time a tmp file is requested
	// If the file to create exists, an additional numeric index is appended until
	// the target file does not exists
	currentIndex := tempFileIndex
	os.WriteFile(
		filepath.Join(root, fmt.Sprintf("%s%d", tail, currentIndex)),
		[]byte{}, os.FileMode(0644),
	)
	f3 := sb.TempFile(tail)
	// If the file exists, it starts incrementing the index
	assert.Equal(t, fmt.Sprintf("%s/%s%d0", root, tail, currentIndex), f3)

	currentIndex = tempFileIndex
	os.WriteFile(
		filepath.Join(root, fmt.Sprintf("%s%d", tail, currentIndex)),
		[]byte{}, os.FileMode(0644),
	)
	for i := 0; i < 2; i++ {
		os.WriteFile(
			filepath.Join(root, fmt.Sprintf("%s%d%d", tail, currentIndex, i)),
			[]byte{}, os.FileMode(0644),
		)
	}
	f4 := sb.TempFile(tail)
	// If the file exists, it starts incrementing the index
	assert.Equal(t, fmt.Sprintf("%s/%s%d2", root, tail, currentIndex), f4)

	f5 := sb.TempFile()
	f6 := sb.TempFile()
	assert.NotEqual(t, f5, f6)
	for _, f := range []string{f5, f6} {
		assert.False(t, fileExists(f))
		assert.Regexp(t,
			regexp.MustCompile(fmt.Sprintf("^%s/[0-9]+$", root)),
			f)
	}
}
