function docker_run_routing_release_tests() {
  local db="${1?Provide DB flavor. e.g e.g mysql-8.0(or mysql),mysql-5.7,postgres}"
  local this_file_dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
  "$this_file_dir/container.bash" "$db" -d
  docker exec routing-release-docker-container '/repo/scripts/docker/build-binaries.bash'
  docker exec routing-release-docker-container '/repo/scripts/docker/test.bash'
}
