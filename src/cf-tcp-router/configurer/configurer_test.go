package configurer_test

import (
	"reflect"

	"code.cloudfoundry.org/routing-release/cf-tcp-router/configurer"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/configurer/haproxy"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Configurer", func() {

	const (
		startPort = 62000
	)
	Describe("NewConfigurer", func() {
		Context("when 'haproxy' tcp load balancer is passed", func() {
			It("should return haproxy configurer", func() {
				routeConfigurer := configurer.NewConfigurer(logger,
					configurer.HaProxyConfigurer, "haproxy/fixtures/haproxy.cfg.template", "haproxy/fixtures/haproxy.cfg", nil, nil)
				Expect(routeConfigurer).ShouldNot(BeNil())
				expectedType := reflect.PtrTo(reflect.TypeOf(haproxy.Configurer{}))
				value := reflect.ValueOf(routeConfigurer)
				Expect(value.Type()).To(Equal(expectedType))
			})

			Context("when invalid config file is passed", func() {
				It("should panic", func() {
					Expect(func() {
						configurer.NewConfigurer(logger, configurer.HaProxyConfigurer, "haproxy/fixtures/haproxy.cfg.template", "", nil, nil)
					}).Should(Panic())
				})
			})

			Context("when invalid base config file is passed", func() {
				It("should panic", func() {
					Expect(func() {
						configurer.NewConfigurer(logger, configurer.HaProxyConfigurer, "", "haproxy/fixtures/haproxy.cfg", nil, nil)
					}).Should(Panic())
				})
			})
		})

		Context("when non-supported tcp load balancer is passed", func() {
			It("should panic", func() {
				Expect(func() {
					configurer.NewConfigurer(logger, "not-supported", "some-base-config-file", "some-config-file", nil, nil)
				}).Should(Panic())
			})
		})

		Context("when empty tcp load balancer is passed", func() {
			It("should panic", func() {
				Expect(func() {
					configurer.NewConfigurer(logger, "", "some-base-config-file", "some-config-file", nil, nil)
				}).Should(Panic())
			})
		})

	})
})
