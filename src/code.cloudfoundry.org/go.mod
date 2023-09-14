module code.cloudfoundry.org

go 1.20

replace github.com/uber-go/zap => github.com/uber-go/zap v0.0.0-20161222040304-a5783ee4b216

replace github.com/uber-go/atomic => github.com/uber-go/atomic v1.1.0

replace github.com/codegangsta/cli => github.com/codegangsta/cli v1.6.0

replace github.com/cactus/go-statsd-client => github.com/cactus/go-statsd-client v2.0.2-0.20150911070441-6fa055a7b594+incompatible

require (
	code.cloudfoundry.org/bbs v0.0.0-20230808235806-d58365d71e0d
	code.cloudfoundry.org/cfhttp/v2 v2.0.1-0.20210513172332-4c5ee488a657
	code.cloudfoundry.org/clock v1.1.0
	code.cloudfoundry.org/debugserver v0.0.0-20230612151301-d597b20f28ce
	code.cloudfoundry.org/eventhub v0.0.0-20230612151808-5a2f17c5e7b7
	code.cloudfoundry.org/go-metric-registry v0.0.0-20230906204350-82f6303ad953
	code.cloudfoundry.org/lager/v3 v3.0.2
	code.cloudfoundry.org/localip v0.0.0-20230612151424-f52ecafaffc4
	code.cloudfoundry.org/locket v0.0.0-20230612151453-08e003863044
	code.cloudfoundry.org/tlsconfig v0.0.0-20230612153104-23c0622de227
	github.com/armon/go-proxyproto v0.0.0-20210323213023-7e956b284f0a
	github.com/cactus/go-statsd-client v3.2.1+incompatible
	github.com/cloudfoundry-community/go-uaa v0.3.2
	github.com/cloudfoundry/cf-routing-test-helpers v0.0.0-20230612154734-4f65ecb98d93
	github.com/cloudfoundry/cf-test-helpers/v2 v2.7.0
	github.com/cloudfoundry/custom-cats-reporters v0.0.2
	github.com/cloudfoundry/dropsonde v1.1.0
	github.com/cloudfoundry/sonde-go v0.0.0-20230911203642-fa89d986ae20
	github.com/codegangsta/cli v1.22.14
	github.com/go-sql-driver/mysql v1.7.1
	github.com/golang-jwt/jwt/v4 v4.5.0
	github.com/jinzhu/gorm v1.9.16
	github.com/kisielk/errcheck v1.6.3
	github.com/lib/pq v1.10.9
	github.com/mailru/easyjson v0.7.7
	github.com/nats-io/nats-server/v2 v2.9.22
	github.com/nats-io/nats.go v1.29.0
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/onsi/ginkgo/v2 v2.12.0
	github.com/onsi/gomega v1.27.10
	github.com/openzipkin/zipkin-go v0.4.2
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/tedsuo/ifrit v0.0.0-20230516164442-7862c310ad26
	github.com/tedsuo/rata v1.0.0
	github.com/uber-go/zap v1.26.0
	github.com/urfave/negroni v1.0.0
	github.com/vito/go-sse v1.0.0
	golang.org/x/crypto v0.13.0
	golang.org/x/net v0.15.0
	golang.org/x/oauth2 v0.12.0
	golang.org/x/tools v0.13.0
	google.golang.org/grpc v1.58.1
	google.golang.org/protobuf v1.31.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	code.cloudfoundry.org/diego-logging-client v0.0.0-20230823164527-31a09b08e0af // indirect
	code.cloudfoundry.org/durationjson v0.0.0-20230612152544-cdd056aaf83d // indirect
	code.cloudfoundry.org/go-diodes v0.0.0-20230911203732-c04be32f825e // indirect
	code.cloudfoundry.org/go-loggregator/v8 v8.0.5 // indirect
	code.cloudfoundry.org/inigo v0.0.0-20210615140442-4bdc4f6e44d5 // indirect
	filippo.io/edwards25519 v1.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cockroachdb/apd v1.1.0 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/facebookgo/limitgroup v0.0.0-20150612190941-6abd8d71ec01 // indirect
	github.com/facebookgo/muster v0.0.0-20150708232844-fd3d7953fd52 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/go-test/deep v1.0.7 // indirect
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/pprof v0.0.0-20230912144702-c363fe2c2ed8 // indirect
	github.com/honeycombio/libhoney-go v1.20.0 // indirect
	github.com/jackc/fake v0.0.0-20150926172116-812a484cc733 // indirect
	github.com/jackc/pgx v3.6.2+incompatible // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/compress v1.16.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/nats-io/jwt/v2 v2.5.0 // indirect
	github.com/nats-io/nkeys v0.4.4 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.16.0 // indirect
	github.com/prometheus/client_model v0.4.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/square/certstrap v1.3.0 // indirect
	github.com/uber-go/atomic v1.11.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.3.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.step.sm/crypto v0.35.0 // indirect
	go.uber.org/atomic v1.8.0 // indirect
	go.uber.org/automaxprocs v1.5.3 // indirect
	golang.org/x/mod v0.12.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230913181813-007df8e322eb // indirect
	gopkg.in/alexcesaro/statsd.v2 v2.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
