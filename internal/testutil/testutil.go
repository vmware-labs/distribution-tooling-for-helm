package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"
	"gopkg.in/yaml.v2"
)

var (
	tmplExtension    = ".tmpl"
	partialExtension = ".partial" + tmplExtension
)

var fns = template.FuncMap{
	"isLast": func(index int, length int) bool {
		return index+1 == length
	},
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
		err := filepath.Walk(p, func(path string, info os.FileInfo, _ error) error {
			if strings.HasSuffix(path, partialExtension) {
				return nil
			}
			relative, _ := filepath.Rel(p, path)
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

type sampleImageData struct {
	Index     v1.ImageIndex
	ImageData ImageData
}

func createSampleImages(imageName string, server string) (map[string]sampleImageData, error) {
	images := make(map[string]sampleImageData, 0)
	src := fmt.Sprintf("%s/%s", server, imageName)
	imageData := ImageData{Name: "test", Image: imageName}
	base := mutate.IndexMediaType(empty.Index, types.DockerManifestList)

	addendums := []mutate.IndexAddendum{}

	for _, plat := range []string{
		"linux/amd64",
		"linux/arm64",
	} {
		img, err := crane.Image(map[string][]byte{
			"platform.txt": []byte(fmt.Sprintf("Image: %s ; plaform: %s", imageName, plat)),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create image: %v", err)
		}
		parts := strings.Split(plat, "/")

		img, err = mutate.ConfigFile(img, &v1.ConfigFile{Architecture: parts[1], OS: parts[0]})
		if err != nil {
			return nil, fmt.Errorf("cannot mutatle image config file: %w", err)
		}

		img, err = mutate.Canonical(img)

		if err != nil {
			return nil, fmt.Errorf("failed to canonicalize image: %w", err)
		}

		addendums = append(addendums, mutate.IndexAddendum{
			Add: img,
			Descriptor: v1.Descriptor{
				Platform: &v1.Platform{
					OS:           parts[0],
					Architecture: parts[1],
				},
			},
		})
		d, err := img.Digest()
		if err != nil {
			return nil, fmt.Errorf("failed to generate digest: %v", err)
		}
		imageData.Digests = append(imageData.Digests, DigestData{Arch: plat, Digest: digest.Digest(d.String())})
	}

	idx := mutate.AppendManifests(base, addendums...)

	images[src] = sampleImageData{Index: idx, ImageData: imageData}
	return images, nil
}

// Auth defines the authentication information to access the container registry
type Auth struct {
	Username string
	Password string
}

// Config defines multiple test util options
type Config struct {
	SignKey     string
	MetadataDir string
	Auth        Auth
}

// NewConfig returns a new Config
func NewConfig(opts ...Option) *Config {
	cfg := &Config{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Option defines a Config option
type Option func(*Config)

// WithSignKey sets a signing key to be used while pushing images
func WithSignKey(key string) Option {
	return func(cfg *Config) {
		cfg.SignKey = key
	}
}

// WithMetadataDir sets a signing key to be used while pushing images
func WithMetadataDir(dir string) Option {
	return func(cfg *Config) {
		cfg.MetadataDir = dir
	}
}

// WithAuth sets the credentials to access the container registry
func WithAuth(username, password string) Option {
	return func(cfg *Config) {
		cfg.Auth.Username = username
		cfg.Auth.Password = password
	}
}

// AddSampleImagesToRegistry adds a set of sample images to the provided registry
func AddSampleImagesToRegistry(imageName string, server string, opts ...Option) ([]ImageData, error) {
	cfg := NewConfig(opts...)
	images := make([]ImageData, 0)
	samples, err := createSampleImages(imageName, server)
	if err != nil {
		return nil, err
	}
	authenticator := authn.Anonymous
	if cfg.Auth.Username != "" && cfg.Auth.Password != "" {
		authenticator = &authn.Basic{Username: cfg.Auth.Username, Password: cfg.Auth.Password}
	}

	for src, data := range samples {
		ref, err := name.ParseReference(src)
		if err != nil {
			return nil, fmt.Errorf("failed to parse reference: %v", err)
		}
		if err := remote.WriteIndex(ref, data.Index, remote.WithAuth(authenticator)); err != nil {
			return nil, fmt.Errorf("failed to write index: %v", err)
		}
		images = append(images, data.ImageData)
		if cfg.SignKey != "" {
			if err := CosignImage(src, cfg.SignKey, crane.WithAuth(authenticator)); err != nil {
				return nil, fmt.Errorf("failed to sign image %q: %v", src, err)
			}
		}
		if cfg.MetadataDir != "" {
			newDir := fmt.Sprintf("%s.layout", cfg.MetadataDir)
			if err := CreateOCILayout(context.Background(), cfg.MetadataDir, newDir); err != nil {
				return nil, fmt.Errorf("failed to serialize metadata as OCI layout: %v", err)
			}
			imgDigest, err := data.Index.Digest()
			if err != nil {
				return nil, fmt.Errorf("failed to get image digest: %v", err)
			}
			metadataImg := fmt.Sprintf("%s:sha256-%s.metadata", ref.Context().Name(), imgDigest.Hex)

			if err := pushArtifact(context.Background(), metadataImg, newDir, crane.WithAuth(authenticator)); err != nil {
				return nil, fmt.Errorf("failed to push metadata: %v", err)
			}
			if cfg.SignKey != "" {
				if err := CosignImage(metadataImg, cfg.SignKey, crane.WithAuth(authenticator)); err != nil {
					return nil, fmt.Errorf("failed to sign image %q: %v", src, err)
				}
			}

		}
	}
	return images, nil
}

// CreateSingleArchImage creates a sample image for the specified platform
func CreateSingleArchImage(imageData *ImageData, plat string) (v1.Image, error) {
	imageName := imageData.Image

	img, err := crane.Image(map[string][]byte{
		"platform.txt": []byte(fmt.Sprintf("Image: %s ; plaform: %s", imageName, plat)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create image: %w", err)
	}
	parts := strings.Split(plat, "/")
	img, err = mutate.ConfigFile(img, &v1.ConfigFile{Architecture: parts[1], OS: parts[0]})
	if err != nil {
		return nil, fmt.Errorf("cannot mutatle image config file: %w", err)
	}

	img, err = mutate.Canonical(img)
	if err != nil {
		return nil, fmt.Errorf("failed to canonicalize image: %w", err)
	}
	d, err := img.Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to get image digest: %w", err)
	}
	imageData.Digests = append(imageData.Digests, DigestData{Arch: plat, Digest: digest.Digest(d.String())})

	return img, nil
}

// CreateSampleImages create a multiplatform sample image
func CreateSampleImages(imageData *ImageData, archs []string) ([]v1.Image, error) {
	craneImgs := []v1.Image{}

	for _, plat := range archs {
		img, err := CreateSingleArchImage(imageData, plat)
		if err != nil {
			return nil, err
		}
		craneImgs = append(craneImgs, img)
	}
	return craneImgs, nil
}

// ReadRemoteImageManifest reads the image src digests from a remote repository
func ReadRemoteImageManifest(src string, opts ...Option) (map[string]DigestData, error) {
	cfg := NewConfig(opts...)
	authenticator := authn.Anonymous
	if cfg.Auth.Username != "" && cfg.Auth.Password != "" {
		authenticator = &authn.Basic{Username: "username", Password: "password"}
	}
	o := crane.GetOptions(crane.WithAuth(authenticator))

	ref, err := name.ParseReference(src, o.Name...)

	if err != nil {
		return nil, fmt.Errorf("failed to parse reference %q: %w", src, err)
	}
	desc, err := remote.Get(ref, o.Remote...)
	if err != nil {
		return nil, fmt.Errorf("failed to get remote image: %w", err)
	}

	var idx v1.IndexManifest
	if err := json.Unmarshal(desc.Manifest, &idx); err != nil {
		return nil, fmt.Errorf("failed to parse images data")
	}
	digests := make(map[string]DigestData, 0)

	var allErrors error
	for _, img := range idx.Manifests {
		// Skip attestations
		if img.Annotations["vnd.docker.reference.type"] == "attestation-manifest" {
			continue
		}
		switch img.MediaType {
		case types.OCIManifestSchema1, types.DockerManifestSchema2:
			if img.Platform == nil {
				continue
			}

			arch := fmt.Sprintf("%s/%s", img.Platform.OS, img.Platform.Architecture)
			imgDigest := DigestData{
				Digest: digest.Digest(img.Digest.String()),
				Arch:   arch,
			}
			digests[arch] = imgDigest
		default:
			allErrors = errors.Join(allErrors, fmt.Errorf("unknown media type %q", img.MediaType))
			continue
		}
	}
	return digests, allErrors
}

// MustNormalizeYAML returns the normalized version of the text YAML or panics
func MustNormalizeYAML(text string) string {
	t, err := NormalizeYAML(text)
	if err != nil {
		panic(err)
	}
	return t
}

// NormalizeYAML returns a normalized version of the provided YAML text
func NormalizeYAML(text string) (string, error) {
	var out interface{}
	err := yaml.Unmarshal([]byte(text), &out)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(out)
	return string(data), err
}
