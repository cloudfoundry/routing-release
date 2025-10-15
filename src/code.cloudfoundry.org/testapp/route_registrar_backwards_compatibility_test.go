package main_test

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func getURL(expectedResponse string) string {
	switch expectedResponse {
	case "encrypted":
		return encryptedURL
	case "unencrypted":
		return unencryptedURL
	case "tcp":
		return TCPURL
	default:
		panic("not valid")
	}

}

var _ = DescribeTable("Testing HTTP Endpoints",
	func(expectedResponse string) {
		url := getURL(expectedResponse)
		fmt.Println("URL:", url)
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Transport: tr}

		req, err := http.NewRequest("GET", url, nil)
		Expect(err).NotTo(HaveOccurred())

		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Proto).To(Equal("HTTP/1.1"))
		bodyBytes, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(bodyBytes)).To(ContainSubstring(expectedResponse))
	},
	Entry("HTTPS endpoint", "encrypted"),
	Entry("HTTP endpoint", "unencrypted"),
)

var _ = DescribeTable("Testing TCP Endpoints",
	func(expectedexpectedResponse string) {
		url := getURL(expectedexpectedResponse)
		tcpAddr, err := net.ResolveTCPAddr("tcp", url)
		Expect(err).NotTo(HaveOccurred())

		conn, err := net.DialTCP("tcp", nil, tcpAddr)
		Expect(err).NotTo(HaveOccurred())
		defer conn.Close()

		_, err = conn.Write([]byte("meow"))
		Expect(err).NotTo(HaveOccurred())

		reply := make([]byte, 1024)
		_, err = conn.Read(reply)
		Expect(err).NotTo(HaveOccurred())

		Expect(string(reply)).To(ContainSubstring(expectedexpectedResponse))

	},
	Entry("TCP", "tcp"),
)
