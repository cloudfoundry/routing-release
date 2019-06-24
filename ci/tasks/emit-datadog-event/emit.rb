# Make sure you replace the API and/or APP key below
# with the ones for your account

require 'rubygems'
require 'dogapi'

if ARGV.size != 5
  puts "Usage: [DATADOG_API_KEY] [aws_env] [sha1] [metric_name] [value]"
  exit
end

api_key = ARGV[0]
aws_env = ARGV[1]
sha1 = ARGV[2]
metric_name = ARGV[3]
value = ARGV[4]

dog = Dogapi::Client.new(api_key)

tags = ["deployment:#{aws_env}", "sha:#{sha1}"]
dog.emit_point(metric_name, value.to_f, :tags => tags)
