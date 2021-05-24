package handlers_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/routing-release/routing-api/handlers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Middleware", func() {
	var (
		client       *http.Client
		ts           *httptest.Server
		dummyHandler http.HandlerFunc
		testSink     *lagertest.TestSink
	)

	BeforeEach(func() {

		// logger
		logger := lagertest.NewTestLogger("dummy-api")

		// dummy handler
		dummyHandler = func(w http.ResponseWriter, r *http.Request) {
			_, err := fmt.Fprintf(w, "Dummy handler")
			Expect(err).NotTo(HaveOccurred())
		}

		// wrap dummy handler in logwrap
		dummyHandler = handlers.LogWrap(dummyHandler, logger)

		// test server
		ts = httptest.NewServer(dummyHandler)

		client = &http.Client{}

		// test sink
		testSink = lagertest.NewTestSink()
		logger.RegisterSink(testSink)

	})

	AfterEach(func() {
		ts.Close()
	})

	It("doesn't output the authorization information", func() {
		req, err := http.NewRequest("GET", ts.URL, nil)
		req.Header.Add("Authorization", "this-is-a-secret")
		req.Header.Add("authorization", "this-is-a-secret2")
		req.Header.Add("AUTHORIZATION", "this-is-a-secret3")
		req.Header.Add("auThoRizaTion", "this-is-a-secret4")

		resp, err := client.Do(req)

		Expect(err).NotTo(HaveOccurred())

		output, err := ioutil.ReadAll(resp.Body)
		defer func() {
			err := resp.Body.Close()
			Expect(err).ToNot(HaveOccurred())
		}()
		Expect(err).NotTo(HaveOccurred())

		Expect(output).To(ContainSubstring("Dummy handler"))

		headers := testSink.Logs()[0].Data["request-headers"]
		Expect(headers).ToNot(HaveKey("Authorization"))
		Expect(headers).ToNot(HaveKey("authorization"))
		Expect(headers).ToNot(HaveKey("AUTHORIZATION"))
		Expect(headers).ToNot(HaveKey("auThoRizaTion"))
	})
})
