package models_test

import (
	"encoding/json"

	. "code.cloudfoundry.org/routing-release/routing-api/models"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Models", func() {
	Describe("ModificationTag", func() {
		var tag ModificationTag

		BeforeEach(func() {
			tag = ModificationTag{Guid: "guid1", Index: 5}
		})

		Describe("Increment", func() {
			BeforeEach(func() {
				tag.Increment()
			})

			It("Increments the index", func() {
				Expect(tag.Index).To(Equal(uint32(6)))
			})
		})

		Describe("SucceededBy", func() {
			var tag2 ModificationTag

			Context("when the guid is the different", func() {
				BeforeEach(func() {
					tag2 = ModificationTag{Guid: "guid5", Index: 0}
				})
				It("new tag should succeed", func() {
					Expect(tag.SucceededBy(&tag2)).To(BeTrue())
				})
			})

			Context("when the guid is the same", func() {

				Context("when the index is the same as the original tag", func() {
					BeforeEach(func() {
						tag2 = ModificationTag{Guid: "guid1", Index: 5}
					})

					It("new tag should not succeed", func() {
						Expect(tag.SucceededBy(&tag2)).To(BeFalse())
					})

				})

				Context("when the index is less than original tag Index", func() {

					BeforeEach(func() {
						tag2 = ModificationTag{Guid: "guid1", Index: 4}
					})

					It("new tag should not succeed", func() {
						Expect(tag.SucceededBy(&tag2)).To(BeFalse())
					})
				})

				Context("when the index is greater than original tag Index", func() {
					BeforeEach(func() {
						tag2 = ModificationTag{Guid: "guid1", Index: 6}
					})

					It("new tag should succeed", func() {
						Expect(tag.SucceededBy(&tag2)).To(BeTrue())
					})

				})

			})

		})
	})

	Describe("RouterGroup", func() {
		var rg RouterGroup

		Describe("Validate", func() {
			It("does not allow ReservablePorts for http type", func() {
				rg = RouterGroup{
					Name:            "router-group-1",
					Type:            "http",
					ReservablePorts: "1025-2025",
				}
				err := rg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Reservable ports are not supported for router groups of type http"))
				By("not having ReservablePorts")
				rg = RouterGroup{
					Name: "router-group-1",
					Type: "http",
				}
				err = rg.Validate()
				Expect(err).ToNot(HaveOccurred())
			})

			It("ReservablePorts are optional for non-http, non-tcp type", func() {
				rg = RouterGroup{
					Name:            "router-group-1",
					Type:            "foo",
					ReservablePorts: "1025-2025",
				}
				err := rg.Validate()
				Expect(err).ToNot(HaveOccurred())

				rg = RouterGroup{
					Name:            "router-group-1",
					Type:            "foo",
					ReservablePorts: "",
				}
				err = rg.Validate()
				Expect(err).ToNot(HaveOccurred())
			})

			It("succeeds for valid router group", func() {
				rg = RouterGroup{
					Name:            "router-group-1",
					Type:            "tcp",
					ReservablePorts: "1025-2025",
				}
				err := rg.Validate()
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails for missing type", func() {
				rg = RouterGroup{
					Name:            "router-group-1",
					ReservablePorts: "10-20",
				}
				err := rg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Missing type in router group"))
			})
			It("fails for missing name", func() {
				rg = RouterGroup{
					Type:            "tcp",
					ReservablePorts: "10-20",
				}
				err := rg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Missing name in router group"))
			})

			It("fails for missing ReservablePorts", func() {
				rg = RouterGroup{
					Type: "tcp",
					Name: "router-group-1",
				}
				err := rg.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Missing reservable_ports in router group: router-group-1"))
			})

			Context("when there are reserved system component ports", func(){
				BeforeEach(func(){
					ReservedSystemComponentPorts = []int{5555, 6666, 7777}
				})

				It("succeeds when the ports don't overlap", func(){
					rg = RouterGroup{
						Name:            "router-group-1",
						Type:            "tcp",
						ReservablePorts: "1025-2025",
					}
					err := rg.Validate()
					Expect(err).ToNot(HaveOccurred())
				})

				It("fails when the ports overlap", func(){
					rg = RouterGroup{
						Name:            "router-group-1",
						Type:            "tcp",
						ReservablePorts: "5000-6000",
					}
					err := rg.Validate()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal("Invalid ports. Reservable ports must not include the following reserved system component ports: [5555 6666 7777]."))
				})
			})
		})
	})

	Describe("ReservablePorts", func() {
		var ports ReservablePorts

		Describe("Validate", func() {
			It("succeeds for valid reservable ports", func() {
				ports = "6001,6005,6010-6020,6021-6030"
				err := ports.Validate()
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails for overlapping ranges", func() {
				ports = "6010-6020,6020-6030"
				err := ports.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Overlapping values: [6010-6020] and [6020-6030]"))
			})

			It("fails for overlapping values", func() {
				ports = "6001,6001,6002,6003,6003,6004"
				err := ports.Validate()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("Overlapping values: 6001 and 6001"))
			})

			It("fails for invalid reservable ports", func() {
				ports = "foo!"
				err := ports.Validate()
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("Parse", func() {
			It("validates a single unsigned integer", func() {
				ports = "9999"
				r, err := ports.Parse()
				Expect(err).NotTo(HaveOccurred())

				Expect(len(r)).To(Equal(1))
				start, end := r[0].Endpoints()
				Expect(start).To(Equal(uint64(9999)))
				Expect(end).To(Equal(uint64(9999)))
			})

			It("validates multiple integers", func() {
				ports = "9999,1111,2222"
				r, err := ports.Parse()
				Expect(err).NotTo(HaveOccurred())
				Expect(len(r)).To(Equal(3))

				expected := []uint64{9999, 1111, 2222}
				for i := 0; i < len(r); i++ {
					start, end := r[i].Endpoints()
					Expect(start).To(Equal(expected[i]))
					Expect(end).To(Equal(expected[i]))
				}
			})

			It("validates a range", func() {
				ports = "10241-10249"
				r, err := ports.Parse()
				Expect(err).NotTo(HaveOccurred())

				Expect(len(r)).To(Equal(1))
				start, end := r[0].Endpoints()
				Expect(start).To(Equal(uint64(10241)))
				Expect(end).To(Equal(uint64(10249)))
			})

			It("validates a list of ranges and integers", func() {
				ports = "6001-6010,6020-6022,6045,6050-6060"
				r, err := ports.Parse()
				Expect(err).NotTo(HaveOccurred())

				Expect(len(r)).To(Equal(4))
				expected := []uint64{6001, 6010, 6020, 6022, 6045, 6045, 6050, 6060}
				for i := 0; i < len(r); i++ {
					start, end := r[i].Endpoints()
					Expect(start).To(Equal(expected[2*i]))
					Expect(end).To(Equal(expected[2*i+1]))
				}
			})

			It("errors on range with 3 dashes", func() {
				ports = "10-999-1000"
				_, err := ports.Parse()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("range (10-999-1000) has too many '-' separators"))
			})

			It("errors on a negative integer", func() {
				ports = "-9999"
				_, err := ports.Parse()
				Expect(err).To(HaveOccurred())
			})

			It("errors on a incomplete range", func() {
				ports = "1030-"
				_, err := ports.Parse()
				Expect(err).To(HaveOccurred())
			})

			It("errors on non-numeric input", func() {
				ports = "adsfasdf"
				_, err := ports.Parse()
				Expect(err).To(HaveOccurred())
			})

			It("errors when range starts with lower number", func() {
				ports = "10000-9999"
				_, err := ports.Parse()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("range (10000-9999) must be in ascending numeric order"))
			})
		})
	})

	Describe("Range", func() {
		Describe("Overlaps", func() {
			testRange, _ := NewRange(6010, 6020)

			It("validates non-overlapping ranges", func() {
				r, _ := NewRange(6021, 6030)
				Expect(testRange.Overlaps(r)).To(BeFalse())
			})

			It("finds overlapping ranges of single values", func() {
				r1, _ := NewRange(6010, 6010)
				r2, _ := NewRange(6010, 6010)
				Expect(r1.Overlaps(r2)).To(BeTrue())
			})

			It("finds overlapping ranges of single value and range", func() {
				r2, _ := NewRange(6015, 6015)
				Expect(testRange.Overlaps(r2)).To(BeTrue())
			})

			It("finds overlapping ranges of single value upper bound and range", func() {
				r2, _ := NewRange(6020, 6020)
				Expect(testRange.Overlaps(r2)).To(BeTrue())
			})

			It("validates single value one above upper bound range", func() {
				r2, _ := NewRange(6021, 6021)
				Expect(testRange.Overlaps(r2)).To(BeFalse())
			})

			It("finds overlapping ranges when start overlaps", func() {
				r, _ := NewRange(6015, 6030)
				Expect(testRange.Overlaps(r)).To(BeTrue())
			})

			It("finds overlapping ranges when end overlaps", func() {
				r, _ := NewRange(6005, 6015)
				Expect(testRange.Overlaps(r)).To(BeTrue())
			})

			It("finds overlapping ranges when the range is a superset", func() {
				r, _ := NewRange(6009, 6021)
				Expect(testRange.Overlaps(r)).To(BeTrue())
			})
		})
	})

	Describe("Route", func() {
		var (
			route Route
		)

		BeforeEach(func() {
			tag, err := NewModificationTag()
			Expect(err).ToNot(HaveOccurred())
			route = NewRoute("/foo/bar", 35, "2.2.2.2", "", "banana", 66)
			route.ModificationTag = tag
		})

		Describe("SetDefaults", func() {
			JustBeforeEach(func() {
				route.SetDefaults(120)
			})

			Context("when ttl is nil", func() {
				BeforeEach(func() {
					route.TTL = nil
				})

				It("sets the default ttl", func() {
					Expect(*route.TTL).To(Equal(120))
				})
			})

			Context("when ttl is not nil", func() {
				It("does not change ttl", func() {
					Expect(*route.TTL).To(Equal(66))
				})
			})
		})
	})

	Describe("TcpRouteMapping", func() {
		var (
			route TcpRouteMapping
		)

		BeforeEach(func() {
			tag, err := NewModificationTag()
			Expect(err).ToNot(HaveOccurred())
			route = NewTcpRouteMapping("router-group-1", 60000, "2.2.2.2", 64000, 66)
			route.ModificationTag = tag
		})

		Describe("SetDefaults", func() {
			JustBeforeEach(func() {
				route.SetDefaults(120)
			})

			Context("when ttl is nil", func() {
				BeforeEach(func() {
					route.TTL = nil
				})

				It("sets default ttl", func() {
					Expect(*route.TTL).To(Equal(120))
				})
			})

			Context("when ttl is not nil", func() {
				It("doesn't change ttl", func() {
					Expect(*route.TTL).To(Equal(66))
				})
			})
		})

		Context("multiple annotations", func() {
			It("return router group object", func() {
				jsonStr :=
					`
{ "guid": "some-guid",
	"name": "name"
}
`
				rg := RouterGroup{}
				err := json.Unmarshal([]byte(jsonStr), &rg)
				Expect(err).ToNot(HaveOccurred())
				Expect(rg.Guid).To(Equal("some-guid"))
				Expect(rg.Name).To(Equal("name"))
			})
		})
	})
})
