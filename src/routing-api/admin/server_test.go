package admin_test

import (
	"fmt"
	"net/http"
	"os"

	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/routing-release/routing-api/admin"
	fake_db "code.cloudfoundry.org/routing-release/routing-api/db/fakes"
	"code.cloudfoundry.org/routing-release/routing-api/test_helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/sigmon"
)

var _ = Describe("AdminServer", func() {
	var (
		logger  *lagertest.TestLogger
		db      *fake_db.FakeDB
		port    int
		process ifrit.Process
	)
	BeforeEach(func() {
		db = new(fake_db.FakeDB)
		logger = lagertest.NewTestLogger("routing-api-test")
		port = test_helpers.NextAvailPort()
		server, err := admin.NewServer(port, db, logger)
		Expect(err).ToNot(HaveOccurred())
		process = ifrit.Invoke(sigmon.New(server))
		Eventually(process.Ready(), "5s").Should(BeClosed())
	})

	AfterEach(func() {
		process.Signal(os.Interrupt)
		Eventually(process.Wait()).Should(Receive())
	})

	DescribeTable("testing the admin endpoint",
		func(endpoint string, callCountFn func(db *fake_db.FakeDB) int) {
			req, _ := http.NewRequest("PUT", fmt.Sprintf("http://127.0.0.1:%d/%s", port, endpoint), nil)
			resp, err := http.DefaultClient.Do(req)
			Expect(err).ToNot(HaveOccurred())
			Expect(resp).ToNot(BeNil())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(callCountFn(db)).To(Equal(1))
		},
		Entry(`"lock_router_group_reads"`, "lock_router_group_reads", func(db *fake_db.FakeDB) int {
			return db.LockRouterGroupReadsCallCount()
		}),
		Entry(`"unlock_router_group_reads"`, "unlock_router_group_reads", func(db *fake_db.FakeDB) int {
			return db.UnlockRouterGroupReadsCallCount()
		}),
		Entry(`"lock_router_group_writes"`, "lock_router_group_writes", func(db *fake_db.FakeDB) int {
			return db.LockRouterGroupWritesCallCount()
		}),
		Entry(`"unlock_router_group_writes"`, "unlock_router_group_writes", func(db *fake_db.FakeDB) int {
			return db.UnlockRouterGroupWritesCallCount()
		}),
	)
})
