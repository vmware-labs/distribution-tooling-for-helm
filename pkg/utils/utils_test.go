package utils

import (
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
)

var (
	sb *tu.Sandbox
)

func TestMain(m *testing.M) {
	sb = tu.NewSandbox()
	c := m.Run()

	if err := sb.Cleanup(); err != nil {
		log.Printf("WARN: failed to cleanup test sandbox: %v", err)
	}

	os.Exit(c)
}

func TestFileExists(t *testing.T) {
	existingFile := sb.Touch(sb.TempFile())
	existingDir, _ := sb.Mkdir(sb.TempFile(), os.FileMode(0755))
	nonTraversableDir, _ := sb.Mkdir(sb.TempFile(), os.FileMode(0000))

	nonExistingFile := sb.TempFile()

	for path, expected := range map[string]bool{
		existingFile:      true,
		existingDir:       true,
		nonTraversableDir: true,
		filepath.Join(nonTraversableDir, "dummy.txt"): false,
		nonExistingFile: false,
	} {
		assert.Equal(t, FileExists(path), expected,
			"Expected FileExists('%s') to be '%t'", path, expected)
	}
}

func TestYamlFileSet(t *testing.T) {
	sampleData := "a:\n  b:\n    c: hello\n"

	t.Run("Modifies existing file", func(t *testing.T) {
		sampleYamlFile := sb.TempFile()

		require.NoError(t, os.WriteFile(sampleYamlFile, []byte(sampleData), 0755))
		assert.NoError(t, YamlFileSet(sampleYamlFile, map[string]string{
			"$.a.b.c": "world",
		}))
		data, err := os.ReadFile(sampleYamlFile)
		require.NoError(t, err)
		assert.Equal(t, "a:\n  b:\n    c: world\n", string(data))
	})
	t.Run("Requires file to exist", func(t *testing.T) {
		sampleYamlFile := sb.TempFile()
		require.NoFileExists(t, sampleYamlFile)
		tu.AssertErrorMatch(t, YamlFileSet(sampleYamlFile, map[string]string{
			"$.a.b.c": "world",
		}), regexp.MustCompile("failed to set YAML file.*no such file or directory"))
	})
	t.Run("Fails to set non-existing YAML path", func(t *testing.T) {
		sampleYamlFile := sb.TempFile()
		require.NoError(t, os.WriteFile(sampleYamlFile, []byte(sampleData), 0755))
		tu.AssertErrorMatch(t, YamlFileSet(sampleYamlFile, map[string]string{
			"$.a.b.c.e": "world",
		}), regexp.MustCompile(`failed to set YAML file.*cannot find YAML path.*`))

	})
}

func TestYamlSet(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		replace     map[string]string
		want        string
		expectedErr string
	}{
		{
			name: "Basic YamlSet",

			data: "a:\n  b:\n    c: hello\n",

			replace: map[string]string{"$.a.b.c": "world"},

			want: "a:\n  b:\n    c: world\n",
		},
		{
			name:        "Malformed YAML",
			data:        "\tmalformed\n\tdata",
			expectedErr: `cannot unmarshal YAML data: yaml: found character that cannot start any token`,
		},
		{
			name:        "Malformed Replacement Path",
			data:        "a: b",
			replace:     map[string]string{"$$": "data"},
			expectedErr: `cannot create YAML path: invalid path syntax at position 1`,
		},
		{
			name:        "Fails if path to replace does not exist",
			data:        "a: b",
			replace:     map[string]string{"$.b.c": "data"},
			expectedErr: `cannot find YAML path "$.b.c"`,
		},
		{
			name:        "Fails if finds too many results",
			data:        "a: b\nc: d",
			replace:     map[string]string{"$.*": "data"},
			expectedErr: `expected single result replacing image but found 2`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := YamlSet([]byte(tt.data), tt.replace)
			validateError(t, tt.expectedErr, err)
			if !reflect.DeepEqual(string(got), tt.want) {
				t.Errorf("YamlSet() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestSafeWriteFile(t *testing.T) {
	nonExistingFile := sb.TempFile()
	sampleData := "hello world"
	assert.NoFileExists(t, nonExistingFile)
	assert.NoError(t, SafeWriteFile(nonExistingFile, []byte(sampleData), 0755))
	assert.FileExists(t, nonExistingFile)
	data, err := os.ReadFile(nonExistingFile)
	assert.NoError(t, err)
	if err == nil {
		assert.Equal(t, sampleData, string(data))
	}
}

func TestRelocateImageURL(t *testing.T) {
	type args struct {
		url                string
		prefix             string
		includeIndentifier bool
	}
	newReg := "mycustom.docker.registry.com/airgap"
	dummyTag := "mytag"
	dummyDigest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("sample")))
	tests := map[string]struct {
		args        args
		want        string
		expectedErr string
	}{
		"Basic replacement": {
			args: args{
				url:    "bitnami/wordpress:mytag",
				prefix: newReg,
			},
			want: fmt.Sprintf("%s/bitnami/wordpress", newReg),
		},
		"Basic replacement including tag": {
			args: args{
				url:                fmt.Sprintf("bitnami/wordpress:%s", dummyTag),
				prefix:             newReg,
				includeIndentifier: true,
			},
			want: fmt.Sprintf("%s/bitnami/wordpress:%s", newReg, dummyTag),
		},
		"Basic replacement including tag and digest gives preference to digest": {
			args: args{
				url:                fmt.Sprintf("bitnami/wordpress:%s@%s", dummyTag, dummyDigest),
				prefix:             newReg,
				includeIndentifier: true,
			},
			want: fmt.Sprintf("%s/bitnami/wordpress@%s", newReg, dummyDigest),
		},
		"Replaces full URL with single component": {
			args: args{
				url:    "example.com:80/foo",
				prefix: newReg,
			},
			want: fmt.Sprintf("%s/foo", newReg),
		},
		"Replaces full URL with multiple components": {
			args: args{
				url:    "example.com:80/foo/bar/bitnami/app",
				prefix: newReg,
			},
			want: fmt.Sprintf("%s/bitnami/app", newReg),
		},
		"Replaces library repositoy": {
			args: args{
				url:    "foo",
				prefix: newReg,
			},
			want: fmt.Sprintf("%s/library/foo", newReg),
		},
		"Fails on malformed urls": {
			args: args{
				url:    "incorrect:::url",
				prefix: newReg,
			},
			expectedErr: "failed to relocate url: could not parse reference",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := RelocateImageURL(tt.args.url, tt.args.prefix, tt.args.includeIndentifier)
			validateError(t, tt.expectedErr, err)

			if got != tt.want {
				t.Errorf("RelocateImageURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func validateError(t *testing.T, expectedErr string, err error) {
	if expectedErr != "" {
		if err == nil {
			t.Errorf("expected error %q but it did not fail", expectedErr)
		} else {
			assert.ErrorContains(t, err, expectedErr)
		}

	} else if err != nil {
		t.Errorf("got error = %v but expected to succeed", err)
	}
}

func TestTruncateStringWithEllipsis(t *testing.T) {

	tests := map[string]struct {
		text      string
		maxLength int
		want      string
	}{
		"String short enough": {
			"hello world", 20, "hello world",
		},
		"Truncated string odd length": {
			"This is a long string to truncate", 15, "This [...]ncate",
		},
		"Truncated string even length": {
			"This is a long string to truncate", 20, "This is[...]truncate",
		},
		"Max length too small": {
			"This is a long string to truncate", 5, "This ",
		},
		"Max length too small (2)": {
			"This is a long string to truncate", 1, "T",
		},
		"Max length too small (3)": {
			"This is a long string to truncate", 0, "",
		},
		"Negative max length too small": {
			"This is a long string to truncate", -5, "",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := TruncateStringWithEllipsis(tt.text, tt.maxLength); got != tt.want {
				t.Errorf("TruncateStringWithEllipsis() = %v, want %v", got, tt.want)
			}
		})
	}
}
