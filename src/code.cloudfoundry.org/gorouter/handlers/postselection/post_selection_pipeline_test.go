package postselection_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/handlers/postselection"
	"code.cloudfoundry.org/gorouter/handlers/postselection/fakes"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("PostSelectionPipeline", func() {
	var (
		pipeline   *postselection.PostSelectionPipeline
		handler1   *fakes.FakePostSelectionHandler
		handler2   *fakes.FakePostSelectionHandler
		handler3   *fakes.FakePostSelectionHandler
		endpoint   *route.Endpoint
		reqInfo    *handlers.RequestInfo
		authError  *postselection.AuthError
		genericErr error
	)

	BeforeEach(func() {
		handler1 = &fakes.FakePostSelectionHandler{}
		handler2 = &fakes.FakePostSelectionHandler{}
		handler3 = &fakes.FakePostSelectionHandler{}

		endpoint = route.NewEndpoint(&route.EndpointOpts{
			AppId: "backend-app",
			Host:  "192.168.1.1",
			Port:  8080,
		})

		reqInfo = &handlers.RequestInfo{}

		authError = postselection.NewAuthError("test:rule", "test reason")
		genericErr = errors.New("generic error")
	})

	Describe("Run", func() {
		Context("with empty pipeline", func() {
			It("returns nil", func() {
				logger := test_util.NewTestLogger("pipeline")
				pipeline = postselection.NewPostSelectionPipeline(logger.Logger)
				err := pipeline.Run(endpoint, reqInfo)
				Expect(err).To(BeNil())
			})
		})

		Context("with single handler", func() {
			It("calls the handler and returns nil on success", func() {
				logger := test_util.NewTestLogger("pipeline")
				handler1.CheckReturns(nil)
				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1)

				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(BeNil())
				Expect(handler1.CheckCallCount()).To(Equal(1))
				ep, ri := handler1.CheckArgsForCall(0)
				Expect(ep).To(Equal(endpoint))
				Expect(ri).To(Equal(reqInfo))
			})

			It("returns error when handler fails", func() {
				logger := test_util.NewTestLogger("pipeline")
				handler1.CheckReturns(authError)
				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1)

				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(Equal(authError))
				Expect(handler1.CheckCallCount()).To(Equal(1))
			})
		})

		Context("with multiple handlers", func() {
			It("calls all handlers in order when all succeed", func() {
				logger := test_util.NewTestLogger("pipeline")
				handler1.CheckReturns(nil)
				handler2.CheckReturns(nil)
				handler3.CheckReturns(nil)
				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1, handler2, handler3)

				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(BeNil())
				Expect(handler1.CheckCallCount()).To(Equal(1))
				Expect(handler2.CheckCallCount()).To(Equal(1))
				Expect(handler3.CheckCallCount()).To(Equal(1))

				// Verify all received same endpoint and reqInfo
				ep1, ri1 := handler1.CheckArgsForCall(0)
				ep2, ri2 := handler2.CheckArgsForCall(0)
				ep3, ri3 := handler3.CheckArgsForCall(0)

				Expect(ep1).To(Equal(endpoint))
				Expect(ep2).To(Equal(endpoint))
				Expect(ep3).To(Equal(endpoint))
				Expect(ri1).To(Equal(reqInfo))
				Expect(ri2).To(Equal(reqInfo))
				Expect(ri3).To(Equal(reqInfo))
			})

			It("stops on first error and does not call remaining handlers", func() {
				logger := test_util.NewTestLogger("pipeline")
				handler1.CheckReturns(nil)
				handler2.CheckReturns(authError) // Fails here
				handler3.CheckReturns(nil)
				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1, handler2, handler3)

				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(Equal(authError))
				Expect(handler1.CheckCallCount()).To(Equal(1))
				Expect(handler2.CheckCallCount()).To(Equal(1))
				Expect(handler3.CheckCallCount()).To(Equal(0)) // Should not be called
			})

			It("stops on first handler error", func() {
				logger := test_util.NewTestLogger("pipeline")
				handler1.CheckReturns(authError) // Fails immediately
				handler2.CheckReturns(nil)
				handler3.CheckReturns(nil)
				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1, handler2, handler3)

				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(Equal(authError))
				Expect(handler1.CheckCallCount()).To(Equal(1))
				Expect(handler2.CheckCallCount()).To(Equal(0)) // Should not be called
				Expect(handler3.CheckCallCount()).To(Equal(0)) // Should not be called
			})

			It("stops on third handler error", func() {
				logger := test_util.NewTestLogger("pipeline")
				handler1.CheckReturns(nil)
				handler2.CheckReturns(nil)
				handler3.CheckReturns(authError) // Fails at the end
				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1, handler2, handler3)

				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(Equal(authError))
				Expect(handler1.CheckCallCount()).To(Equal(1))
				Expect(handler2.CheckCallCount()).To(Equal(1))
				Expect(handler3.CheckCallCount()).To(Equal(1))
			})
		})

		Context("error type handling", func() {
			It("returns AuthError as-is", func() {
				logger := test_util.NewTestLogger("pipeline")
				handler1.CheckReturns(authError)
				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1)

				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(Equal(authError))
				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("test:rule"))
				Expect(authErr.Reason).To(Equal("test reason"))
			})

			It("returns generic errors as-is", func() {
				logger := test_util.NewTestLogger("pipeline")
				handler1.CheckReturns(genericErr)
				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1)

				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(Equal(genericErr))
				Expect(err.Error()).To(Equal("generic error"))
			})
		})

		Context("handler state isolation", func() {
			It("does not interfere with reqInfo modifications by handlers", func() {
				logger := test_util.NewTestLogger("pipeline")
				// Handler 1 modifies reqInfo
				handler1.CheckStub = func(ep *route.Endpoint, ri *handlers.RequestInfo) error {
					if ri.AuthResult == nil {
						ri.AuthResult = &handlers.AuthResult{}
					}
					ri.AuthResult.Rule = "first-rule"
					return nil
				}

				// Handler 2 should see the modification
				handler2.CheckStub = func(ep *route.Endpoint, ri *handlers.RequestInfo) error {
					Expect(ri.AuthResult.Rule).To(Equal("first-rule"))
					ri.AuthResult.Rule = "second-rule"
					return nil
				}

				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1, handler2)
				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("second-rule"))
			})
		})

		Context("real-world scenario", func() {
			It("runs scope check then route policies check", func() {
				logger := test_util.NewTestLogger("pipeline")
				// Simulate scope check (passes)
				handler1.CheckStub = func(ep *route.Endpoint, ri *handlers.RequestInfo) error {
					// Scope check passed - no error
					return nil
				}

				// Simulate route policies check (passes and sets AuthResult.Rule)
				handler2.CheckStub = func(ep *route.Endpoint, ri *handlers.RequestInfo) error {
					// Route policies matched
					if ri.AuthResult == nil {
						ri.AuthResult = &handlers.AuthResult{}
					}
					ri.AuthResult.Rule = "route:cf:app:allowed-app"
					return nil
				}

				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1, handler2)
				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:app:allowed-app"))
			})

			It("returns error from scope check before running route policies", func() {
				logger := test_util.NewTestLogger("pipeline")
				scopeErr := postselection.NewAuthError(
					"domain:scope=org:post-selection",
					"caller org mismatch",
				)

				// Simulate scope check (fails)
				handler1.CheckReturns(scopeErr)

				// Simulate route policies check (should not be called)
				handler2.CheckStub = func(ep *route.Endpoint, ri *handlers.RequestInfo) error {
					Fail("route policies handler should not be called")
					return nil
				}

				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1, handler2)
				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(Equal(scopeErr))
				Expect(handler1.CheckCallCount()).To(Equal(1))
				Expect(handler2.CheckCallCount()).To(Equal(0))
			})

			It("returns error from route policies check when scope passes", func() {
				logger := test_util.NewTestLogger("pipeline")
				accessErr := postselection.NewAuthError(
					"route:route_policies",
					"caller not in route policies",
				)

				// Simulate scope check (passes)
				handler1.CheckReturns(nil)

				// Simulate route policies check (fails)
				handler2.CheckReturns(accessErr)

				pipeline = postselection.NewPostSelectionPipeline(logger.Logger, handler1, handler2)
				err := pipeline.Run(endpoint, reqInfo)

				Expect(err).To(Equal(accessErr))
				Expect(handler1.CheckCallCount()).To(Equal(1))
				Expect(handler2.CheckCallCount()).To(Equal(1))
			})
		})
	})
})
