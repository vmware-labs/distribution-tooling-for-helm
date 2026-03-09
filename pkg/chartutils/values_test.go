package chartutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chartutil"
)

func TestValuesImageElement_Relocate(t *testing.T) {
	tests := []struct {
		name             string
		elem             *ValuesImageElement
		prefix           string
		expectedErr      bool
		expectedRegistry string
		expectedRepo     string
	}{
		{
			name: "relocate with registry field with default project",
			elem: &ValuesImageElement{
				Registry:    "docker.io",
				Repository:  "nginx",
				Tag:         "latest",
				foundFields: []string{"registry", "repository", "tag"},
			},
			prefix:           "registry.example.com/myrepo",
			expectedErr:      false,
			expectedRegistry: "registry.example.com",
			expectedRepo:     "myrepo/library/nginx",
		},
		{
			name: "relocate with registry field with non default project",
			elem: &ValuesImageElement{
				Registry:    "docker.io",
				Repository:  "redpandadata/redpanda",
				Tag:         "latest",
				foundFields: []string{"registry", "repository", "tag"},
			},
			prefix:           "007439368137.dkr.ecr.us-east-2.amazonaws.com/kafka",
			expectedErr:      false,
			expectedRegistry: "007439368137.dkr.ecr.us-east-2.amazonaws.com",
			expectedRepo:     "kafka/redpandadata/redpanda",
		},
		{
			name: "relocate without registry field",
			elem: &ValuesImageElement{
				Repository:  "quay.io/cert-manager-controller",
				Tag:         "latest",
				foundFields: []string{"repository", "tag"},
			},
			prefix:           "007439368137.dkr.ecr.us-east-2.amazonaws.com",
			expectedErr:      false,
			expectedRegistry: "",
			expectedRepo:     "007439368137.dkr.ecr.us-east-2.amazonaws.com/cert-manager-controller",
		},
		{
			name: "relocate without registry field and non default project",
			elem: &ValuesImageElement{
				Repository:  "quay.io/jetstack/cert-manager-controller",
				Tag:         "latest",
				foundFields: []string{"repository", "tag"},
			},
			prefix:           "007439368137.dkr.ecr.us-east-2.amazonaws.com",
			expectedErr:      false,
			expectedRegistry: "",
			expectedRepo:     "007439368137.dkr.ecr.us-east-2.amazonaws.com/jetstack/cert-manager-controller",
		},
		{
			name: "relocate without registry field with default project",
			elem: &ValuesImageElement{
				Repository:  "nginx",
				Tag:         "latest",
				foundFields: []string{"repository", "tag"},
			},
			prefix:           "localhost:5000/myrepo",
			expectedErr:      false,
			expectedRegistry: "localhost:5000",
			expectedRepo:     "myrepo/library/nginx",
		},
		{
			name: "relocate without registry field with non default project",
			elem: &ValuesImageElement{
				Repository:  "redpandadata/redpanda",
				Tag:         "latest",
				foundFields: []string{"repository", "tag"},
			},
			prefix:           "localhost:5000/kafka",
			expectedErr:      false,
			expectedRegistry: "localhost:5000",
			expectedRepo:     "kafka/redpandadata/redpanda",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.elem.Relocate(tt.prefix)

			if tt.expectedErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if !tt.expectedErr {
				if tt.elem.Registry != tt.expectedRegistry {
					t.Errorf("Registry = %v, want %v", tt.elem.Registry, tt.expectedRegistry)
				}
				if tt.elem.Repository != tt.expectedRepo {
					t.Errorf("Repository = %v, want %v", tt.elem.Repository, tt.expectedRepo)
				}
			}
		})
	}
}

func TestFindImageElementsInValuesMap_SkipsNonImageRepositories(t *testing.T) {
	tests := []struct {
		name          string
		valuesYAML    string
		expectedCount int
		expectedURLs  []string
	}{
		{
			name: "skips git clone repository URL",
			valuesYAML: `
appFromExternalRepo:
  clone:
    repository: https://github.com/dotnet/AspNetCore.Docs.git
`,
			expectedCount: 0,
		},
		{
			name: "skips http helm chart repository URL (lowercase path, would pass name.ParseReference alone)",
			valuesYAML: `
helmRepo:
  repository: http://charts.example.com/my-chart
`,
			expectedCount: 0,
		},
		{
			name: "skips https helm chart repository URL (lowercase path, would pass name.ParseReference alone)",
			valuesYAML: `
helmRepo:
  repository: https://charts.example.com/my-chart
`,
			expectedCount: 0,
		},
		{
			name: "skips git+ssh repository URL",
			valuesYAML: `
source:
  repository: git://github.com/org/repo.git
`,
			expectedCount: 0,
		},
		{
			name: "detects valid docker image with repository only",
			valuesYAML: `
image:
  repository: bitnami/nginx
  tag: "1.25"
`,
			expectedCount: 1,
			expectedURLs:  []string{"bitnami/nginx:1.25"},
		},
		{
			name: "detects valid docker image with registry and repository",
			valuesYAML: `
image:
  registry: docker.io
  repository: bitnami/nginx
  tag: "1.25"
`,
			expectedCount: 1,
			expectedURLs:  []string{"docker.io/bitnami/nginx:1.25"},
		},
		{
			name: "skips git URL but still detects sibling valid image",
			valuesYAML: `
appFromExternalRepo:
  clone:
    repository: https://github.com/dotnet/AspNetCore.Docs.git
image:
  repository: bitnami/aspnet-core
  tag: "9.2.1"
`,
			expectedCount: 1,
			expectedURLs:  []string{"bitnami/aspnet-core:9.2.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valuesMap, err := chartutil.ReadValues([]byte(tt.valuesYAML))
			require.NoError(t, err)

			elems, err := FindImageElementsInValuesMap(valuesMap)
			require.NoError(t, err)
			assert.Len(t, elems, tt.expectedCount)

			for i, expectedURL := range tt.expectedURLs {
				if i < len(elems) {
					assert.Equal(t, expectedURL, elems[i].URL())
				}
			}
		})
	}
}
