package secrets_test

import (
	"context"
	"time"

	"github.com/kumahq/kuma/pkg/test/resources/builders"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	mesh_proto "github.com/kumahq/kuma/api/mesh/v1alpha1"
	"github.com/kumahq/kuma/pkg/core"
	core_ca "github.com/kumahq/kuma/pkg/core/ca"
	core_mesh "github.com/kumahq/kuma/pkg/core/resources/apis/mesh"
	core_model "github.com/kumahq/kuma/pkg/core/resources/model"
	"github.com/kumahq/kuma/pkg/core/secrets/cipher"
	secrets_manager "github.com/kumahq/kuma/pkg/core/secrets/manager"
	secrets_store "github.com/kumahq/kuma/pkg/core/secrets/store"
	core_metrics "github.com/kumahq/kuma/pkg/metrics"
	ca_builtin "github.com/kumahq/kuma/pkg/plugins/ca/builtin"
	"github.com/kumahq/kuma/pkg/plugins/resources/memory"
	test_metrics "github.com/kumahq/kuma/pkg/test/metrics"
	"github.com/kumahq/kuma/pkg/test/resources/model"
	. "github.com/kumahq/kuma/pkg/xds/secrets"
)

var _ = Describe("Secrets", func() {

	var secrets Secrets
	var metrics core_metrics.Metrics
	var now time.Time

	newMesh := func() *core_mesh.MeshResource {
		return &core_mesh.MeshResource{
			Meta: &model.ResourceMeta{
				Name: "default",
			},
			Spec: &mesh_proto.Mesh{
				Mtls: &mesh_proto.Mesh_Mtls{
					EnabledBackend: "ca-1",
					Backends: []*mesh_proto.CertificateAuthorityBackend{
						{
							Name: "ca-1",
							Type: "builtin",
							DpCert: &mesh_proto.CertificateAuthorityBackend_DpCert{
								Rotation: &mesh_proto.CertificateAuthorityBackend_DpCert_Rotation{
									Expiration: "1h",
								},
							},
						},
						{
							Name: "ca-2",
							Type: "builtin",
							DpCert: &mesh_proto.CertificateAuthorityBackend_DpCert{
								Rotation: &mesh_proto.CertificateAuthorityBackend_DpCert_Rotation{
									Expiration: "1h",
								},
							},
						},
					},
				},
			},
		}
	}

	BeforeEach(func() {
		resStore := memory.NewStore()
		secretManager := secrets_manager.NewSecretManager(secrets_store.NewSecretStore(resStore), cipher.None(), nil)
		builtinCaManager := ca_builtin.NewBuiltinCaManager(secretManager)
		caManagers := core_ca.Managers{
			"builtin": builtinCaManager,
		}
		err := builtinCaManager.EnsureBackends(context.Background(), "default", newMesh().Spec.Mtls.Backends)
		Expect(err).ToNot(HaveOccurred())

		caProvider := NewCaProvider(caManagers)
		identityProvider := NewIdentityProvider(caManagers)

		m, err := core_metrics.NewMetrics("local")
		Expect(err).ToNot(HaveOccurred())
		metrics = m

		secrets, err = NewSecrets(caProvider, identityProvider, metrics)
		Expect(err).ToNot(HaveOccurred())

		now = time.Now()
		core.Now = func() time.Time {
			return now
		}
	})

	It("should generate cert and emit statistic and info", func() {
		// given
		dp := builders.Dataplane().Build()

		// when
		identity, ca, err := secrets.Get(dp, newMesh())

		// then certs are generated
		Expect(err).ToNot(HaveOccurred())
		Expect(identity.PemCerts).ToNot(BeEmpty())
		Expect(identity.PemKey).ToNot(BeEmpty())
		Expect(ca.PemCerts).ToNot(BeEmpty())

		// and info is stored
		info := secrets.Info(core_model.MetaToResourceKey(dp.Meta))
		Expect(info.Generation).To(Equal(now))
		Expect(info.Expiration.Unix()).To(Equal(now.Add(1 * time.Hour).Unix()))
		Expect(info.MTLS.EnabledBackend).To(Equal("ca-1"))
		Expect(info.Tags).To(Equal(mesh_proto.MultiValueTagSet{
			"kuma.io/service": map[string]bool{
				dp.Spec.GetIdentifyingService(): true,
			},
		}))

		// and metric is published
		Expect(test_metrics.FindMetric(metrics, "cert_generation").GetCounter().GetValue()).To(Equal(1.0))
	})

	It("should not regenerate certs if nothing has changed", func() {
		// given
		dp := builders.Dataplane().Build()
		identity, ca, err := secrets.Get(dp, newMesh())
		Expect(err).ToNot(HaveOccurred())

		// when
		newIdentity, newCa, err := secrets.Get(dp, newMesh())

		// then
		Expect(err).ToNot(HaveOccurred())
		Expect(identity).To(Equal(newIdentity))
		Expect(ca).To(Equal(newCa))
		Expect(test_metrics.FindMetric(metrics, "cert_generation").GetCounter().GetValue()).To(Equal(1.0))
	})

	Context("should regenerate certificate", func() {
		BeforeEach(func() {
			_, _, err := secrets.Get(builders.Dataplane().Build(), newMesh())
			Expect(err).ToNot(HaveOccurred())
		})

		It("when mTLS settings has changed", func() {
			// given
			mesh := newMesh()
			mesh.Spec.Mtls.EnabledBackend = "ca-2"

			// when
			_, _, err := secrets.Get(builders.Dataplane().Build(), mesh)

			// then
			Expect(err).ToNot(HaveOccurred())
			Expect(test_metrics.FindMetric(metrics, "cert_generation").GetCounter().GetValue()).To(Equal(2.0))
		})

		It("when dp tags has changed", func() {
			// given
			dataplane := builders.Dataplane().
				WithServices("web2").
				Build()

			// when
			_, _, err := secrets.Get(dataplane, newMesh())

			// then
			Expect(err).ToNot(HaveOccurred())
			Expect(test_metrics.FindMetric(metrics, "cert_generation").GetCounter().GetValue()).To(Equal(2.0))
		})

		It("when cert is expiring", func() {
			// given
			now = now.Add(48*time.Minute + 1*time.Millisecond) // 4/5 of 60 minutes

			// when
			_, _, err := secrets.Get(builders.Dataplane().Build(), newMesh())

			// then
			Expect(err).ToNot(HaveOccurred())
			Expect(test_metrics.FindMetric(metrics, "cert_generation").GetCounter().GetValue()).To(Equal(2.0))
		})

		It("when cert was cleaned up", func() {
			// given
			dp := builders.Dataplane().Build()
			secrets.Cleanup(core_model.MetaToResourceKey(dp.Meta))

			// when
			_, _, err := secrets.Get(dp, newMesh())

			// then
			Expect(err).ToNot(HaveOccurred())
			Expect(test_metrics.FindMetric(metrics, "cert_generation").GetCounter().GetValue()).To(Equal(2.0))
		})
	})

	It("should cleanup certs", func() {
		// given
		dp := builders.Dataplane().Build()
		_, _, err := secrets.Get(dp, newMesh())
		Expect(err).ToNot(HaveOccurred())

		// when
		secrets.Cleanup(core_model.MetaToResourceKey(dp.Meta))

		// then
		Expect(secrets.Info(core_model.MetaToResourceKey(dp.Meta))).To(BeNil())
	})
})
