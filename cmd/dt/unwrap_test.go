package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/unwrap"
	"github.com/vmware-labs/distribution-tooling-for-helm/cmd/dt/wrap"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/artifacts"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/dtlog/logrus"
	"github.com/vmware-labs/distribution-tooling-for-helm/pkg/utils"

	"helm.sh/helm/v3/pkg/repo/repotest"
)

type unwrapOpts struct {
	FetchedArtifacts      bool
	PublicKey             string
	Images                []tu.ImageData
	ChartName             string
	Version               string
	ArtifactsMetadata     map[string][]byte
	UseAPI                bool
	Auth                  tu.Auth
	ContainerRegistryAuth tu.Auth
}

func testChartUnwrap(t *testing.T, sb *tu.Sandbox, inputChart string, targetRegistry string, chartTargetRegistry string, srcRegistry string, cfg unwrapOpts) {
	args := []string{"unwrap", "--log-level", "debug", "--plain", "--yes", "--use-plain-http", inputChart, targetRegistry}
	craneAuth := authn.Anonymous
	if cfg.ContainerRegistryAuth.Username != "" && cfg.ContainerRegistryAuth.Password != "" {
		craneAuth = &authn.Basic{Username: cfg.ContainerRegistryAuth.Username, Password: cfg.ContainerRegistryAuth.Password}
	}
	if chartTargetRegistry == "" {
		cfg.Auth = cfg.ContainerRegistryAuth
		chartTargetRegistry = targetRegistry
	} else {
		args = append(args, "--push-chart-url", chartTargetRegistry)
	}
	if cfg.UseAPI {
		l := logrus.NewSectionLogger()
		l.SetWriter(io.Discard)
		opts := []unwrap.Option{
			unwrap.WithLogger(l),
			unwrap.WithUsePlainHTTP(true),
			unwrap.WithSayYes(true),
			unwrap.WithAuth(cfg.Auth.Username, cfg.Auth.Password),
			unwrap.WithContainerRegistryAuth(cfg.ContainerRegistryAuth.Username, cfg.ContainerRegistryAuth.Password),
		}
		_, err := unwrap.Chart(inputChart, targetRegistry, chartTargetRegistry, opts...)
		require.NoError(t, err)
	} else {
		dt(args...).AssertSuccessMatch(t, "")
	}
	assert.True(t,
		artifacts.RemoteChartExist(
			fmt.Sprintf("oci://%s/%s", chartTargetRegistry, cfg.ChartName),
			cfg.Version,
			artifacts.WithRegistryAuth(cfg.Auth.Username, cfg.Auth.Password),
			artifacts.WithPlainHTTP(true),
		),
		"chart should exist in the repository",
	)

	normalizedSrcRegistry := srcRegistry
	if !strings.Contains(normalizedSrcRegistry, "://") {
		normalizedSrcRegistry = "http://" + normalizedSrcRegistry
	}
	u, err := url.Parse(normalizedSrcRegistry)
	require.NoError(t, err)

	path := u.Path

	relocatedRegistryPath := targetRegistry
	if path != "" && path != "/" {
		relocatedRegistryPath = fmt.Sprintf("%s/%s", relocatedRegistryPath, strings.Trim(filepath.Base(path), "/"))

	}

	// Verify the images were pushed
	for _, img := range cfg.Images {
		src := fmt.Sprintf("%s/%s", relocatedRegistryPath, img.Image)
		remoteDigests, err := tu.ReadRemoteImageManifest(src, tu.WithAuth(cfg.ContainerRegistryAuth.Username, cfg.ContainerRegistryAuth.Password))
		if err != nil {
			t.Fatal(err)
		}
		for _, dgstData := range img.Digests {
			assert.Equal(t, dgstData.Digest.Hex(), remoteDigests[dgstData.Arch].Digest.Hex())
		}

		tagsInfo := map[string]string{"main": "", "metadata": ""}
		tags, err := artifacts.ListTags(context.Background(), fmt.Sprintf("%s/%s", srcRegistry, "test"), crane.WithAuth(craneAuth))
		require.NoError(t, err)

		for _, tag := range tags {
			if tag == "latest" {
				tagsInfo["main"] = tag
			} else if strings.HasSuffix(tag, ".metadata") {
				tagsInfo["metadata"] = tag
			}
		}
		for _, k := range []string{"main", "metadata"} {
			v := tagsInfo[k]
			if v == "" {
				assert.Fail(t, "Tag %q should not be empty", k)
				continue
			}
			if cfg.PublicKey != "" {
				assert.NoError(t, tu.CosignVerifyImage(fmt.Sprintf("%s:%s", src, v), cfg.PublicKey, crane.WithAuth(craneAuth)), "Signature for %q failed", src)
			}
		}
		// Verify the metadata
		if cfg.FetchedArtifacts {
			ociMetadataDir, err := sb.Mkdir(sb.TempFile(), 0755)
			require.NoError(t, err)
			require.NoError(t, tu.PullArtifact(context.Background(), fmt.Sprintf("%s:%s", src, tagsInfo["metadata"]), ociMetadataDir, crane.WithAuth(craneAuth)))

			verifyArtifactsContents(t, sb, ociMetadataDir, cfg.ArtifactsMetadata)
		}
	}
}

func writeSampleImages(imageName string, imageTag string, dir string) ([]tu.ImageData, error) {
	_ = os.MkdirAll(dir, 0755)
	fullImageName := fmt.Sprintf("%s:%s", imageName, imageTag)
	imageData := tu.ImageData{Name: imageName, Image: fullImageName}

	craneImages, err := tu.CreateSampleImages(&imageData, []string{
		"linux/amd64",
		"linux/arm64",
	})

	if err != nil {
		return nil, err
	}

	for _, img := range craneImages {
		d, err := img.Digest()
		if err != nil {
			return nil, fmt.Errorf("failed to get image digest: %w", err)
		}

		imgDir := filepath.Join(dir, fmt.Sprintf("%s.layout", d.Hex))

		if err := crane.SaveOCI(img, imgDir); err != nil {
			return nil, fmt.Errorf("failed to save image %q to %q: %w", fullImageName, imgDir, err)
		}

	}
	return []tu.ImageData{imageData}, nil
}

func (suite *CmdSuite) TestUnwrapCommand() {
	t := suite.T()
	tests := []struct {
		name string
		auth bool
	}{
		{name: "WithoutAuth", auth: false},
		{name: "WithAuth", auth: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var username, password string
			var useAPI bool
			var registryURL string
			if tc.auth {
				useAPI = true

				srv, err := repotest.NewTempServerWithCleanup(t, "")
				if err != nil {
					t.Fatal(err)
				}
				defer srv.Stop()

				ociSrv, err := tu.NewOCIServer(t, srv.Root())
				if err != nil {
					t.Fatal(err)
				}
				go ociSrv.ListenAndServe()
				registryURL = ociSrv.RegistryURL

				username = "username"
				password = "password"
			} else {
				silentLog := log.New(io.Discard, "", 0)

				s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
				defer s.Close()
				u, err := url.Parse(s.URL)
				if err != nil {
					t.Fatal(err)
				}

				registryURL = u.Host
			}

			imageName := "test"
			imageTag := "mytag"

			sb := suite.sb
			serverURL := registryURL
			scenarioName := "complete-chart"
			chartName := "test"
			version := "1.0.0"
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
			currentRegIdx := 0

			newTargetRegistry := func(name string) string {
				return fmt.Sprintf("%s/%s", serverURL, name)
			}
			newUniqueTargetRegistry := func() string {
				currentRegIdx++
				return newTargetRegistry(fmt.Sprintf("new-images-%d", currentRegIdx))
			}
			t.Run("Unwrap Chart", func(t *testing.T) {
				require := suite.Require()
				assert := suite.Assert()

				wrapDir := sb.TempFile()

				chartDir := filepath.Join(wrapDir, "chart")

				images, err := writeSampleImages(imageName, imageTag, filepath.Join(wrapDir, "images"))
				require.NoError(err)

				require.NoError(tu.RenderScenario(scenarioDir, chartDir,
					map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version, "RepositoryURL": serverURL},
				))

				data, err := tu.RenderTemplateFile(filepath.Join(scenarioDir, "imagelock.partial.tmpl"),
					map[string]interface{}{"ServerURL": serverURL, "Images": images, "Name": chartName, "Version": version},
				)
				require.NoError(err)
				require.NoError(os.WriteFile(filepath.Join(chartDir, "Images.lock"), []byte(data), 0755))
				targetRegistry := newUniqueTargetRegistry()
				args := []string{"unwrap", wrapDir, targetRegistry, "--plain", "--yes", "--use-plain-http"}
				if useAPI {
					l := logrus.NewSectionLogger()
					l.SetWriter(io.Discard)
					opts := []unwrap.Option{
						unwrap.WithLogger(l),
						unwrap.WithUsePlainHTTP(true),
						unwrap.WithSayYes(true),
						unwrap.WithContainerRegistryAuth(username, password),
					}
					_, err := unwrap.Chart(wrapDir, targetRegistry, "", opts...)
					require.NoError(err)
				} else {
					dt(args...).AssertSuccessMatch(suite.T(), "")
				}
				// Verify the images were pushed
				for _, img := range images {
					src := fmt.Sprintf("%s/%s", targetRegistry, img.Image)
					remoteDigests, err := tu.ReadRemoteImageManifest(src, tu.WithAuth(username, password))
					if err != nil {
						t.Fatal(err)
					}
					for _, dgstData := range img.Digests {
						assert.Equal(dgstData.Digest.Hex(), remoteDigests[dgstData.Arch].Digest.Hex())
					}
				}
				assert.True(
					artifacts.RemoteChartExist(
						fmt.Sprintf("oci://%s/%s", targetRegistry, chartName),
						version,
						artifacts.WithRegistryAuth(username, password),
						artifacts.WithPlainHTTP(true),
					),
					"chart should exist in the repository",
				)
			})
		})
	}
}
func (suite *CmdSuite) TestEndToEnd() {
	t := suite.T()
	tests := []struct {
		name string
		auth bool
	}{
		{name: "WithoutAuth", auth: false},
		{name: "WithAuth", auth: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var contUser, contPass string
			var username, password string
			var useAPI bool
			var registryURL string
			var pushChartURL string
			if tc.auth {
				useAPI = true
				contUser = "username"
				contPass = "password"

				srv, err := repotest.NewTempServerWithCleanup(t, "")
				if err != nil {
					t.Fatal(err)
				}
				defer srv.Stop()

				srv2, err := repotest.NewTempServerWithCleanup(t, "")
				if err != nil {
					t.Fatal(err)
				}
				defer srv2.Stop()

				contSrv, err := tu.NewOCIServer(t, srv.Root())
				if err != nil {
					t.Fatal(err)
				}
				go contSrv.ListenAndServe()
				registryURL = contSrv.RegistryURL

				username = "username2"
				password = "password2"
				ociSrv, err := tu.NewOCIServerWithCustomCreds(t, srv2.Root(), username, password)
				if err != nil {
					t.Fatal(err)
				}
				go ociSrv.ListenAndServe()
				pushChartURL = ociSrv.RegistryURL
			} else {
				silentLog := log.New(io.Discard, "", 0)

				s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
				defer s.Close()
				u, err := url.Parse(s.URL)
				if err != nil {
					t.Fatal(err)
				}
				registryURL = u.Host
			}
			imageName := "test"

			sb := suite.sb
			serverURL := registryURL
			scenarioName := "complete-chart"
			chartName := "test"
			version := "1.0.0"
			scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)
			currentRegIdx := 0

			newTargetRegistry := func(name string) string {
				return fmt.Sprintf("%s/%s", serverURL, name)
			}
			newUniqueTargetRegistry := func() string {
				currentRegIdx++
				return newTargetRegistry(fmt.Sprintf("new-images-%d", currentRegIdx))
			}
			t.Run("Wrap and unwrap Chart", func(t *testing.T) {
				require := suite.Require()
				chartDir := sb.TempFile()

				srcRegistryNamespace := "wrap-unwrap-test"
				srcRegistry := newTargetRegistry(srcRegistryNamespace)
				targetRegistry := newUniqueTargetRegistry()

				certDir, err := sb.Mkdir(sb.TempFile(), 0755)
				require.NoError(err)

				keyFile, pubKey, err := tu.GenerateCosignCertificateFiles(certDir)
				require.NoError(err)

				metadataDir, err := sb.Mkdir(sb.TempFile(), 0755)
				require.NoError(err)

				metdataFileText := "this is a sample text"

				metadataArtifacts := map[string][]byte{
					"metadata.txt": []byte(metdataFileText),
				}
				for fileName, data := range metadataArtifacts {
					_, writeErr := sb.Write(filepath.Join(metadataDir, fileName), string(data))
					require.NoError(writeErr)
				}

				images, err := tu.AddSampleImagesToRegistry(imageName, srcRegistry, tu.WithSignKey(keyFile), tu.WithMetadataDir(metadataDir), tu.WithAuth(contUser, contPass))
				if err != nil {
					require.NoError(err)
				}

				require.NoError(tu.RenderScenario(scenarioDir, chartDir,
					map[string]interface{}{"ServerURL": srcRegistry, "Images": images, "Name": chartName, "Version": version, "RepositoryURL": srcRegistry},
				))

				tempFilename := fmt.Sprintf("%s/chart.wrap.tar.gz", sb.TempFile())

				testChartWrap(t, sb, chartDir, nil, wrapOpts{
					FetchArtifacts:        true,
					GenerateCarvelBundle:  false,
					ChartName:             chartName,
					Version:               version,
					OutputFile:            tempFilename,
					SkipExpectedLock:      true,
					Images:                images,
					ArtifactsMetadata:     metadataArtifacts,
					UseAPI:                useAPI,
					ContainerRegistryAuth: tu.Auth{Username: contUser, Password: contPass},
				})

				testChartUnwrap(t, sb, tempFilename, targetRegistry, pushChartURL, srcRegistry, unwrapOpts{
					FetchedArtifacts: true, Images: images, PublicKey: pubKey,
					ArtifactsMetadata:     metadataArtifacts,
					ChartName:             chartName,
					Version:               version,
					UseAPI:                useAPI,
					ContainerRegistryAuth: tu.Auth{Username: contUser, Password: contPass},
					Auth:                  tu.Auth{Username: username, Password: password},
				})
			})
		})
	}
}

// TestInnerManifestSignaturePreservedThroughWrapUnwrap verifies that a cosign signature
// created for an inner platform manifest (rather than the manifest list) is correctly
// preserved through wrap+unwrap with the per-architecture storage model:
//
//  1. Pull side (dt wrap): PullImageSignatures stores the inner-manifest sig at
//     <tag>.<arch>.sig/ (e.g. latest.linux_amd64.sig/) rather than the shared <tag>.sig/.
//
//  2. Push side (dt unwrap): PushImageSignatures pushes <tag>.<arch>.sig/ under the
//     matching sha256-<inner-digest>.sig tag on the target registry, so CRI-O/containerd
//     runtimes that resolve the manifest list and verify the selected platform's inner
//     manifest can locate and validate the correct signature.
func (suite *CmdSuite) TestInnerManifestSignaturePreservedThroughWrapUnwrap() {
	require := suite.Require()
	assert := suite.Assert()

	silentLog := log.New(io.Discard, "", 0)
	s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
	defer s.Close()
	u, err := url.Parse(s.URL)
	require.NoError(err)
	serverURL := u.Host

	sb := suite.sb
	scenarioDir := "../../testdata/scenarios/complete-chart"
	chartName := "test"
	version := "1.0.0"
	imageName := "test"

	srcNS := "inner-sig-src"
	dstNS := "inner-sig-dst"
	srcRegistry := fmt.Sprintf("%s/%s", serverURL, srcNS)
	targetRegistry := fmt.Sprintf("%s/%s", serverURL, dstNS)

	certDir, err := sb.Mkdir(sb.TempFile(), 0755)
	require.NoError(err)
	keyFile, pubKey, err := tu.GenerateCosignCertificateFiles(certDir)
	require.NoError(err)

	// Push the image WITHOUT manifest-list-level signing so that only the inner platform
	// manifest digest tag is present on the source registry after we sign below.
	images, err := tu.AddSampleImagesToRegistry(imageName, srcRegistry)
	require.NoError(err)
	require.NotEmpty(images)
	require.NotEmpty(images[0].Digests)

	// Locate the linux/amd64 inner platform manifest digest.
	var amd64Hex, amd64Full string
	for _, dgst := range images[0].Digests {
		if dgst.Arch == "linux/amd64" {
			amd64Hex = dgst.Digest.Hex()
			amd64Full = dgst.Digest.String()
			break
		}
	}
	require.NotEmpty(amd64Hex, "linux/amd64 digest must be present in the sample image set")

	// Sign the inner platform manifest by its digest — this is the standard cosign workflow
	// used by Kubernetes clusters with sigstoreSigned admission policy, where the runtime
	// resolves the manifest list and then looks for sha256-<inner-digest>.sig.
	innerSrcRef := fmt.Sprintf("%s/%s@%s", srcRegistry, imageName, amd64Full)
	require.NoError(tu.CosignImage(innerSrcRef, keyFile))

	// Set up the Helm chart referencing the source images.
	chartDir := sb.TempFile()
	require.NoError(tu.RenderScenario(scenarioDir, chartDir,
		map[string]interface{}{"ServerURL": srcRegistry, "Images": images, "Name": chartName, "Version": version, "RepositoryURL": srcRegistry},
	))

	// Wrap with --fetch-artifacts. PullImageSignatures should detect sha256-<amd64-hex>.sig
	// on the source and save it to latest.linux_amd64.sig/ inside the bundle.
	tempWrapFile := sb.TempFile() + ".wrap.tgz"
	l1 := logrus.NewSectionLogger()
	l1.SetWriter(io.Discard)
	_, err = wrap.Chart(chartDir,
		wrap.WithLogger(l1),
		wrap.WithUsePlainHTTP(true),
		wrap.WithFetchArtifacts(true),
		wrap.WithOutputFile(tempWrapFile),
	)
	require.NoError(err)

	// Extract the bundle and confirm the per-arch sig OCI layout was captured.
	// With the new per-arch storage model the sig is stored at <tag>.<arch>.sig/ rather than
	// the shared <tag>.sig/ (which is reserved for the manifest-list level signature).
	wrapExtractDir := sb.TempFile()
	require.NoError(utils.Untar(tempWrapFile, wrapExtractDir, utils.TarConfig{StripComponents: 1}))
	sigArtifactDir := filepath.Join(wrapExtractDir, "artifacts", "images", chartName, imageName, "latest.linux_amd64.sig")
	require.DirExists(sigArtifactDir, "per-arch sig artifact must be present in the wrap bundle at latest.linux_amd64.sig/")

	// Unwrap to the target registry. PushImageSignatures should push the per-arch sig stored
	// at latest.linux_amd64.sig/ under sha256-<amd64-inner-digest>.sig on the target.
	l2 := logrus.NewSectionLogger()
	l2.SetWriter(io.Discard)
	_, err = unwrap.Chart(tempWrapFile, targetRegistry, "",
		unwrap.WithLogger(l2),
		unwrap.WithUsePlainHTTP(true),
		unwrap.WithSayYes(true),
	)
	require.NoError(err)

	// With PreserveRepository=true (default) the source namespace is appended to the target.
	targetImageRepo := fmt.Sprintf("%s/%s/%s/%s", serverURL, dstNS, srcNS, imageName)

	tags, err := artifacts.ListTags(context.Background(), targetImageRepo, crane.Insecure)
	require.NoError(err)

	expectedSigTag := fmt.Sprintf("sha256-%s.sig", amd64Hex)
	assert.Contains(tags, expectedSigTag,
		"sig tag for the linux/amd64 inner manifest digest must exist on the target registry after unwrap")

	// Confirm cosign can validate the signature on the target using the inner digest reference.
	innerTargetRef := fmt.Sprintf("%s/%s/%s/%s@%s", serverURL, dstNS, srcNS, imageName, amd64Full)
	assert.NoError(tu.CosignVerifyImage(innerTargetRef, pubKey),
		"cosign signature validation must succeed for the inner manifest on the target registry")
}
