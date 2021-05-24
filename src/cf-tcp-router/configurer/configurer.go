package configurer

import (
	"errors"

	"code.cloudfoundry.org/routing-release/cf-tcp-router/configurer/haproxy"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/models"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/monitor"
	"code.cloudfoundry.org/lager"
)

const (
	HaProxyConfigurer = "HAProxy"
)

//go:generate counterfeiter -o fakes/fake_configurer.go . RouterConfigurer
type RouterConfigurer interface {
	Configure(routingTable models.RoutingTable) error
}

func NewConfigurer(logger lager.Logger, tcpLoadBalancer string, tcpLoadBalancerBaseCfg string, tcpLoadBalancerCfg string, monitor monitor.Monitor, scriptRunner haproxy.ScriptRunner) RouterConfigurer {
	switch tcpLoadBalancer {
	case HaProxyConfigurer:
		routerHostInfo, err := haproxy.NewHaProxyConfigurer(
			logger,
			haproxy.NewConfigMarshaller(),
			tcpLoadBalancerBaseCfg,
			tcpLoadBalancerCfg,
			monitor,
			scriptRunner)

		if err != nil {
			logger.Fatal("could not create tcp load balancer",
				err,
				lager.Data{"tcp_load_balancer": tcpLoadBalancer})
			return nil
		}
		return routerHostInfo
	default:
		logger.Fatal("not-supported", errors.New("unsupported tcp load balancer"), lager.Data{"tcp_load_balancer": tcpLoadBalancer})
		return nil
	}
}
