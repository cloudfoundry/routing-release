#!bin/bash

# tee_output_to_sys_log
#
# When syslog_utils.sh is loaded, this sends stdout and stderr to /var/vcap/sys/log.
function tee_output_to_sys_log() {
  declare log_dir="$1"
  declare log_name="$2"

  exec 1> >(tee -a >(log_prefix "O" >> "${log_dir}/${log_name}.log") | logger -p user.info -t "vcap.${log_name}.stdout")
  exec 2> >(tee -a >(log_prefix "E" >> "${log_dir}/${log_name}.log") | logger -p user.error -t "vcap.${log_name}.stderr")
}

function log_prefix() {
  perl -sne 'BEGIN { use Time::HiRes "time"; use POSIX "strftime"; STDOUT->autoflush(1) }; my $t = time; my $fsec = sprintf ".%06d", ($t-int($t))*1000000; my $time = strftime("%Y-%m-%dT%H:%M:%S".$fsec."%z", localtime $t); print("$time $prefix $_")' -- -prefix="${1:--}"
}
