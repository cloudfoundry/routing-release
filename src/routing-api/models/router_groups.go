package models

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var InvalidPortError = errors.New("Port must be between 1024 and 65535")

type RouterGroupType string

var ReservedSystemComponentPorts = []int{}

const (
	RouterGroup_TCP  RouterGroupType = "tcp"
	RouterGroup_HTTP RouterGroupType = "http"
)

type RouterGroupsDB []RouterGroupDB

type RouterGroupDB struct {
	Model
	Name            string
	Type            string
	ReservablePorts string
}

type RouterGroup struct {
	Model
	Guid            string          `json:"guid"`
	Name            string          `json:"name"`
	Type            RouterGroupType `json:"type"`
	ReservablePorts ReservablePorts `json:"reservable_ports" yaml:"reservable_ports"`
}

func NewRouterGroupDB(routerGroup RouterGroup) RouterGroupDB {
	if routerGroup.Model.Guid == "" {
		routerGroup.Model = Model{
			Guid: routerGroup.Guid,
		}
	}
	return RouterGroupDB{
		Model:           routerGroup.Model,
		Name:            routerGroup.Name,
		Type:            string(routerGroup.Type),
		ReservablePorts: string(routerGroup.ReservablePorts),
	}
}

func (RouterGroupDB) TableName() string {
	return "router_groups"
}

func (rg *RouterGroupDB) ToRouterGroup() RouterGroup {
	return RouterGroup{
		Model:           rg.Model,
		Guid:            rg.Guid,
		Name:            rg.Name,
		Type:            RouterGroupType(rg.Type),
		ReservablePorts: ReservablePorts(rg.ReservablePorts),
	}
}

func (rgs RouterGroupsDB) ToRouterGroups() RouterGroups {
	routerGroups := RouterGroups{}
	for _, routerGroupDB := range rgs {
		routerGroups = append(routerGroups, routerGroupDB.ToRouterGroup())
	}
	return routerGroups
}

type RouterGroups []RouterGroup

func (g RouterGroups) validateRouterGroupName() error {
	encountered := map[string]bool{}
	for _, r := range g {
		if _, exist := encountered[r.Name]; exist {
			return fmt.Errorf("Router group name must be unique")
		}
		encountered[r.Name] = true
	}
	return nil
}

func (g RouterGroups) Validate() error {
	for _, r := range g {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (g RouterGroup) Validate() error {
	if g.Name == "" {
		return errors.New("Missing name in router group")
	}

	if g.Type == "" {
		return errors.New("Missing type in router group")
	}

	if g.ReservablePorts == "" {
		if g.Type == RouterGroup_TCP {
			return fmt.Errorf("Missing reservable_ports in router group: %s", g.Name)
		}

		return nil
	}

	if g.Type == RouterGroup_HTTP {
		return errors.New("Reservable ports are not supported for router groups of type http")
	}

	return g.ReservablePorts.Validate()
}

type ReservablePorts string

func (p *ReservablePorts) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var input interface{}

	err := unmarshal(&input)
	if err != nil {
		return err // untested
	}

	switch t := input.(type) {
	case int:
		*p = ReservablePorts(strconv.Itoa(t))
	case string:
		*p = ReservablePorts(input.(string))
	case []interface{}:
		var s []string

		for _, v := range t {
			val, ok := v.(int)
			if !ok {
				return errors.New("invalid type for reservable port")
			}

			s = append(s, strconv.Itoa(val))
		}

		*p = ReservablePorts(strings.Join(s, ","))
	default:
		return errors.New("reservable port unmarshal failed") // untested
	}

	return nil
}

func (p ReservablePorts) Validate() error {
	portRanges, err := p.Parse()
	if err != nil {
		return err
	}

	// check for overlapping ranges
	for i, r1 := range portRanges {
		for j, r2 := range portRanges {
			if i == j {
				continue
			}
			if r1.Overlaps(r2) {
				errMsg := fmt.Sprintf("Overlapping values: %s and %s", r1.String(), r2.String())
				return errors.New(errMsg)
			}
		}
	}
	// check if ports overlap with reservedSystemComponentPorts
	for _, r1 := range portRanges {
		for _, reservedPort := range ReservedSystemComponentPorts {
			if uint64(reservedPort) >= r1.start && uint64(reservedPort) <= r1.end {
				errMsg := fmt.Sprintf("Invalid ports. Reservable ports must not include the following reserved system component ports: %v.", ReservedSystemComponentPorts)
				return errors.New(errMsg)
			}
		}
	}
	return nil
}

func (p ReservablePorts) Parse() (Ranges, error) {
	rangesArray := strings.Split(string(p), ",")
	var ranges Ranges

	for _, p := range rangesArray {
		r, err := parseRange(p)
		if err != nil {
			return Ranges{}, err
		} else {
			ranges = append(ranges, r)
		}
	}

	return ranges, nil
}

type Range struct {
	start uint64 // inclusive
	end   uint64 // inclusive
}
type Ranges []Range

func portIsInRange(port uint64) bool {
	return port >= 1024 && port <= 65535
}

func NewRange(start, end uint64) (Range, error) {
	if portIsInRange(start) && portIsInRange(end) {
		return Range{
			start: start,
			end:   end,
		}, nil
	}
	return Range{}, InvalidPortError
}

func (r Range) Overlaps(other Range) bool {
	maxUpper := r.max(other)
	minLower := r.min(other)
	// check bounds for both, then see if size of both fit
	// For example: 10-20 and 15-30
	// |----10-20----|
	//         |-------15-30------|
	// |==========================|
	// 	minLower: 10  maxUpper: 30
	//  (30 - 10) <= (20 - 10) + (30 - 15)
	//         20 <= 25?
	return maxUpper-minLower <= (r.end-r.start)+(other.end-other.start)
}

func (r Range) String() string {
	if r.start == r.end {
		return fmt.Sprintf("%d", r.start)
	}
	return fmt.Sprintf("[%d-%d]", r.start, r.end)
}

func (r Range) max(other Range) uint64 {
	if r.end > other.end {
		return r.end
	}
	return other.end
}

func (r Range) min(other Range) uint64 {
	if r.start < other.start {
		return r.start
	}
	return other.start
}

func (r Range) Endpoints() (uint64, uint64) {
	return r.start, r.end
}

func parseRange(r string) (Range, error) {
	endpoints := strings.Split(r, "-")

	len := len(endpoints)
	switch len {
	case 1:
		n, err := strconv.ParseUint(endpoints[0], 10, 64)
		if err != nil {
			return Range{}, InvalidPortError
		}
		return NewRange(n, n)
	case 2:
		start, err := strconv.ParseUint(endpoints[0], 10, 64)
		if err != nil {
			return Range{}, fmt.Errorf("range (%s) requires a starting port", r)
		}

		end, err := strconv.ParseUint(endpoints[1], 10, 64)
		if err != nil {
			return Range{}, fmt.Errorf("range (%s) requires an ending port", r)
		}

		if start > end {
			return Range{}, fmt.Errorf("range (%s) must be in ascending numeric order", r)
		}

		return NewRange(start, end)
	default:
		return Range{}, fmt.Errorf("range (%s) has too many '-' separators", r)
	}
}
