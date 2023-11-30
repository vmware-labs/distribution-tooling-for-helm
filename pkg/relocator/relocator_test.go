package relocator

import (
	"log"
	"os"
	"testing"

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
