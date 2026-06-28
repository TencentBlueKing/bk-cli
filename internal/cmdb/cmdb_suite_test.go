package cmdb

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCMDB(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "internal/cmdb Suite")
}
