package handlers

import (
	"crypto/x509/pkix"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subject DN round trip (envoy XFCC)", func() {
	identityFor := func(name pkix.Name) (*CallerIdentity, error) {
		return extractIdentityFromEnvoyXFCC(
			fmt.Sprintf(`Hash=abc123;Subject="%s"`, formatSubjectDN(name)))
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
})
