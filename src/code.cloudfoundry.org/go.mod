module code.cloudfoundry.org

go 1.26.4

replace (
	code.cloudfoundry.org/locket => code.cloudfoundry.org/locket v0.0.0-20260602143356-23bea5865010
	github.com/cactus/go-statsd-client => github.com/cactus/go-statsd-client v2.0.2-0.20150911070441-6fa055a7b594+incompatible
)

require (
	code.cloudfoundry.org/cfhttp/v2 v2.80.0
	code.cloudfoundry.org/clock v1.73.0
	code.cloudfoundry.org/debugserver v0.100.0
	code.cloudfoundry.org/diego-logging-client v0.110.0
	code.cloudfoundry.org/eventhub v0.75.0
	code.cloudfoundry.org/go-loggregator/v9 v9.2.1
	code.cloudfoundry.org/go-metric-registry v0.0.0-20260602223408-eae7443fd7fa
	code.cloudfoundry.org/lager/v3 v3.72.0
	code.cloudfoundry.org/localip v0.74.0
	code.cloudfoundry.org/locket v0.0.0-20260602143356-23bea5865010
	code.cloudfoundry.org/tlsconfig v0.58.0
	github.com/armon/go-proxyproto v0.1.0
	github.com/cactus/go-statsd-client v3.2.1+incompatible
	github.com/cloudfoundry-community/go-uaa v0.4.0
	github.com/cloudfoundry/cf-test-helpers/v2 v2.13.0
	github.com/cloudfoundry/custom-cats-reporters v0.0.2
	github.com/cloudfoundry/dropsonde v1.1.0
	github.com/cloudfoundry/sonde-go v0.0.0-20260526083715-66f310f13c26
	github.com/go-sql-driver/mysql v1.10.0
	github.com/golang-jwt/jwt/v4 v4.5.2
	github.com/kisielk/errcheck v1.20.0
	github.com/nats-io/nats-server/v2 v2.14.2
	github.com/nats-io/nats.go v1.52.0
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/onsi/ginkgo/v2 v2.29.0
	github.com/onsi/gomega v1.41.0
	github.com/openzipkin/zipkin-go v0.4.3
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9
	github.com/tedsuo/ifrit v0.0.0-20260418191334-846868129986
	github.com/tedsuo/rata v1.0.0
	github.com/urfave/cli v1.22.17
	github.com/urfave/negroni/v3 v3.1.1
	github.com/vito/go-sse v1.1.3
	go.step.sm/crypto v0.82.0
	go.uber.org/zap v1.28.0
	go.uber.org/zap/exp v0.3.0
	golang.org/x/crypto v0.53.0
	golang.org/x/net v0.55.0
	golang.org/x/oauth2 v0.36.0
	golang.org/x/tools v0.45.0
	google.golang.org/grpc v1.81.1
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/driver/mysql v1.6.0
	gorm.io/driver/postgres v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	code.cloudfoundry.org/bbs v0.0.0-20260601194358-002fe939f0da // indirect
	code.cloudfoundry.org/durationjson v0.75.0 // indirect
	code.cloudfoundry.org/go-diodes v0.0.0-20260526122959-0284fcb5ac88 // indirect
	code.cloudfoundry.org/inigo v0.0.0-20210615140442-4bdc4f6e44d5 // indirect
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/antithesishq/antithesis-sdk-go v0.7.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/facebookgo/limitgroup v0.0.0-20150612190941-6abd8d71ec01 // indirect
	github.com/facebookgo/muster v0.0.0-20150708232844-fd3d7953fd52 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/go-test/deep v1.0.7 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/google/pprof v0.0.0-20260604005048-7023385849c0 // indirect
	github.com/honeycombio/libhoney-go v1.27.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/minio/highwayhash v1.0.4 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nats-io/jwt/v2 v2.8.2 // indirect
	github.com/nats-io/nkeys v0.4.16 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.68.1 // indirect
	github.com/prometheus/procfs v0.20.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/square/certstrap v1.3.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	gopkg.in/alexcesaro/statsd.v2 v2.0.0 // indirect
)
