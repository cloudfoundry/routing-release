.PHONY: test
#: run the tests!
test:
ifeq (, $(shell which gotestsum))
	@echo " ***"
	@echo "Running with standard go test because gotestsum was not found on PATH. Consider installing gotestsum for friendlier test output!"
	@echo " ***"
	go test -race -v ./...
else
	gotestsum --junitfile unit-tests.xml --format testname -- -race ./...
endif
