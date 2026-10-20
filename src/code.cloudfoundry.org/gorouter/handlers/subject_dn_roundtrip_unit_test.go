package handlers

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subject DN round trip (envoy XFCC)", func() {
	// Drive the real header producer so the test covers formatXFCCEnvoy and
	// not just formatSubjectDN.
	identityFor := func(name pkix.Name) (*CallerIdentity, error) {
		return extractIdentityFromEnvoyXFCC(
			formatXFCCEnvoy(&x509.Certificate{Raw: []byte("raw"), Subject: name}))
	}

	It("preserves a legitimate identity", func() {
		id, err := identityFor(pkix.Name{
			CommonName: "instance-id",
			OrganizationalUnit: []string{
				"app:app-guid", "space:space-guid", "organization:org-guid",
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(id).To(Equal(&CallerIdentity{AppGUID: "app-guid", SpaceGUID: "space-guid", OrgGUID: "org-guid"}))
	})

	DescribeTable("attacker-influenced fields cannot forge or override identity",
		func(name pkix.Name, expectedAppGUID string) {
			id, err := identityFor(name)
			if expectedAppGUID == "" {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(id.AppGUID).To(Equal(expectedAppGUID))
		},
		Entry("comma and quote in CN",
			pkix.Name{CommonName: `x,OU=app:victim"`}, ""),
		Entry("comma in CN with no real OU",
			pkix.Name{CommonName: "x,OU=app:victim"}, ""),
		Entry("comma in O appended after the real OUs",
			pkix.Name{CommonName: "i", OrganizationalUnit: []string{"app:real"}, Organization: []string{"acme,OU=app:victim"}}, "real"),
		Entry("trailing backslash in O",
			pkix.Name{CommonName: "i", OrganizationalUnit: []string{"app:real"}, Organization: []string{`a\`, "b"}}, "real"),
		Entry("slash in a CN-only subject",
			pkix.Name{CommonName: "x/OU=app:victim"}, ""),
		Entry("slash in an O-only subject",
			pkix.Name{Organization: []string{"y/OU=app:victim"}}, ""),
		Entry("slash in CN alongside real OUs",
			pkix.Name{CommonName: "x/OU=app:victim", OrganizationalUnit: []string{"app:real"}}, "real"),
	)

	DescribeTable("RFC 4514 special characters in an OU value survive the round trip",
		func(special string) {
			id, err := identityFor(pkix.Name{
				CommonName:         "i",
				OrganizationalUnit: []string{"app:a" + special + "b", "space:s"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(id.AppGUID).To(Equal("a" + special + "b"))
			Expect(id.SpaceGUID).To(Equal("s"))
		},
		Entry("comma", ","),
		Entry("plus", "+"),
		Entry("double quote", `"`),
		Entry("backslash", `\`),
		Entry("semicolon", ";"),
		Entry("less than", "<"),
		Entry("greater than", ">"),
		Entry("hash", "#"),
		Entry("equals", "="),
		Entry("space", " "),
	)

	DescribeTable("leading and trailing whitespace in values",
		func(name pkix.Name, expectedAppGUID string) {
			id, err := identityFor(name)
			Expect(err).NotTo(HaveOccurred())
			Expect(id.AppGUID).To(Equal(expectedAppGUID))
		},
		Entry("trailing space in the last OU",
			pkix.Name{CommonName: "i", OrganizationalUnit: []string{"app:guid "}}, "guid "),
		Entry("trailing space in a middle OU",
			pkix.Name{CommonName: "i", OrganizationalUnit: []string{"app:guid ", "space:s"}}, "guid "),
		Entry("several trailing spaces",
			pkix.Name{CommonName: "i", OrganizationalUnit: []string{"app:guid   "}}, "guid   "),
		Entry("trailing backslash then space",
			pkix.Name{CommonName: "i", OrganizationalUnit: []string{`app:guid\ `}}, `guid\ `),
		Entry("leading and trailing space in the CN",
			pkix.Name{CommonName: " i ", OrganizationalUnit: []string{"app:real"}}, "real"),
		Entry("leading hash in the CN",
			pkix.Name{CommonName: "#i", OrganizationalUnit: []string{"app:real"}}, "real"),
	)

	Describe("legacy slash-separated DNs", func() {
		parse := func(dn string) (*CallerIdentity, error) {
			return extractIdentityFromEnvoyXFCC(fmt.Sprintf(`Hash=abc123;Subject="%s"`, dn))
		}

		DescribeTable("are parsed, including with surrounding whitespace",
			func(dn string) {
				id, err := parse(dn)
				Expect(err).NotTo(HaveOccurred())
				Expect(id).To(Equal(&CallerIdentity{AppGUID: "legacy", SpaceGUID: "sp"}))
			},
			Entry("leading slash", "/CN=i/OU=app:legacy/OU=space:sp"),
			Entry("leading space", " /CN=i/OU=app:legacy/OU=space:sp"),
			Entry("leading tab", "\t/CN=i/OU=app:legacy/OU=space:sp"),
			Entry("trailing space", "/CN=i/OU=app:legacy/OU=space:sp "),
		)

		It("does not split on a slash unless the DN starts with one", func() {
			// formatSubjectDN does not escape "/", so a comma-style DN must
			// never be re-read as slash-separated or a CN could forge an OU.
			_, err := parse("CN=x/OU=app:victim")
			Expect(err).To(HaveOccurred())
		})
	})
})
