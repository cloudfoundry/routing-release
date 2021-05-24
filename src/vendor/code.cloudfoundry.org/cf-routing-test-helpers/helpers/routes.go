package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	. "github.com/onsi/gomega/gexec"

	"code.cloudfoundry.org/cf-routing-test-helpers/schema"
	"github.com/cloudfoundry-incubator/cf-test-helpers/cf"
)

const (
	deaUnsupportedTag = "{NO_DEA_SUPPORT} "
)

func MapRandomTcpRouteToApp(app, domain string, timeout time.Duration) {
	if isVersion7() {
		Expect(cf.Cf("map-route", app, domain).Wait(timeout)).To(Exit(0))
	} else {
		Expect(cf.Cf("map-route", app, domain, "--random-port").Wait(timeout)).To(Exit(0))
	}
}

func MapRouteToApp(app, domain, host, path string, timeout time.Duration) {
	Expect(cf.Cf("map-route", app, domain, "--hostname", host, "--path", path).Wait(timeout)).To(Exit(0))
}

func DeleteTcpRoute(domain, port string, timeout time.Duration) {
	Expect(cf.Cf("delete-route", domain,
		"--port", port,
		"-f",
	).Wait(timeout)).To(Exit(0))
}

func DeleteRoute(hostname, contextPath, domain string, timeout time.Duration) {
	Expect(cf.Cf("delete-route", domain,
		"--hostname", hostname,
		"--path", contextPath,
		"-f",
	).Wait(timeout)).To(Exit(0))
}

func CreateRoute(hostname, contextPath, space, domain string, timeout time.Duration) {
	Expect(cf.Cf("create-route", space, domain,
		"--hostname", hostname,
		"--path", contextPath,
	).Wait(timeout)).To(Exit(0))
}

func CreateTcpRouteWithRandomPort(space, domain string, timeout time.Duration) uint16 {
	CFColor := os.Getenv("CF_COLOR")

	err := os.Setenv("CF_COLOR", "false")
	Expect(err).NotTo(HaveOccurred())

	defer os.Setenv("CF_COLOR", CFColor)

	var responseBuffer *gexec.Session
	if isVersion7() {
		responseBuffer = cf.Cf("create-route", domain)
	} else {
		responseBuffer = cf.Cf("create-route", space, domain, "--random-port")
	}
	Expect(responseBuffer.Wait(timeout)).To(Exit(0))

	port, err := strconv.Atoi(grabPort(responseBuffer.Out.Contents(), domain))
	Expect(err).NotTo(HaveOccurred())
	return uint16(port)
}

func grabPort(response []byte, domain string) string {
	re := regexp.MustCompile("Route " + domain + ":([0-9]*) has been created")
	matches := re.FindStringSubmatch(string(response))

	Expect(len(matches)).To(Equal(2))
	//port
	return matches[1]
}

func VerifySharedDomain(domainName string, timeout time.Duration) {
	output := cf.Cf("domains")
	Expect(output.Wait(timeout)).To(Exit(0))

	Expect(string(output.Out.Contents())).To(ContainSubstring(domainName))
}

func getGuid(curlPath string, timeout time.Duration) string {
	err := os.Setenv("CF_TRACE", "false")
	Expect(err).NotTo(HaveOccurred())

	var response schema.ListResponse

	responseBuffer := cf.Cf("curl", curlPath)
	Expect(responseBuffer.Wait(timeout)).To(Exit(0))

	err = json.Unmarshal(responseBuffer.Out.Contents(), &response)
	Expect(err).NotTo(HaveOccurred())
	if response.TotalResults == 1 {
		return response.Resources[0].Metadata.Guid
	}
	return ""
}

func GetPortFromAppsInfo(appName, domainName string, timeout time.Duration) string {
	cfResponse := cf.Cf("apps").Wait(timeout).Out.Contents()
	re := regexp.MustCompile(appName + ".*" + domainName + ":([0-9]*)")
	matches := re.FindStringSubmatch(string(cfResponse))

	Expect(len(matches)).To(Equal(2))
	return matches[1]
}

func GetRouteGuidWithPort(hostname, path string, port uint16, timeout time.Duration) string {
	routeQuery := fmt.Sprintf("/v2/routes?q=host:%s&q=path:%s", hostname, path)
	if port > 0 {
		routeQuery = routeQuery + fmt.Sprintf("&q=port:%d", port)
	}
	routeGuid := getGuid(routeQuery, timeout)
	Expect(routeGuid).NotTo(Equal(""))
	return routeGuid
}

func GetRouteGuid(hostname, path string, timeout time.Duration) string {
	return GetRouteGuidWithPort(hostname, path, 0, timeout)
}

func GetAppInfo(appName string, timeout time.Duration) (host, port string) {
	os.Setenv("CF_TRACE", "false")
	var appsResponse schema.AppsResponse
	var statsResponse schema.StatsResponse

	cfResponse := cf.Cf("curl", fmt.Sprintf("/v2/apps?q=name:%s", appName)).Wait(timeout).Out.Contents()
	err := json.Unmarshal(cfResponse, &appsResponse)
	Expect(err).NotTo(HaveOccurred())
	serverAppUrl := appsResponse.Resources[0].Metadata.Url

	cfResponse = cf.Cf("curl", fmt.Sprintf("%s/stats", serverAppUrl)).Wait(timeout).Out.Contents()
	err = json.Unmarshal(cfResponse, &statsResponse)
	Expect(err).NotTo(HaveOccurred())

	appIp := statsResponse["0"].Stats.Host
	appPort := fmt.Sprintf("%d", statsResponse["0"].Stats.Port)
	return appIp, appPort
}

func UpdatePorts(appName string, ports []uint16, timeout time.Duration) {
	appGuid := GetAppGuid(appName, timeout)

	bodyMap := map[string][]uint16{
		"ports": ports,
	}

	data, err := json.Marshal(bodyMap)
	Expect(err).ToNot(HaveOccurred())

	Expect(cf.Cf("curl", fmt.Sprintf("/v2/apps/%s", appGuid), "-X", "PUT", "-d", string(data)).Wait(timeout)).To(Exit(0))
}

func CreateRouteMapping(appName string, hostname string, externalPort uint16, appPort uint16, timeout time.Duration) {
	appGuid := GetAppGuid(appName, timeout)
	routeGuid := GetRouteGuidWithPort(hostname, "", externalPort, timeout)

	bodyMap := map[string]interface{}{
		"app_guid":   appGuid,
		"route_guid": routeGuid,
		"app_port":   appPort,
	}

	data, err := json.Marshal(bodyMap)
	Expect(err).ToNot(HaveOccurred())

	Expect(cf.Cf("curl", fmt.Sprintf("/v2/route_mappings"), "-X", "POST", "-d", string(data)).Wait(timeout)).To(Exit(0))
}

func CreateSharedDomain(domainName, routerGroupName string, timeout time.Duration) {
	Expect(cf.Cf("create-shared-domain", domainName, "--router-group", routerGroupName).Wait(timeout)).To(Exit(0))
}

func DeleteSharedDomain(domainName string, timeout time.Duration) {
	Expect(cf.Cf("delete-shared-domain", domainName, "-f").Wait(timeout)).To(Exit(0))
}

func isVersion7() bool {
	// cf version 6.51.0+2acd15650.2020-04-07
	// cf7 version 7.0.2+17b4eeafd.2020-07-24
	bytes, err := exec.Command("cf", "version").CombinedOutput()
	Expect(err).ToNot(HaveOccurred())

	versionString := string(bytes)
	versionString = strings.Split(versionString, " ")[2]
	versionString = strings.Split(versionString, ".")[0]
	majorVersion, _ := strconv.Atoi(versionString)
	Expect(err).ToNot(HaveOccurred())

	return majorVersion == 7
}
