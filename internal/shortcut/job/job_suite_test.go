package job

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestJobShortcut(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "internal/shortcut/job Suite")
}
