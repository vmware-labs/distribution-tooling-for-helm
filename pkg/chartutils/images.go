package chartutils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/imagelock"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"
)

func getNumberOfArtifacts(images imagelock.ImageList) int {
	n := 0
	for _, imgDesc := range images {
		n += len(imgDesc.Digests)
	}
	return n
}

func getArtifactsDir(defaultValue string, cfg *Configuration) string {
	if cfg.ArtifactsDir != "" {
		return cfg.ArtifactsDir
	}
	return defaultValue
}

// PullImages downloads the list of images specified in the provided ImagesLock
func PullImages(lock *imagelock.ImagesLock, imagesDir string, opts ...Option) error {

	cfg := NewConfiguration(opts...)
	ctx := cfg.Context

	artifactsDir := getArtifactsDir(filepath.Join(imagesDir, "artifacts"), cfg)

	o := crane.GetOptions(crane.WithContext(ctx))

	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create bundle directory: %v", err)
	}
	l := cfg.Log

	p, _ := cfg.ProgressBar.WithTotal(getNumberOfArtifacts(lock.Images)).UpdateTitle("Pulling Images").Start()
	defer p.Stop()
	maxRetries := cfg.MaxRetries

	for _, imgDesc := range lock.Images {
		for _, dgst := range imgDesc.Digests {
			select {
			// Early abort if the context is done
			case <-ctx.Done():
				return fmt.Errorf("cancelled execution")
			default:
				p.Add(1)
				p.UpdateTitle(fmt.Sprintf("Saving image %s/%s %s (%s)", imgDesc.Chart, imgDesc.Name, imgDesc.Image, dgst.Arch))
				err := utils.ExecuteWithRetry(maxRetries, func(try int, prevErr error) error {
					if try > 0 {
						// The context is done, so we are not retrying, just return the error
						if ctx.Err() != nil {
							return prevErr
						}
						l.Debugf("Failed to pull image: %v", prevErr)
						p.Warnf("Failed to pull image: retrying %d/%d", try, maxRetries)
					}
					if _, err := pullImage(imgDesc.Image, dgst, imagesDir, o); err != nil {
						return err
					}
					return nil
				})

				if err != nil {
					return fmt.Errorf("failed to pull image %q: %w", imgDesc.Name, err)
				}
			}
		}
		if cfg.FetchArtifacts {
			p.UpdateTitle(fmt.Sprintf("Saving image %s/%s signature", imgDesc.Chart, imgDesc.Name))
			if err := artifacts.PullImageSignatures(context.Background(), imgDesc, artifactsDir); err != nil {
				if err == artifacts.ErrTagDoesNotExist {
					l.Debugf("image %q does not have an associated signature", imgDesc.Image)
				} else {
					return fmt.Errorf("failed to fetch image signatures: %w", err)
				}
			} else {
				l.Debugf("image %q signature fetched", imgDesc.Image)
			}
			p.UpdateTitle(fmt.Sprintf("Saving image %s/%s metadata", imgDesc.Chart, imgDesc.Name))
			if err := artifacts.PullImageMetadata(context.Background(), imgDesc, artifactsDir); err != nil {
				if err == artifacts.ErrTagDoesNotExist {
					l.Debugf("image %q does not have an associated metadata artifact", imgDesc.Image)
				} else {
					return fmt.Errorf("failed to fetch image metadata: %w", err)
				}
			} else {
				l.Debugf("image %q metadata fetched", imgDesc.Image)
			}
		}
	}
	return nil
}

// PushImages push the list of images in imagesDir to the destination specified in the ImagesLock
func PushImages(lock *imagelock.ImagesLock, imagesDir string, opts ...Option) error {
	cfg := NewConfiguration(opts...)
	l := cfg.Log

	ctx := cfg.Context

	artifactsDir := getArtifactsDir(filepath.Join(imagesDir, "artifacts"), cfg)

	p, _ := cfg.ProgressBar.WithTotal(len(lock.Images)).UpdateTitle("Pushing images").Start()
	defer p.Stop()

	craneOpts := make([]crane.Option, 0)
	craneOpts = append(craneOpts, crane.WithContext(ctx))
	if cfg.InsecureMode {
		craneOpts = append(craneOpts, crane.Insecure)
	}
	o := crane.GetOptions(craneOpts...)

	maxRetries := cfg.MaxRetries
	for _, imgData := range lock.Images {

		select {
		// Early abort if the context is done
		case <-ctx.Done():
			return fmt.Errorf("cancelled execution")
		default:
			p.Add(1)
			p.UpdateTitle(fmt.Sprintf("Pushing image %q", imgData.Image))
			err := utils.ExecuteWithRetry(maxRetries, func(try int, prevErr error) error {
				if try > 0 {
					// The context is done, so we are not retrying, just return the error
					if ctx.Err() != nil {
						return prevErr
					}
					l.Debugf("Failed to push image: %v", prevErr)
					p.Warnf("Failed to push image: retrying %d/%d", try, maxRetries)
				}
				if err := pushImage(imgData, imagesDir, o); err != nil {
					return err
				}
				if err := artifacts.PushImageSignatures(context.Background(),
					imgData,
					artifactsDir,
					artifacts.WithInsecureMode(cfg.InsecureMode)); err != nil {
					if err == artifacts.ErrLocalArtifactNotExist {
						l.Debugf("image %q does not have a local signature stored", imgData.Image)
					} else {
						return fmt.Errorf("failed to push image signatures: %w", err)
					}
				} else {
					p.UpdateTitle(fmt.Sprintf("Pushed image %q signature", imgData.Image))
				}

				if err := artifacts.PushImageMetadata(context.Background(),
					imgData,
					artifactsDir,
					artifacts.WithInsecureMode(cfg.InsecureMode)); err != nil {
					if err == artifacts.ErrLocalArtifactNotExist {
						l.Debugf("image %q does not have a local metadata artifact stored", imgData.Image)
					} else {
						return fmt.Errorf("failed to push image metadata: %w", err)
					}
				} else {
					p.UpdateTitle(fmt.Sprintf("Pushed image %q metadata", imgData.Image))
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed to push image %q: %w", imgData.Name, err)
			}
		}
	}
	return nil
}

func loadImage(path string) (v1.Image, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !stat.IsDir() {
		img, err := crane.Load(path)
		if err != nil {
			return nil, fmt.Errorf("could not load %q as tarball: %w", path, err)
		}
		return img, nil
	}

	l, err := layout.ImageIndexFromPath(path)
	if err != nil {
		return nil, fmt.Errorf("could load %q as OCI layout: %w", path, err)
	}
	m, err := l.IndexManifest()
	if err != nil {
		return nil, err
	}
	if len(m.Manifests) != 1 {
		return nil, fmt.Errorf("layout contains too many entries (%d)", len(m.Manifests))
	}
	desc := m.Manifests[0]
	if desc.MediaType.IsImage() {
		return l.Image(desc.Digest)
	}
	return nil, fmt.Errorf("layout contains non-image (mediaType: %q)", desc.MediaType)
}

func buildImageIndex(image *imagelock.ChartImage, imagesDir string) (v1.ImageIndex, error) {
	adds := make([]mutate.IndexAddendum, 0, len(image.Digests))

	base := mutate.IndexMediaType(empty.Index, types.DockerManifestList)
	for _, dgstData := range image.Digests {
		imgDir := getImageLayoutDir(imagesDir, dgstData)

		img, err := loadImage(imgDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load image %q: %w", imgDir, err)
		}
		newDesc, err := partial.Descriptor(img)
		if err != nil {
			return nil, fmt.Errorf("failed to create descriptor: %w", err)
		}
		cf, err := img.ConfigFile()
		if err != nil {
			return nil, fmt.Errorf("failed to obtain image config file: %w", err)
		}
		newDesc.Platform = cf.Platform()

		adds = append(adds, mutate.IndexAddendum{
			Add:        img,
			Descriptor: *newDesc,
		})
	}
	return mutate.AppendManifests(base, adds...), nil
}

func pushImage(imgData *imagelock.ChartImage, imagesDir string, o crane.Options) error {
	idx, err := buildImageIndex(imgData, imagesDir)
	if err != nil {
		return fmt.Errorf("failed to build image index: %w", err)
	}

	ref, err := name.ParseReference(imgData.Image, o.Name...)
	if err != nil {
		return fmt.Errorf("failed to parse image reference %q: %w", imgData.Image, err)
	}

	if err := remote.WriteIndex(ref, idx, o.Remote...); err != nil {
		return fmt.Errorf("failed to write image index: %w", err)
	}

	return nil
}

func getImageLayoutDir(imagesDir string, dgst imagelock.DigestInfo) string {
	return filepath.Join(imagesDir, fmt.Sprintf("%s.layout", dgst.Digest.Encoded()))
}

func pullImage(image string, digest imagelock.DigestInfo, imagesDir string, o crane.Options) (string, error) {
	imgDir := getImageLayoutDir(imagesDir, digest)

	src := fmt.Sprintf("%s@%s", image, digest.Digest)
	ref, err := name.ParseReference(src, o.Name...)
	if err != nil {
		return "", fmt.Errorf("parsing reference %q: %w", src, err)
	}
	rmt, err := remote.Get(ref, o.Remote...)
	if err != nil {
		return "", err
	}
	img, err := rmt.Image()
	if err != nil {
		return "", err
	}
	// We do not want to keep adding images to the index so we
	// start fresh
	if utils.FileExists(imgDir) {
		if err := os.RemoveAll(imgDir); err != nil {
			return "", fmt.Errorf("failed to remove existing image dir %q: %v", imgDir, err)
		}
	}
	if err := crane.SaveOCI(img, imgDir); err != nil {
		return "", fmt.Errorf("failed to save image %q to %q: %w", image, imgDir, err)
	}
	return imgDir, nil
}
