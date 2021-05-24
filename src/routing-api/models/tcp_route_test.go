package models_test

import (
	. "code.cloudfoundry.org/routing-release/routing-api/models"
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("TCP Route", func() {
	Describe("TcpMappingEntity", func() {
		Describe("SNI Hostname", func() {
			It("Is nillable", func() {
				tcpRouteMapping := NewSniTcpRouteMapping("a-guid", 1234, nil, "hostIp", 5678, 5)
				Expect(tcpRouteMapping.SniHostname).To(BeNil())
			})
			It("Accepts a value", func() {
				sniHostname := "sniHostname"
				tcpRouteMapping := NewSniTcpRouteMapping("a-guid", 1234, &sniHostname, "hostIp", 5678, 5)
				Expect(*tcpRouteMapping.SniHostname).To(Equal("sniHostname"))
			})
			It("Matches if values are the same", func() {
				sniHostname := "sniHostname"
				tcpRouteMapping1 := NewSniTcpRouteMapping("a-guid", 1234, &sniHostname, "hostIp", 5678, 5)
				tcpRouteMapping2 := NewSniTcpRouteMapping("a-guid", 1234, &sniHostname, "hostIp", 5678, 5)
				Expect(tcpRouteMapping1.Matches(tcpRouteMapping2)).To(BeTrue())
			})
			It("Matches if values are equal", func() {
				sniHostname1 := "sniHostname"
				tcpRouteMapping1 := NewSniTcpRouteMapping("a-guid", 1234, &sniHostname1, "hostIp", 5678, 5)
				sniHostname2 := "sniHostname"
				tcpRouteMapping2 := NewSniTcpRouteMapping("a-guid", 1234, &sniHostname2, "hostIp", 5678, 5)
				Expect(tcpRouteMapping1.Matches(tcpRouteMapping2)).To(BeTrue())
			})
			It("Doesn't match if value is different", func() {
				sniHostname1 := "sniHostname1"
				tcpRouteMapping1 := NewSniTcpRouteMapping("a-guid", 1234, &sniHostname1, "hostIp", 5678, 5)
				sniHostname2 := "sniHostname2"
				tcpRouteMapping2 := NewSniTcpRouteMapping("a-guid", 1234, &sniHostname2, "hostIp", 5678, 5)
				Expect(tcpRouteMapping1.Matches(tcpRouteMapping2)).To(BeFalse())
			})
			It("Matches if one is nil", func() {
				tcpRouteMapping1 := NewSniTcpRouteMapping("a-guid", 1234, nil, "hostIp", 5678, 5)
				sniHostname2 := "sniHostname2"
				tcpRouteMapping2 := NewSniTcpRouteMapping("a-guid", 1234, &sniHostname2, "hostIp", 5678, 5)
				Expect(tcpRouteMapping1.Matches(tcpRouteMapping2)).To(BeFalse())
			})
			It("JSON omits when nil", func() {
				tcpRouteMapping := NewSniTcpRouteMapping("a-guid", 1234, nil, "hostIp", 5678, 5)
				j, err := json.Marshal(tcpRouteMapping)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(j)).NotTo(ContainSubstring("backend_sni_hostname"))
			})
			It("JSON contains when empty", func() {
				sniHostname := ""
				tcpRouteMapping := NewSniTcpRouteMapping("a-guid", 1234, &sniHostname, "hostIp", 5678, 5)
				j, err := json.Marshal(tcpRouteMapping)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(j)).To(ContainSubstring("backend_sni_hostname"))
			})
			It("JSON contains when not nil", func() {
				sniHostname := "sniHostname"
				tcpRouteMapping := NewSniTcpRouteMapping("a-guid", 1234, &sniHostname, "hostIp", 5678, 5)
				j, err := json.Marshal(tcpRouteMapping)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(j)).To(ContainSubstring("backend_sni_hostname"))
			})
		})
	})
})
