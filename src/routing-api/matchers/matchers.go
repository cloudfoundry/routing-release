package matchers

import (
	routing_api "code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

func MatchTcpRoute(target models.TcpRouteMapping) types.GomegaMatcher {
	return SatisfyAll(
		WithTransform(func(t models.TcpRouteMapping) string {
			return t.RouterGroupGuid
		}, Equal(target.RouterGroupGuid)),
		WithTransform(func(t models.TcpRouteMapping) string {
			return t.HostIP
		}, Equal(target.HostIP)),
		WithTransform(func(t models.TcpRouteMapping) uint16 {
			return t.HostPort
		}, Equal(target.HostPort)),
		WithTransform(func(t models.TcpRouteMapping) uint16 {
			return t.ExternalPort
		}, Equal(target.ExternalPort)),
		WithTransform(func(t models.TcpRouteMapping) string {
			return t.IsolationSegment
		}, Equal(target.IsolationSegment)),
	)
}

func MatchRouterGroup(target models.RouterGroup) types.GomegaMatcher {
	return SatisfyAll(
		WithTransform(func(t models.RouterGroup) string {
			return t.Guid
		}, Equal(target.Guid)),
		WithTransform(func(t models.RouterGroup) string {
			return t.Name
		}, Equal(target.Name)),
		WithTransform(func(t models.RouterGroup) models.RouterGroupType {
			return t.Type
		}, Equal(target.Type)),
		WithTransform(func(t models.RouterGroup) models.ReservablePorts {
			return t.ReservablePorts
		}, Equal(target.ReservablePorts)),
	)
}

func MatchHttpRoute(target models.Route) types.GomegaMatcher {
	return SatisfyAll(
		WithTransform(func(t models.Route) string {
			return t.Route
		}, Equal(target.Route)),
		WithTransform(func(t models.Route) uint16 {
			return t.Port
		}, Equal(target.Port)),
		WithTransform(func(t models.Route) string {
			return t.IP
		}, Equal(target.IP)),
		WithTransform(func(t models.Route) string {
			return t.LogGuid
		}, Equal(target.LogGuid)),
		WithTransform(func(t models.Route) string {
			return t.RouteServiceUrl
		}, Equal(target.RouteServiceUrl)),
	)
}

func MatchHttpEvent(target routing_api.Event) types.GomegaMatcher {
	return SatisfyAll(
		WithTransform(func(t routing_api.Event) string {
			return t.Action
		}, Equal(target.Action)),
		WithTransform(func(t routing_api.Event) models.Route {
			return t.Route
		}, MatchHttpRoute(target.Route)),
	)
}
