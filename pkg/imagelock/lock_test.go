package imagelock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	tu "github.com/vmware-labs/distribution-tooling-for-helm/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func createLockFromImageData(images map[string][]*tu.ImageData) *ImagesLock {
	lock := NewImagesLock()
	for chartName, imgs := range images {
		for _, img := range imgs {
			chartImage := &ChartImage{
				Chart:   chartName,
				Image:   img.Image,
				Name:    img.Name,
				Digests: make([]DigestInfo, 0),
			}
			for _, digestInfo := range img.Digests {
				chartImage.Digests = append(chartImage.Digests, DigestInfo{
					Arch:   digestInfo.Arch,
					Digest: digestInfo.Digest,
				})
			}
			lock.Images = append(lock.Images, chartImage)
		}
	}
	return lock
}

func initializeReferenceImages() ([]*tu.ImageData, error) {
	var referenceImages []*tu.ImageData

	fh, err := os.Open("../../testdata/images.json")
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	dec := json.NewDecoder(fh)
	if err := dec.Decode(&referenceImages); err != nil {
		return nil, fmt.Errorf("failed to decode reference images: %w", err)
	}
	return referenceImages, nil
}

func getImageLockImage(raw tu.ImageData, chart string) *ChartImage {
	img := &ChartImage{
		Name:    raw.Name,
		Chart:   chart,
		Digests: make([]DigestInfo, 0),
		Image:   raw.Image,
	}
	for _, d := range raw.Digests {
		img.Digests = append(img.Digests, DigestInfo{Arch: d.Arch, Digest: d.Digest})
	}
	return img
}

type ImageLockTestSuite struct {
	suite.Suite
	sb              *tu.Sandbox
	testServer      *tu.TestServer
	referenceImages []*tu.ImageData
}

func (suite *ImageLockTestSuite) findImageByName(imageName string) (*tu.ImageData, error) {
	for _, ref := range suite.referenceImages {
		if ref.Name == imageName {
			return ref, nil
		}
	}
	return nil, fmt.Errorf("cannot find reference image %q", imageName)
}

func (suite *ImageLockTestSuite) getCustomizedReferenceImages(chart string, names ...string) ([]*ChartImage, error) {
	imgs := make([]*ChartImage, 0)

	for _, id := range names {
		imgData, err := suite.findImageByName(id)
		if err != nil {
			return nil, err
		}

		img := getImageLockImage(*imgData, chart)
		img.Image = fmt.Sprintf("%s/%s", suite.testServer.ServerURL, imgData.Image)
		imgs = append(imgs, img)
	}
	return imgs, nil
}

func (suite *ImageLockTestSuite) TearDownSuite() {
	suite.testServer.Close()
	_ = suite.sb.Cleanup()
}

func (suite *ImageLockTestSuite) SetupSuite() {
	suite.sb = tu.NewSandbox()
	s, err := tu.NewTestServer()
	suite.Require().NoError(err)

	images, err := initializeReferenceImages()
	suite.Require().NoError(err)

	suite.referenceImages = images

	for _, img := range suite.referenceImages {
		suite.Require().NoError(s.AddImage(img))
	}

	suite.Require().Nil(err)
	suite.testServer = s
}

func (suite *ImageLockTestSuite) TestGenerateFromChart() {
	t := suite.T()
	sb := suite.sb
	require := suite.Require()
	assert := suite.Assert()

	chartName := "wordpress"
	chartVersion := "1.0.0"
	appVersion := "6.7.8"
	serverURL := suite.testServer.ServerURL

	sampleImages, err := suite.testServer.LoadImagesFromFile("../../testdata/images.json")
	suite.Require().NoError(err)

	t.Run("Loads from Helm chart", func(_ *testing.T) {

		scenarioName := "custom-chart"
		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

		referenceLock := NewImagesLock()

		referenceLock.Chart.Name = chartName
		referenceLock.Chart.Version = chartVersion
		referenceLock.Chart.AppVersion = appVersion

		imgs, err := suite.getCustomizedReferenceImages(referenceLock.Chart.Name,
			"wordpress", "bitnami-shell", "apache-exporter")
		require.NoError(err)

		referenceLock.Images = imgs

		chartRoot := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartRoot,
			map[string]interface{}{
				"ServerURL": serverURL, "Images": imgs, "Name": chartName, "Version": chartVersion, "AppVersion": appVersion,
			},
		))

		lock, err := GenerateFromChart(chartRoot, Insecure)
		assert.Nil(err, "failed to create Images.lock from Helm chart: %v", err)
		assert.NotNil(lock)
		// Not interested on this for the comparison
		lock.Metadata["generatedAt"] = ""
		referenceLock.Metadata["generatedAt"] = ""

		assert.Equal(referenceLock, lock)
	})
	t.Run("Loads Helm chart with dependencies", func(_ *testing.T) {
		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario("../../testdata/scenarios/chart1", chartDir,
			map[string]interface{}{"ServerURL": serverURL},
		))

		lock, err := GenerateFromChart(chartDir, Insecure)
		assert.NoError(err, "failed to create Images.lock from Helm chart: %v", err)
		require.NotNil(lock)
		// Not interested on this for the comparison
		lock.Metadata["generatedAt"] = ""

		existingLock, err := FromYAMLFile(filepath.Join(chartDir, "Images.lock"))
		assert.NoError(err)
		// Not interested on this for the comparison
		existingLock.Metadata["generatedAt"] = ""
		assert.Equal(existingLock, lock)
	})

	t.Run("Retrieves only the specified platforms", func(_ *testing.T) {
		scenarioName := "custom-chart"

		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{"ServerURL": serverURL, "Images": sampleImages, "Name": "test", "RepositoryURL": serverURL},
		))

		platforms := []string{"linux/amd64"}
		lock, err := GenerateFromChart(chartDir, Insecure, WithPlatforms(platforms))
		require.NoError(err, "failed to create Images.lock from Helm chart: %v", err)
		// Not interested on this for the comparison
		assert.Len(lock.Images, len(sampleImages))
		for _, img := range lock.Images {
			assert.Len(img.Digests, len(platforms))
			// To ensure this will fail if we ever change the list of architectures
			assert.Len(img.Digests, 1)
			assert.Equal(img.Digests[0].Arch, platforms[0])
		}
	})
	t.Run("Lock from single arch images", func(t *testing.T) {
		silentLog := log.New(io.Discard, "", 0)
		s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
		defer s.Close()

		u, err := url.Parse(s.URL)
		if err != nil {
			t.Fatal(err)
		}
		images := []*tu.ImageData{
			{
				Name:  "app1",
				Image: fmt.Sprintf("%s/bitnami/app1:latest", u.Host),
			},
		}
		for _, img := range images {
			craneImg, tErr := tu.CreateSingleArchImage(img, "linux/amd64")
			require.NoError(tErr)
			require.NoError(crane.Push(craneImg, img.Image, crane.Insecure))
		}
		scenarioName := "custom-chart"
		chartName := "test"     //nolint:govet
		chartVersion := "1.0.0" //nolint:govet
		appVersion := "2.2.0"   //nolint:govet
		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{"ServerURL": u.Host, "Images": images, "Name": chartName, "Version": chartVersion, "AppVersion": appVersion},
		))

		expectedLock := createLockFromImageData(map[string][]*tu.ImageData{
			chartName: images,
		})
		expectedLock.Chart.Name = chartName
		expectedLock.Chart.Version = chartVersion
		expectedLock.Chart.AppVersion = appVersion
		expectedLock.Metadata["generatedAt"] = ""

		lock, err := GenerateFromChart(chartDir, WithInsecure(true))
		require.NoError(err, "failed to create Images.lock from Helm chart: %v", err)

		// Not interested on this for the comparison
		lock.Metadata["generatedAt"] = ""

		assert.Equal(expectedLock, lock)
	})

	t.Run("Gracefully fails when loading images without platform", func(t *testing.T) {
		silentLog := log.New(io.Discard, "", 0)
		s := httptest.NewServer(registry.New(registry.Logger(silentLog)))
		defer s.Close()

		u, err := url.Parse(s.URL)
		if err != nil {
			t.Fatal(err)
		}

		craneImg, err := crane.Image(map[string][]byte{
			"platform.txt": []byte("undefined"),
		})
		require.NoError(err)

		image := fmt.Sprintf("%s/bitnami/app1:latest", u.Host)

		require.NoError(crane.Push(craneImg, image, crane.Insecure))

		scenarioName := "custom-chart"
		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartDir,
			map[string]interface{}{
				"ServerURL": u.Host,
				"Images": []*tu.ImageData{
					{
						Name:  "app1",
						Image: image,
					},
				},
				"Name":    "test",
				"Version": "1.0.0"},
		))

		_, err = GenerateFromChart(chartDir, Insecure)
		assert.ErrorContains(err, "failed to obtain image platform")
	})

	t.Run("Fails when no archs are retrieved", func(_ *testing.T) {
		chartDir := sb.TempFile()

		require.NoError(tu.RenderScenario("../../testdata/scenarios/chart1", chartDir,
			map[string]interface{}{"ServerURL": serverURL},
		))

		lock, err := GenerateFromChart(chartDir, Insecure, WithPlatforms([]string{"invalid"}))
		assert.ErrorContains(err, "got empty list of digests after applying platforms filter")
		require.Nil(lock)
	})

	t.Run("Fails when missing Helm chart dependencies", func(_ *testing.T) {
		type chartDependency struct {
			Name       string
			Repository string
			Version    string
		}

		scenarioName := "custom-chart"
		scenarioDir := fmt.Sprintf("../../testdata/scenarios/%s", scenarioName)

		chartRoot := sb.TempFile()

		require.NoError(tu.RenderScenario(scenarioDir, chartRoot, map[string]interface{}{"ServerURL": serverURL,
			"Dependencies": []chartDependency{{
				Name: "wordpress", Version: "1.0.0",
				Repository: "oci://registry-1.docker.io/bitnamicharts",
			}},
			"Name": chartName, "Version": chartVersion,
		}))

		_, err := GenerateFromChart(chartRoot, Insecure)
		assert.ErrorContains(err, "the Helm chart defines dependencies but they are not present in the charts directory")
	})

	t.Run("Fails to load from invalid directory", func(_ *testing.T) {
		chartRoot := sb.TempFile()
		require.NoFileExists(chartRoot)

		_, err := GenerateFromChart(chartRoot, Insecure)
		assert.ErrorContains(err, "no such file or directory")
	})
}

func TestImageLockTestSuite(t *testing.T) {
	suite.Run(t, new(ImageLockTestSuite))
}

func (suite *ImageLockTestSuite) TestFindImageByName() {
	t := suite.T()
	il := NewImagesLock()
	imgs, err := suite.getCustomizedReferenceImages("sample",
		"wordpress", "bitnami-shell", "apache-exporter")

	require.NoError(t, err)
	il.Images = imgs

	// All images are found
	t.Run("All images are found", func(t *testing.T) {
		for _, img := range imgs {
			foundImg, err := il.FindImageByName("sample", img.Name)
			assert.NoError(t, err)
			assert.Equal(t, img, foundImg)
		}
	})
	t.Run("Image is not found for different Helm chart", func(t *testing.T) {
		for _, img := range imgs {
			foundImg, err := il.FindImageByName("invalid_chart", img.Name)
			assert.Nil(t, foundImg)
			assert.ErrorContains(t, err, "cannot find image")
		}
	})

}
func (suite *ImageLockTestSuite) TestValidate() {
	t := suite.T()
	il := NewImagesLock()
	imgs, err := suite.getCustomizedReferenceImages("sample",
		"wordpress", "bitnami-shell", "apache-exporter")

	require.NoError(t, err)
	il.Images = imgs

	cloneImages := func(imgs []*ChartImage) []*ChartImage {
		newImgs := make([]*ChartImage, 0)
		for _, img := range imgs {
			// copy the struct
			newImg := *img
			newImg.Digests = make([]DigestInfo, len(img.Digests))
			copy(newImg.Digests, img.Digests)
			newImgs = append(newImgs, &newImg)
		}
		return newImgs
	}
	t.Run("Properly Validates Without Changes", func(t *testing.T) {
		newImgs := cloneImages(imgs)

		assert.NoError(t, il.Validate(newImgs))
	})
	t.Run("Fails to Validate when missing image", func(t *testing.T) {
		newImgs := cloneImages(imgs)
		newImgs = append(newImgs, &ChartImage{
			Chart: "dummy",
			Name:  "dummy_image",
		})
		assert.ErrorContains(t, il.Validate(newImgs), `chart "dummy": cannot find image "dummy_image"`)
	})
	t.Run("Fails to Validate when changed digest", func(t *testing.T) {
		newImgs := cloneImages(imgs)
		newImgs[0].Digests[0].Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
		assert.ErrorContains(t, il.Validate(newImgs), `digests do not match`)
	})
	t.Run("Fails to Validate when missing arch digest", func(t *testing.T) {
		newImgs := cloneImages(imgs)
		newImgs[0].Digests = append(newImgs[0].Digests, DigestInfo{Arch: "windows/arm64"})
		assert.ErrorContains(t, il.Validate(newImgs), `failed to find digest for arch "windows/arm64"`)
	})
}

func (suite *ImageLockTestSuite) TestYAML() {
	t := suite.T()
	il := NewImagesLock()
	il.Chart.Name = "test"
	il.Chart.Version = "1.0.0"
	il.Images = []*ChartImage{
		{
			Name:  "test",
			Chart: "test",
			Digests: []DigestInfo{
				{
					Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
					Arch:   "linux/amd64",
				},
			},
		},
	}

	expected := fmt.Sprintf(`apiVersion: v0
kind: ImagesLock
metadata:
  generatedAt: "%s"
  generatedBy: Distribution Tooling for Helm
chart:
  name: test
  version: 1.0.0
  appVersion: ""
images:
  - name: test
    image: ""
    chart: test
    digests:
      - digest: sha256:0000000000000000000000000000000000000000000000000000000000000000
        arch: linux/amd64
`, il.Metadata["generatedAt"])

	t.Run("ToYAML", func(t *testing.T) {
		t.Run("Serializes to YAML", func(t *testing.T) {
			buff := &bytes.Buffer{}
			err := il.ToYAML(buff)
			assert.NoError(t, err)
			assert.Equal(t, expected, buff.String())
		})
	})
	t.Run("FromYAML", func(t *testing.T) {
		t.Run("Deserializes from YAML", func(t *testing.T) {
			buff := bytes.NewBufferString(expected)
			newLock, err := FromYAML(buff)
			assert.NoError(t, err)
			assert.True(t, reflect.DeepEqual(newLock, il), "read lock does not match")
			assert.Equal(t, il, newLock)
		})
		t.Run("Fails on invalid YAML", func(t *testing.T) {
			buff := bytes.NewBufferString(`this is invalid`)
			_, err := FromYAML(buff)
			assert.ErrorContains(t, err, "failed to load")
		})
	})
	t.Run("FromYAMLFile", func(t *testing.T) {
		sb := suite.sb
		require := suite.Require()

		assert := suite.Assert()

		t.Run("Deserializes from YAML File", func(_ *testing.T) {
			f := sb.TempFile()
			require.NoError(os.WriteFile(f, []byte(expected), 0644))
			newLock, err := FromYAMLFile(f)
			assert.NoError(err)
			assert.Equal(il, newLock)
		})

		t.Run("Fails on invalid YAML file", func(_ *testing.T) {
			nonExisting := sb.TempFile()
			require.NoFileExists(nonExisting)
			_, err := FromYAMLFile(nonExisting)
			assert.ErrorContains(err, "no such file or directory")
		})
	})
}
