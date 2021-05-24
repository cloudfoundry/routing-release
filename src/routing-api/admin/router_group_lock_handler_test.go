package admin_test

import (
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/routing-release/routing-api/admin"
	fake_db "code.cloudfoundry.org/routing-release/routing-api/db/fakes"
	"code.cloudfoundry.org/routing-release/routing-api/handlers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RouterGroupLockHandler", func() {
	var (
		routerGroupLockHandler *admin.RouterGroupLockHandler
		request                *http.Request
		responseRecorder       *httptest.ResponseRecorder
		database               *fake_db.FakeDB
		logger                 *lagertest.TestLogger
	)

	BeforeEach(func() {
		database = &fake_db.FakeDB{}
		logger = lagertest.NewTestLogger("routing-api-test")
		routerGroupLockHandler = admin.NewRouterGroupLockHandler(database, logger)
		responseRecorder = httptest.NewRecorder()
	})

	Describe("LockReads", func() {
		It("responds with a 200 OK and locks the ability to read router_groups", func() {
			request = handlers.NewTestRequest("")

			routerGroupLockHandler.LockReads(responseRecorder, request)
			Expect(responseRecorder.Code).To(Equal(http.StatusOK))
			Expect(logger.Logs()[0].Message).To(ContainSubstring("locking router group reads"))
			Expect(database.LockRouterGroupReadsCallCount()).To(Equal(1))
		})
	})

	Describe("LockWritess", func() {
		It("responds with a 200 OK and locks the ability to write router_groups", func() {
			request = handlers.NewTestRequest("")

			routerGroupLockHandler.LockWrites(responseRecorder, request)
			Expect(responseRecorder.Code).To(Equal(http.StatusOK))
			Expect(logger.Logs()[0].Message).To(ContainSubstring("locking router group writes"))
			Expect(database.LockRouterGroupWritesCallCount()).To(Equal(1))
		})
	})

	Describe("UnlockReads", func() {
		It("responds with a 200 OK and unlocks the ability to read router_groups", func() {
			request = handlers.NewTestRequest("")

			routerGroupLockHandler.UnlockReads(responseRecorder, request)
			Expect(responseRecorder.Code).To(Equal(http.StatusOK))
			Expect(logger.Logs()[0].Message).To(ContainSubstring("unlocking router group reads"))
			Expect(database.UnlockRouterGroupReadsCallCount()).To(Equal(1))
		})
	})

	Describe("UnlockWrites", func() {
		It("response with a 200 OK and locks the ability to write router_groups", func() {
			request = handlers.NewTestRequest("")

			routerGroupLockHandler.UnlockWrites(responseRecorder, request)
			Expect(responseRecorder.Code).To(Equal(http.StatusOK))
			Expect(logger.Logs()[0].Message).To(ContainSubstring("unlocking router group writes"))
			Expect(database.UnlockRouterGroupWritesCallCount()).To(Equal(1))
		})
	})
})
