package varz_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestVarz(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Varz Suite")
}
