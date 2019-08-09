package cmd

import (
	"bytes"
	"io/ioutil"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"github.com/Kong/konvoy/components/konvoy-control-plane/app/konvoyctl/pkg/install/data"
	"github.com/Kong/konvoy/components/konvoy-control-plane/app/konvoyctl/pkg/install/tls"
)

var _ = Describe("konvoyctl install control-plane", func() {

	var backupNewSelfSignedCert func(string) (tls.KeyPair, error)

	BeforeEach(func() {
		backupNewSelfSignedCert = newSelfSignedCert
	})
	AfterEach(func() {
		newSelfSignedCert = backupNewSelfSignedCert
	})

	BeforeEach(func() {
		newSelfSignedCert = func(string) (tls.KeyPair, error) {
			return tls.KeyPair{
				CertPEM: []byte("CERT"),
				KeyPEM:  []byte("KEY"),
			}, nil
		}
	})

	var stdout *bytes.Buffer
	var stderr *bytes.Buffer

	BeforeEach(func() {
		stdout = &bytes.Buffer{}
		stderr = &bytes.Buffer{}
	})

	type testCase struct {
		extraArgs  []string
		goldenFile string
	}

	DescribeTable("should generate Kubernetes resources",
		func(given testCase) {
			// given
			cmd := defaultRootCmd()
			cmd.SetArgs(append([]string{"install", "control-plane"}, given.extraArgs...))
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)

			// when
			err := cmd.Execute()
			// then
			Expect(err).ToNot(HaveOccurred())
			// and
			Expect(stderr.Bytes()).To(BeNil())

			// when
			expected, err := ioutil.ReadFile(filepath.Join("testdata", given.goldenFile))
			// then
			Expect(err).ToNot(HaveOccurred())
			// and
			expectedManifests := data.SplitYAML(expected)

			// when
			actual := stdout.Bytes()
			// then
			Expect(actual).To(MatchYAML(expected))
			// and
			actualManifests := data.SplitYAML(actual)

			// and
			Expect(len(actualManifests)).To(Equal(len(expectedManifests)))
			// and
			for i := range expectedManifests {
				Expect(actualManifests[i]).To(MatchYAML(expectedManifests[i]))
			}
		},
		Entry("should generate Kubernetes resources with default settings", testCase{
			extraArgs:  nil,
			goldenFile: "install-control-plane.defaults.golden.yaml",
		}),
		Entry("should generate Kubernetes resources with custom settings", testCase{
			extraArgs: []string{
				"--namespace", "konvoy",
				"--image-pull-policy", "Never",
				"--control-plane-version", "greatest",
				"--control-plane-image", "konvoy-ci/konvoy-control-plane",
				"--injector-image", "konvoy-ci/konvoy-injector",
				"--injector-failure-policy", "Crash",
				"--injector-service-name", "injector",
				"--injector-tls-cert", "AnotherCert",
				"--injector-tls-key", "AnotherKey",
				"--dataplane-image", "konvoy-ci/konvoy-dataplane",
				"--dataplane-init-image", "konvoy-ci/konvoy-init",
				"--dataplane-init-version", "dev",
			},
			goldenFile: "install-control-plane.overrides.golden.yaml",
		}),
	)
})
