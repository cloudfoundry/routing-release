module code.cloudfoundry.org

go 1.21.0

replace github.com/uber-go/zap => github.com/uber-go/zap v0.0.0-20161222040304-a5783ee4b216

replace github.com/uber-go/atomic => github.com/uber-go/atomic v1.1.0

replace github.com/codegangsta/cli => github.com/codegangsta/cli v1.6.0

replace github.com/cactus/go-statsd-client => github.com/cactus/go-statsd-client v2.0.2-0.20150911070441-6fa055a7b594+incompatible

require (
	code.cloudfoundry.org/bbs v0.0.0-20231204223625-b4a8e8d97ad9
	code.cloudfoundry.org/cfhttp/v2 v2.0.1-0.20210513172332-4c5ee488a657
	code.cloudfoundry.org/clock v1.1.0
	code.cloudfoundry.org/debugserver v0.0.0-20231206170715-fd2d2aa39d90
	code.cloudfoundry.org/eventhub v0.0.0-20231206170708-7e26ec40032d
	code.cloudfoundry.org/go-metric-registry v0.0.0-20231211170043-fcb9057b0d09
	code.cloudfoundry.org/lager/v3 v3.0.3
	code.cloudfoundry.org/localip v0.0.0-20231206170541-8d74d8ef21ec
	code.cloudfoundry.org/locket v0.0.0-20231114154242-23339499a62f
	code.cloudfoundry.org/tlsconfig v0.0.0-20231017135636-f0e44068c22f
	github.com/armon/go-proxyproto v0.1.0
	github.com/cactus/go-statsd-client v3.2.1+incompatible
	github.com/cloudfoundry-community/go-uaa v0.3.2
	github.com/cloudfoundry/cf-routing-test-helpers v0.0.0-20230612154734-4f65ecb98d93
	github.com/cloudfoundry/cf-test-helpers/v2 v2.8.0
	github.com/cloudfoundry/custom-cats-reporters v0.0.2
	github.com/cloudfoundry/dropsonde v1.1.0
	github.com/cloudfoundry/sonde-go v0.0.0-20231211170018-44d78969daf7
	github.com/codegangsta/cli v1.22.14
	github.com/go-sql-driver/mysql v1.7.1
	github.com/golang-jwt/jwt/v4 v4.5.0
	github.com/jinzhu/gorm v1.9.16
	github.com/kisielk/errcheck v1.6.3
	github.com/lib/pq v1.10.9
	github.com/nats-io/nats-server/v2 v2.10.7
	github.com/nats-io/nats.go v1.31.0
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/onsi/ginkgo/v2 v2.13.2
	github.com/onsi/gomega v1.30.0
	github.com/openzipkin/zipkin-go v0.4.2
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/tedsuo/ifrit v0.0.0-20230516164442-7862c310ad26
	github.com/tedsuo/rata v1.0.0
	github.com/uber-go/zap v1.26.0
	github.com/urfave/negroni v1.0.0
	github.com/vito/go-sse v1.0.0
	go.step.sm/crypto v0.40.0
	golang.org/x/crypto v0.17.0
	golang.org/x/net v0.19.0
	golang.org/x/oauth2 v0.15.0
	golang.org/x/tools v0.16.1
	google.golang.org/grpc v1.60.0
	google.golang.org/protobuf v1.31.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	code.cloudfoundry.org/diego-logging-client v0.0.0-20231206170752-1baebe76b38e // indirect
	code.cloudfoundry.org/durationjson v0.0.0-20231206170547-9040c1b5941e // indirect
	code.cloudfoundry.org/go-diodes v0.0.0-20231218170342-258647f3c6ec // indirect
	code.cloudfoundry.org/go-loggregator/v8 v8.0.5 // indirect
	code.cloudfoundry.org/inigo v0.0.0-20210615140442-4bdc4f6e44d5 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cockroachdb/apd v1.1.0 // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/facebookgo/limitgroup v0.0.0-20150612190941-6abd8d71ec01 // indirect
	github.com/facebookgo/muster v0.0.0-20150708232844-fd3d7953fd52 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/go-test/deep v1.0.7 // indirect
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/pprof v0.0.0-20231212022811-ec68065c825e // indirect
	github.com/honeycombio/libhoney-go v1.20.0 // indirect
	github.com/jackc/fake v0.0.0-20150926172116-812a484cc733 // indirect
	github.com/jackc/pgx v3.6.2+incompatible // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/minio/highwayhash v1.0.2 // indirect
	github.com/nats-io/jwt/v2 v2.5.3 // indirect
	github.com/nats-io/nkeys v0.4.6 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.17.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/square/certstrap v1.3.0 // indirect
	github.com/uber-go/atomic v1.11.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.uber.org/atomic v1.8.0 // indirect
	go.uber.org/automaxprocs v1.5.3 // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.5.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231212172506-995d672761c0 // indirect
	gopkg.in/alexcesaro/statsd.v2 v2.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
