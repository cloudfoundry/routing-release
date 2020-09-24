# Scripts

This is the README for our scripts. To learn more about `routing-release`, go to the main [README](../README.md).

| Name | Purpose | Notes |
| --- | --- | --- |
| commit_with_shortlog | creates a commit with provided story number and a shortlog of all submodule changes | depends on staged_shortlog |
| light-commit-with-submodule-log | lightweight script for submodule bumps, allows for commits that don't finish a story | depends on submodule-log |
| push_with_recurse | pushes release and local submodule bumps whose commits not available on remote | |
| run-all-component-tests.sh | runs all the `bin/test` scripts for each component we maintain or will run a provided component test suite. This requires setup (like a database) for all tests to pass. The easiest way to get this setup is to run it inside of the docker container from "start-docker-for-testing". | allows for 3 retries on failed tests |
| run-unit-tests | sets up the test environment and runs all unit tests or the provided component test suite | depends on setup-test-environment and run-all-component-tests |
| run-unit-tests-in-docker | starts a docker container and runs all unit tests | depends on run-unit-tests |
| setup-git-hooks | installs git hooks from routing-release | |
| setup-test-environment.sh | installs test dependencies and runs template tests | |
| staged_shortlog | prints the submodule log titles for any staged submodule changes | |
| start-docker-for-testing | starts up a docker to run tests in manually | useful if you want to keep a docker alive rather than spinning one up each time |
| submodule-log | prints the cached submodule log and if you provide story id(s) will add finishes tag(s) | |
| sync-package-specs | updates the package spec file for component dependencies | |
| template-tests | runs the spec templating tests | |
| update | updates all submodules | |

