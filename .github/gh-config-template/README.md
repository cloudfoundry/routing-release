# Generate github actions from template

ytt -f ./gh_template.yml -f [ytt-helpers.star](https://github.com/cloudfoundry/wg-app-platform-runtime-ci/blob/main/shared/helpers/ytt-helpers.star) -f [index.yml](https://github.com/cloudfoundry/wg-app-platform-runtime-ci/blob/main/routing-release/index.yml) > ./workflows/tests-workflow.yml

## Supported jobs
- Template tests
- Basic Verifications
- Unit and Integration tests

### How to run

Request the repo owner to add a label as `ready-to-run` to validate PR.