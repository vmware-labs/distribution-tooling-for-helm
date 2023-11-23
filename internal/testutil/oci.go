package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"oras.land/oras-go/v2/content/oci"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
)

func parseFileRef(reference string, defaultMetadata string) (filePath, metadata string, err error) {
	i := strings.LastIndex(reference, ":")
	if i < 0 {
		filePath, metadata = reference, defaultMetadata
	} else {
		filePath, metadata = reference[:i], reference[i+1:]
	}
	if filePath == "" {
		return "", "", fmt.Errorf("found empty file path in %q", reference)
	}
	return filePath, metadata, nil
}

func listFilesRecursively(dirPath string) ([]string, error) {

	var files []string

	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		files = append(files, dirPath)
		return files, nil
	}

	if err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return files, nil
}
func loadDir(ctx context.Context, store *file.Store, annotations map[string]map[string]string, dir string) ([]ocispec.Descriptor, error) {
	files, err := listFilesRecursively(dir)
	if err != nil {
		return nil, err
	}
	return loadFiles(ctx, store, annotations, files, dir)
}

func loadFiles(ctx context.Context, store *file.Store, annotations map[string]map[string]string, fileRefs []string, rootDir string) ([]ocispec.Descriptor, error) {
	var files []ocispec.Descriptor
	for _, fileRef := range fileRefs {
		filename, mediaType, err := parseFileRef(fileRef, "")
		if err != nil {
			return nil, err
		}

		name := filepath.Clean(filename)
		if !filepath.IsAbs(name) {
			name = filepath.ToSlash(name)
		}
		if rootDir != "" {
			name, err = filepath.Rel(rootDir, name)
			if err != nil {
				return nil, err
			}
		}

		file, err := store.Add(ctx, name, mediaType, filename)
		if err != nil {
			return nil, err
		}
		if value, ok := annotations[filename]; ok {
			if file.Annotations == nil {
				file.Annotations = value
			} else {
				for k, v := range value {
					file.Annotations[k] = v
				}
			}
		}
		files = append(files, file)
	}

	return files, nil
}

// UnpackOCILayout takes an oci-layout directory and extracts its artifacts to the destDir
func UnpackOCILayout(ctx context.Context, srcLayout string, destDir string) error {
	src, err := oci.NewFromFS(ctx, os.DirFS(srcLayout))
	if err != nil {
		return err
	}

	l, err := layout.ImageIndexFromPath(srcLayout)
	if err != nil {
		return err
	}
	man, err := l.IndexManifest()
	if err != nil {
		return err
	}

	if len(man.Manifests) > 1 {
		return fmt.Errorf("found too many manifests (expected 1)")
	}

	tag := man.Manifests[0].Digest.String()

	dest, err := file.New(destDir)
	if err != nil {
		return err
	}
	defer dest.Close()

	if _, err := oras.Copy(ctx, src, tag, dest, "", oras.DefaultCopyOptions); err != nil {
		return err
	}

	return nil
}

// CreateOCILayout creates a oc-layout directory from a source directory containing a set of files
func CreateOCILayout(ctx context.Context, srcDir, destDir string) error {

	dest, err := oci.New(destDir)

	if err != nil {
		return err
	}
	store, err := file.New("")
	if err != nil {
		return err
	}
	defer store.Close()

	packOpts := oras.PackManifestOptions{}

	descs, err := loadDir(ctx, store, nil, srcDir)
	if err != nil {
		return err
	}

	packOpts.Layers = descs

	pack := func() (ocispec.Descriptor, error) {
		root, err := oras.PackManifest(ctx, store, oras.PackManifestVersion1_1_RC4, oras.MediaTypeUnknownArtifact, packOpts)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		if err = store.Tag(ctx, root, root.Digest.String()); err != nil {
			return ocispec.Descriptor{}, err
		}
		return root, nil
	}
	root, err := pack()
	if err != nil {
		return err
	}
	err = oras.CopyGraph(context.Background(), store, dest, root, oras.CopyGraphOptions{})

	if err != nil {
		return err
	}
	return err
}

func pushArtifact(ctx context.Context, image string, dir string) error {
	opts := []crane.Option{crane.WithContext(ctx)}

	img, err := loadImage(dir)
	if err != nil {
		return err
	}

	switch t := img.(type) {
	case v1.Image:
		return crane.Push(t, image, opts...)
	default:
		return fmt.Errorf("unsupported image type %T", t)
	}
}

// PullArtifact downloads an artifact from a remote oci into a oci-layout
func PullArtifact(ctx context.Context, src string, dir string) error {
	craneOpts := []crane.Option{crane.WithContext(ctx)}
	o := crane.GetOptions(craneOpts...)

	ref, err := name.ParseReference(src, o.Name...)

	if err != nil {
		return fmt.Errorf("failed to parse reference %q: %w", src, err)
	}
	desc, err := remote.Get(ref, o.Remote...)
	if err != nil {
		return fmt.Errorf("failed to get remote descriptor: %w", err)
	}

	img, err := desc.Image()
	if err != nil {
		return err
	}

	if err := crane.SaveOCI(img, dir); err != nil {
		return fmt.Errorf("failed to save image: %v", err)
	}
	return nil
}

func loadImage(path string) (partial.WithRawManifest, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("expected %q to be a directory", path)
	}

	l, err := layout.ImageIndexFromPath(path)
	if err != nil {
		return nil, fmt.Errorf("loading %s as OCI layout: %w", path, err)
	}

	m, err := l.IndexManifest()
	if err != nil {
		return nil, err
	}
	if len(m.Manifests) != 1 {
		return nil, fmt.Errorf("layout contains multiple entries (%d)", len(m.Manifests))
	}

	desc := m.Manifests[0]
	return l.Image(desc.Digest)
}
