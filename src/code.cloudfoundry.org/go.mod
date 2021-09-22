module code.cloudfoundry.org

go 1.16

replace github.com/uber-go/zap => github.com/uber-go/zap v0.0.0-20161222040304-a5783ee4b216

replace github.com/uber-go/atomic => github.com/uber-go/atomic v1.1.0

replace github.com/codegangsta/cli => github.com/codegangsta/cli v1.6.0

replace github.com/hashicorp/consul => github.com/hashicorp/consul v0.7.0

replace github.com/cactus/go-statsd-client => github.com/cactus/go-statsd-client v2.0.2-0.20150911070441-6fa055a7b594+incompatible

replace github.com/cloudfoundry/dropsonde => github.com/cloudfoundry/dropsonde v1.0.1-0.20180504154030-a5c24343b09d

require (
	code.cloudfoundry.org/bbs v0.0.0-20210615140220-2942e7d25726
	code.cloudfoundry.org/cfhttp/v2 v2.0.1-0.20210513172332-4c5ee488a657
	code.cloudfoundry.org/clock v1.0.0
	code.cloudfoundry.org/consuladapter v0.0.0-20210615194356-31457193d2fd // indirect
	code.cloudfoundry.org/debugserver v0.0.0-20210608171006-d7658ce493f4
	code.cloudfoundry.org/diego-logging-client v0.0.0-20210622170659-8861ae5ba2ed // indirect
	code.cloudfoundry.org/durationjson v0.0.0-20210615172401-3a89d41c90da // indirect
	code.cloudfoundry.org/eventhub v0.0.0-20210615172938-0b896ce72257
	code.cloudfoundry.org/inigo v0.0.0-20210615140442-4bdc4f6e44d5 // indirect
	code.cloudfoundry.org/lager v2.0.0+incompatible
	code.cloudfoundry.org/localip v0.0.0-20210608161955-43c3ec713c20
	code.cloudfoundry.org/locket v0.0.0-20210706150026-b8ae29a4de0f
	code.cloudfoundry.org/rep v0.1441.2 // indirect
	code.cloudfoundry.org/tlsconfig v0.0.0-20210615191307-5d92ef3894a7
	github.com/armon/go-metrics v0.3.9 // indirect
	github.com/armon/go-proxyproto v0.0.0-20210323213023-7e956b284f0a
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cactus/go-statsd-client v0.0.0-00010101000000-000000000000
	github.com/cloudfoundry-incubator/bbs v0.0.0-20210615140220-2942e7d25726 // indirect
	github.com/cloudfoundry-incubator/cf-test-helpers v1.0.0
	github.com/cloudfoundry-incubator/executor v0.0.0-20210615140407-a538c11377aa // indirect
	github.com/cloudfoundry/custom-cats-reporters v0.0.2
	github.com/cloudfoundry/dropsonde v1.0.0
	github.com/cloudfoundry/sonde-go v0.0.0-20200416163440-a42463ba266b
	github.com/cockroachdb/apd v1.1.0 // indirect
	github.com/codegangsta/cli v0.0.0-00010101000000-000000000000
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/go-sql-driver/mysql v1.6.0
	github.com/go-test/deep v1.0.7 // indirect
	github.com/gofrs/uuid v4.0.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.2
	github.com/hashicorp/consul v1.10.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/honeycombio/libhoney-go v1.15.3
	github.com/jackc/fake v0.0.0-20150926172116-812a484cc733 // indirect
	github.com/jackc/pgx v3.6.2+incompatible // indirect
	github.com/jinzhu/gorm v1.9.16
	github.com/kisielk/errcheck v1.6.0
	github.com/lib/pq v1.10.2
	github.com/mailru/easyjson v0.7.7
	github.com/nats-io/nats-server/v2 v2.3.0
	github.com/nats-io/nats.go v1.11.0
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/pivotal-golang/clock v1.0.0 // indirect
	github.com/pivotal-golang/lager v2.0.0+incompatible // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/tedsuo/ifrit v0.0.0-20191009134036-9a97d0632f00
	github.com/tedsuo/rata v1.0.0
	github.com/uber-go/atomic v0.0.0-00010101000000-000000000000 // indirect
	github.com/uber-go/zap v0.0.0-00010101000000-000000000000
	github.com/urfave/negroni v1.0.0
	github.com/vito/go-sse v1.0.0
	go.uber.org/atomic v1.8.0 // indirect
	golang.org/x/crypto v0.0.0-20210616213533-5ff15b29337e
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	golang.org/x/tools v0.1.4
	google.golang.org/genproto v0.0.0-20210701191553-46259e63a0a9 // indirect
	google.golang.org/grpc v1.39.0
	gopkg.in/yaml.v2 v2.4.0
)
