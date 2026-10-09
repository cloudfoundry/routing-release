package proxy

import (
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("escapePathAndPreserveSlashes", func() {
	DescribeTable("escapes each segment and preserves slashes",
		func(input, expected string) {
			Expect(escapePathAndPreserveSlashes(input)).To(Equal(expected))
		},
		Entry("empty path", "", ""),
		Entry("single slash", "/", "/"),
		Entry("double slash", "//", "//"),
		Entry("leading double slash", "//foo/bar", "//foo/bar"),
		Entry("trailing slash", "/foo/bar/", "/foo/bar/"),
		Entry("repeated slashes", "//a//b///c", "//a//b///c"),
		Entry("spaces", "//foo bar/baz qux", "//foo%20bar/baz%20qux"),
		Entry("characters PathEscape escapes", "//a?b/c#d/e%f", "//a%3Fb/c%23d/e%25f"),
		Entry("unicode", "//café/日本", "//caf%C3%A9/%E6%97%A5%E6%9C%AC"),
	)

	It("matches the previous concatenation-based implementation", func() {
		previous := func(unescaped string) string {
			escapedPath := ""
			for _, part := range strings.Split(unescaped, "/") {
				escapedPath = escapedPath + url.PathEscape(part) + "/"
			}
			return strings.TrimSuffix(escapedPath, "/")
		}

		for _, input := range []string{
			"", "/", "//", "///", "//x", "//x/", "/a b/", "//a/b c//d/", "//%2F/%", "//ü/;/=/@/:",
		} {
			Expect(escapePathAndPreserveSlashes(input)).To(Equal(previous(input)), "input: %q", input)
		}
	})

	It("handles a path with a very large number of segments in linear time", func() {
		// ~1MB, the largest request line the gorouter accepts by default.
		path := "//x" + strings.Repeat("/a", 500000)

		start := time.Now()
		escaped := escapePathAndPreserveSlashes(path)
		elapsed := time.Since(start)

		Expect(escaped).To(Equal(path))
		// The quadratic implementation took ~30s for this input.
		Expect(elapsed).To(BeNumerically("<", 2*time.Second))
	})
})
