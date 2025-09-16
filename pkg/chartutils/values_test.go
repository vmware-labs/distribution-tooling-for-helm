package chartutils

import (
	"testing"
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
