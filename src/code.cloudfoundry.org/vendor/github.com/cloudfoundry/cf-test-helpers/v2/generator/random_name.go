package generator

import (
	"crypto/rand"
	"fmt"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
)

func randomName() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}

	return fmt.Sprintf("%x", b)
}

func PrefixedRandomName(prefixName, resourceName string) string {
	return prefixName + "-" + strconv.Itoa(GinkgoParallelProcess()) + "-" + resourceName + "-" + randomName()
}
