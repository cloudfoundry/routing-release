module code.cloudfoundry.org/routing-release

go 1.16

require (
	code.cloudfoundry.org/cf-routing-test-helpers v0.0.0-20200827173955-6ac4653025b4
	code.cloudfoundry.org/cfhttp/v2 v2.0.0
	code.cloudfoundry.org/clock v1.0.0
	code.cloudfoundry.org/consuladapter v0.0.0-20210518201129-f961dd99a7b2 // indirect
	code.cloudfoundry.org/debugserver v0.0.0-20210513170648-513d45197033
	code.cloudfoundry.org/diego-logging-client v0.0.0-20210518201212-41c9e880aaea // indirect
	code.cloudfoundry.org/durationjson v0.0.0-20200131001738-04c274cd71ed // indirect
	code.cloudfoundry.org/eventhub v0.0.0-20200131001618-613224898d70
	code.cloudfoundry.org/go-diodes v0.0.0-20190809170250-f77fb823c7ee // indirect
	code.cloudfoundry.org/go-loggregator v7.4.0+incompatible // indirect
	code.cloudfoundry.org/lager v2.0.0+incompatible
	code.cloudfoundry.org/localip v0.0.0-20210513163154-20d795cea8ec
	code.cloudfoundry.org/locket v0.0.0-20210519145606-91fc7012746e
	code.cloudfoundry.org/multierror v0.0.0-20170123201326-dafed03eebc6
	code.cloudfoundry.org/rfc5424 v0.0.0-20201103192249-000122071b78 // indirect
	code.cloudfoundry.org/tlsconfig v0.0.0-20200131000646-bbe0f8da39b3
	code.cloudfoundry.org/trace-logger v0.0.0-20170119230301-107ef08a939d
	code.cloudfoundry.org/uaa-go-client v0.0.0-20200427231439-19a7eb57a1dc
	github.com/apex/log v1.9.0
	github.com/apoydence/eachers v0.0.0-20181020210610-23942921fe77 // indirect
	github.com/armon/go-proxyproto v0.0.0-20210323213023-7e956b284f0a
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cactus/go-statsd-client/v5 v5.0.0
	github.com/cloudfoundry-incubator/cf-test-helpers v1.0.0
	github.com/cloudfoundry/custom-cats-reporters v0.0.2
	github.com/cloudfoundry/dropsonde v1.0.1-0.20180504154030-a5c24343b09d
	github.com/cloudfoundry/sonde-go v0.0.0-20200416163440-a42463ba266b
	github.com/go-kit/kit v0.10.0
	github.com/go-sql-driver/mysql v1.6.0
	github.com/gogo/protobuf v1.3.2
	github.com/hashicorp/consul/api v1.8.1 // indirect
	github.com/honeycombio/libhoney-go v1.15.2
	github.com/jinzhu/gorm v1.9.16
	github.com/lib/pq v1.10.2
	github.com/mailru/easyjson v0.7.7
	github.com/nats-io/nats-server/v2 v2.2.5 // indirect
	github.com/nats-io/nats.go v1.11.0
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/onsi/ginkgo v1.16.2
	github.com/onsi/gomega v1.12.0
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.6.1
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
	github.com/tedsuo/rata v1.0.0
	github.com/uber-common/bark v1.3.0
	github.com/urfave/cli v1.22.5
	github.com/urfave/negroni v1.0.0
	github.com/vito/go-sse v1.0.0
	golang.org/x/crypto v0.0.0-20210513164829-c07d793c2f9a
	golang.org/x/net v0.0.0-20210521195947-fe42d452be8f
	google.golang.org/grpc v1.38.0
	gopkg.in/inconshreveable/log15.v2 v2.0.0-20200109203555-b30bc20e4fd1
	gopkg.in/yaml.v2 v2.4.0
)
