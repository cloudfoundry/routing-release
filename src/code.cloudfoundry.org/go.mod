module code.cloudfoundry.org

go 1.21.0

replace github.com/uber-go/zap => github.com/uber-go/zap v0.0.0-20161222040304-a5783ee4b216

replace github.com/uber-go/atomic => github.com/uber-go/atomic v1.1.0

replace github.com/cactus/go-statsd-client => github.com/cactus/go-statsd-client v2.0.2-0.20150911070441-6fa055a7b594+incompatible

require (
	code.cloudfoundry.org/bbs v0.0.0-20240717173038-096d0e1a7e95
	code.cloudfoundry.org/cfhttp/v2 v2.1.0
	code.cloudfoundry.org/clock v1.2.0
	code.cloudfoundry.org/debugserver v0.0.0-20240711175828-bd4bb5559e42
	code.cloudfoundry.org/eventhub v0.0.0-20240711175829-9b6874164e5c
	code.cloudfoundry.org/go-metric-registry v0.0.0-20240604201903-7cef498efb7a
	code.cloudfoundry.org/lager/v3 v3.0.3
	code.cloudfoundry.org/localip v0.0.0-20240711175824-a1a2d50a2164
	code.cloudfoundry.org/locket v0.0.0-20240521151413-b344fdd15d03
	code.cloudfoundry.org/tlsconfig v0.0.0-20240712175922-ffce9516cec8
	github.com/armon/go-proxyproto v0.1.0
	github.com/cactus/go-statsd-client v3.2.1+incompatible
	github.com/cloudfoundry-community/go-uaa v0.3.2
	github.com/cloudfoundry/cf-routing-test-helpers v0.0.0-20240304203209-3404f81a986b
	github.com/cloudfoundry/cf-test-helpers/v2 v2.9.0
	github.com/cloudfoundry/custom-cats-reporters v0.0.2
	github.com/cloudfoundry/dropsonde v1.1.0
	github.com/cloudfoundry/sonde-go v0.0.0-20240620221854-09ef53324489
	github.com/go-sql-driver/mysql v1.8.1
	github.com/golang-jwt/jwt/v4 v4.5.0
	github.com/jinzhu/gorm v1.9.16
	github.com/kisielk/errcheck v1.7.0
	github.com/lib/pq v1.10.9
	github.com/nats-io/nats-server/v2 v2.10.18
	github.com/nats-io/nats.go v1.36.0
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/onsi/ginkgo/v2 v2.19.0
	github.com/onsi/gomega v1.33.1
	github.com/openzipkin/zipkin-go v0.4.3
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/tedsuo/ifrit v0.0.0-20230516164442-7862c310ad26
	github.com/tedsuo/rata v1.0.0
	github.com/uber-go/zap v1.27.0
	github.com/urfave/cli v1.22.15
	github.com/urfave/negroni/v3 v3.1.1
	github.com/vito/go-sse v1.0.0
	go.step.sm/crypto v0.50.0
	golang.org/x/crypto v0.25.0
	golang.org/x/net v0.27.0
	golang.org/x/oauth2 v0.21.0
	golang.org/x/tools v0.23.0
	google.golang.org/grpc v1.65.0
	google.golang.org/protobuf v1.34.2
	gopkg.in/yaml.v2 v2.4.0
)

require (
	code.cloudfoundry.org/diego-logging-client v0.0.0-20240713175943-b577f5aaad4e // indirect
	code.cloudfoundry.org/durationjson v0.0.0-20240711175827-ec11f50a5c9e // indirect
	code.cloudfoundry.org/go-diodes v0.0.0-20240604201846-c756bfed2ed3 // indirect
	code.cloudfoundry.org/go-loggregator/v9 v9.2.1 // indirect
	code.cloudfoundry.org/inigo v0.0.0-20210615140442-4bdc4f6e44d5 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.4 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/facebookgo/limitgroup v0.0.0-20150612190941-6abd8d71ec01 // indirect
	github.com/facebookgo/muster v0.0.0-20150708232844-fd3d7953fd52 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/go-test/deep v1.0.7 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/pprof v0.0.0-20240711041743-f6c9dda6c6da // indirect
	github.com/honeycombio/libhoney-go v1.23.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.6.0 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nats-io/jwt/v2 v2.5.8 // indirect
	github.com/nats-io/nkeys v0.4.7 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.19.1 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/square/certstrap v1.3.0 // indirect
	github.com/uber-go/atomic v1.11.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.uber.org/atomic v1.8.0 // indirect
	go.uber.org/automaxprocs v1.5.3 // indirect
	golang.org/x/mod v0.19.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.22.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240711142825-46eb208f015d // indirect
	gopkg.in/alexcesaro/statsd.v2 v2.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
