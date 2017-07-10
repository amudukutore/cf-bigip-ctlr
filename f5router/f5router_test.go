/*-
 * Copyright (c) 2016,2017, F5 Networks, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package f5router

import (
	"encoding/json"
	"os"
	"sort"
	"sync"

	"github.com/F5Networks/cf-bigip-ctlr/config"
	"github.com/F5Networks/cf-bigip-ctlr/metrics/fakes"
	"github.com/F5Networks/cf-bigip-ctlr/registry"
	"github.com/F5Networks/cf-bigip-ctlr/route"
	"github.com/F5Networks/cf-bigip-ctlr/test_util"

	"code.cloudfoundry.org/routing-api/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
)

var _ = Describe("F5Router", func() {
	Describe("sorting", func() {
		Context("rules", func() {
			It("should sort correctly", func() {
				l7 := rules{}

				expectedList := make(rules, 10)

				p := rule{}
				p.FullURI = "bar"
				l7 = append(l7, &p)
				expectedList[1] = &p

				p = rule{}
				p.FullURI = "foo"
				l7 = append(l7, &p)
				expectedList[5] = &p

				p = rule{}
				p.FullURI = "foo"
				l7 = append(l7, &p)
				expectedList[7] = &p

				p = rule{}
				p.FullURI = "baz"
				l7 = append(l7, &p)
				expectedList[2] = &p

				p = rule{}
				p.FullURI = "foo"
				l7 = append(l7, &p)
				expectedList[6] = &p

				p = rule{}
				p.FullURI = "foo"
				l7 = append(l7, &p)
				expectedList[9] = &p

				p = rule{}
				p.FullURI = "baz"
				l7 = append(l7, &p)
				expectedList[3] = &p

				p = rule{}
				p.FullURI = "foo"
				l7 = append(l7, &p)
				expectedList[8] = &p

				p = rule{}
				p.FullURI = "foo"
				l7 = append(l7, &p)
				expectedList[4] = &p

				p = rule{}
				p.FullURI = "bar"
				l7 = append(l7, &p)
				expectedList[0] = &p

				sort.Sort(l7)

				for i := range expectedList {
					Expect(l7[i]).To(Equal(expectedList[i]),
						"Sorted list elements should be equal")
				}
			})
		})
	})

	Describe("verify configs", func() {
		It("should process the config correctly", func() {
			logger := test_util.NewTestZapLogger("router-test")
			mw := &MockWriter{}
			c := config.DefaultConfig()

			r, err := NewF5Router(logger, nil, mw)
			Expect(r).To(BeNil())
			Expect(err).To(HaveOccurred())

			r, err = NewF5Router(logger, c, nil)
			Expect(r).To(BeNil())
			Expect(err).To(HaveOccurred())

			c.BigIP.URL = "http://example.com"
			r, err = NewF5Router(logger, c, mw)
			Expect(r).To(BeNil())
			Expect(err).To(HaveOccurred())

			c.BigIP.User = "admin"
			r, err = NewF5Router(logger, c, mw)
			Expect(r).To(BeNil())
			Expect(err).To(HaveOccurred())

			c.BigIP.Pass = "pass"
			r, err = NewF5Router(logger, c, mw)
			Expect(r).To(BeNil())
			Expect(err).To(HaveOccurred())

			c.BigIP.Partitions = []string{"cf"}
			r, err = NewF5Router(logger, c, mw)
			Expect(r).To(BeNil())
			Expect(err).To(HaveOccurred())

			c.BigIP.ExternalAddr = "127.0.0.1"
			r, err = NewF5Router(logger, c, mw)
			Expect(r).NotTo(BeNil())
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("running router", func() {
		var mw *MockWriter
		var router *F5Router
		var err error
		var logger *test_util.TestZapLogger
		var c *config.Config
		var r registry.Registry

		var (
			fooEndpoint,
			barEndpoint,
			bar2Endpoint,
			bazEndpoint,
			baz2Endpoint,
			bazSegment1Endpoint,
			baz2Segment1Endpoint,
			bazSegment3Endpoint,
			baz2Segment3Endpoint,
			wildCfEndpoint,
			wildFooEndpoint,
			quxEndpoint *route.Endpoint
		)

		registerRoutes := func() {
			// this weird pattern let's us test some concurrency while still
			// keeping the pool internal lists sorted for easier matching
			go func() {
				r.Register("foo.cf.com", fooEndpoint)
			}()
			go func() {
				r.Register("bar.cf.com", barEndpoint)
				r.Register("bar.cf.com", bar2Endpoint)
			}()
			go func() {
				r.Register("baz.cf.com", bazEndpoint)
			}()
			go func() {
				r.Register("baz.cf.com/segment1", bazSegment1Endpoint)
				r.Register("baz.cf.com/segment1", baz2Segment1Endpoint)
			}()
			go func() {
				r.Register("baz.cf.com/segment1/segment2/segment3", bazSegment3Endpoint)
				r.Register("baz.cf.com/segment1/segment2/segment3", baz2Segment3Endpoint)
			}()
			go func() {
				r.Register("*.cf.com", wildCfEndpoint)
			}()
			go func() {
				r.Register("*.foo.cf.com", wildFooEndpoint)
			}()
		}

		BeforeEach(func() {
			logger = test_util.NewTestZapLogger("router-test")
			c = makeConfig()
			mw = &MockWriter{}

			router, err = NewF5Router(logger, c, mw)

			Expect(router).NotTo(BeNil(), "%v", router)
			Expect(err).NotTo(HaveOccurred())

			fooEndpoint = makeEndpoint("127.0.0.1")
			barEndpoint = makeEndpoint("127.0.1.1")
			bar2Endpoint = makeEndpoint("127.0.1.2")
			bazEndpoint = makeEndpoint("127.0.2.1")
			baz2Endpoint = makeEndpoint("127.0.2.2")
			bazSegment1Endpoint = makeEndpoint("127.0.3.1")
			baz2Segment1Endpoint = makeEndpoint("127.0.3.2")
			bazSegment3Endpoint = makeEndpoint("127.0.4.1")
			baz2Segment3Endpoint = makeEndpoint("127.0.4.2")
			wildCfEndpoint = makeEndpoint("127.0.5.1")
			wildFooEndpoint = makeEndpoint("127.0.6.1")
			quxEndpoint = makeEndpoint("127.0.7.1")

			r = registry.NewRouteRegistry(
				logger,
				c,
				router,
				new(fakes.FakeRouteRegistryReporter),
				"",
			)
		})

		AfterEach(func() {
			if nil != logger {
				logger.Close()
			}
		})

		It("should run", func() {
			done := make(chan struct{})
			os := make(chan os.Signal)
			ready := make(chan struct{})

			go func() {
				defer GinkgoRecover()
				Expect(func() {
					err = router.Run(os, ready)
					Expect(err).NotTo(HaveOccurred())
					close(done)
				}).NotTo(Panic())
			}()
			// wait for the router to be ready
			Eventually(ready).Should(BeClosed(), "timed out waiting for ready")
			// send a kill signal to the router
			os <- MockSignal(123)
			//wait for the router to stop
			Eventually(done).Should(BeClosed(), "timed out waiting for Run to complete")
		})

		It("should update routes", func() {
			done := make(chan struct{})
			os := make(chan os.Signal)
			ready := make(chan struct{})
			update := 0

			go func() {
				defer GinkgoRecover()
				Expect(func() {
					err = router.Run(os, ready)
					Expect(err).NotTo(HaveOccurred())
					close(done)
				}).NotTo(Panic())
			}()

			registerRoutes()
			// config0
			matchConfig(mw, expectedConfigs[update])
			update++

			// make some changes and update the verification function
			r.Unregister("bar.cf.com", barEndpoint)
			p := r.LookupWithoutWildcard("bar.cf.com")
			Expect(p).ToNot(BeNil())
			Expect(p.FindById(barEndpoint.CanonicalAddr())).To(BeNil())

			r.Unregister("baz.cf.com/segment1", baz2Segment1Endpoint)
			p = r.LookupWithoutWildcard("baz.cf.com/segment1")
			Expect(p).ToNot(BeNil())
			Expect(p.FindById(baz2Segment1Endpoint.CanonicalAddr())).To(BeNil())

			r.Register("baz.cf.com", baz2Endpoint)
			p = r.LookupWithoutWildcard("baz.cf.com")
			Expect(p).ToNot(BeNil())
			Expect(p.FindById(baz2Endpoint.CanonicalAddr())).ToNot(BeNil())

			r.Unregister("*.foo.cf.com", wildFooEndpoint)
			p = r.LookupWithoutWildcard("*.foo.cf.com")
			Expect(p).To(BeNil())

			r.Unregister("foo.cf.com", fooEndpoint)
			p = r.LookupWithoutWildcard("foo.cf.com")
			Expect(p).To(BeNil())
			// config1
			matchConfig(mw, expectedConfigs[update])
			update++

			r.Register("qux.cf.com", quxEndpoint)
			p = r.LookupWithoutWildcard("qux.cf.com")
			Expect(p).ToNot(BeNil())
			Expect(p.FindById(quxEndpoint.CanonicalAddr())).ToNot(BeNil())
			// config2
			matchConfig(mw, expectedConfigs[update])

			os <- MockSignal(123)
			Eventually(done).Should(BeClosed(), "timed out waiting for Run to complete")
		})

		It("should handle policies, profiles and health monitors", func() {
			done := make(chan struct{})
			os := make(chan os.Signal)
			ready := make(chan struct{})

			c.BigIP.HealthMonitors = []string{"Common/potato"}
			c.BigIP.SSLProfiles = []string{"Common/clientssl"}
			c.BigIP.Profiles = []string{"Common/http", "/Common/fakeprofile"}
			c.BigIP.Policies = []string{"Common/fakepolicy", "/cf/anotherpolicy"}

			router, err = NewF5Router(logger, c, mw)
			r = registry.NewRouteRegistry(
				logger,
				c,
				router,
				new(fakes.FakeRouteRegistryReporter),
				"",
			)

			go func() {
				defer GinkgoRecover()
				Expect(func() {
					err = router.Run(os, ready)
					Expect(err).NotTo(HaveOccurred())
					close(done)
				}).NotTo(Panic())
			}()
			Eventually(ready).Should(BeClosed())

			registerRoutes()

			matchConfig(mw, expectedConfigs[3])
		})

		Context("fail cases", func() {
			It("should error when not passing a URI for route update", func() {
				done := make(chan struct{})
				os := make(chan os.Signal)
				ready := make(chan struct{})

				go func() {
					defer GinkgoRecover()
					Expect(func() {
						err = router.Run(os, ready)
						Expect(err).NotTo(HaveOccurred())
						close(done)
					}).NotTo(Panic())
				}()

				r.Register("", fooEndpoint)

				Eventually(logger).Should(Say("f5router-skipping-update"))
			})

			It("should error on invalid policy format", func() {
				done := make(chan struct{})
				os := make(chan os.Signal)
				ready := make(chan struct{})

				c.BigIP.Policies = []string{"fakepolicy", "/cf/anotherpolicy"}

				router, err = NewF5Router(logger, c, mw)
				r = registry.NewRouteRegistry(
					logger,
					c,
					router,
					new(fakes.FakeRouteRegistryReporter),
					"",
				)

				go func() {
					defer GinkgoRecover()
					Expect(func() {
						err = router.Run(os, ready)
						Expect(err).NotTo(HaveOccurred())
						close(done)
					}).NotTo(Panic())
				}()
				Eventually(ready).Should(BeClosed())

				Eventually(logger).Should(Say("f5router-skipping-name"))
				p, _ := json.Marshal(router.routeVSHTTP.Policies)
				cp := `[{"name":"anotherpolicy","partition":"cf"},{"name":"cf-routing-policy","partition":"cf"}]`
				Eventually(p).Should(MatchJSON(cp))
			})

		})

	})
})

type configMatcher struct {
	Global    globalConfig       `json:"global"`
	BigIP     config.BigIPConfig `json:"bigip"`
	Resources resources          `json:"resources"`
}

type testRoutes struct {
	Key         route.Uri
	Addrs       []*route.Endpoint
	ContextPath string
}

type MockWriter struct {
	sync.Mutex
	input []byte
}

func (mw *MockWriter) GetOutputFilename() string {
	return "mock-file"
}

func (mw *MockWriter) Write(input []byte) (n int, err error) {
	mw.Lock()
	defer mw.Unlock()
	mw.input = input

	return len(input), nil
}

func (mw *MockWriter) getInput() *configMatcher {
	mw.Lock()
	defer mw.Unlock()
	var m configMatcher
	err := json.Unmarshal(mw.input, &m)
	Expect(err).To(BeNil())
	return &m
}

type MockSignal int

func (ms MockSignal) String() string {
	return "mock signal"
}

func (ms MockSignal) Signal() {
	return
}

func makeConfig() *config.Config {
	c := config.DefaultConfig()
	c.BigIP.URL = "http://example.com"
	c.BigIP.User = "admin"
	c.BigIP.Pass = "pass"
	c.BigIP.Partitions = []string{"cf"}
	c.BigIP.ExternalAddr = "127.0.0.1"

	return c
}

func makeEndpoint(addr string) *route.Endpoint {
	r := route.NewEndpoint("1",
		addr,
		80,
		"1",
		"1",
		make(map[string]string),
		1,
		"",
		models.ModificationTag{
			Guid:  "1",
			Index: 1,
		},
	)
	return r
}

func matchConfig(mw *MockWriter, expected []byte) {
	var matcher configMatcher
	err := json.Unmarshal(expected, &matcher)
	ExpectWithOffset(1, err).To(BeNil())

	EventuallyWithOffset(1, func() globalConfig {
		return mw.getInput().Global
	}).Should(Equal(matcher.Global))
	EventuallyWithOffset(1, func() config.BigIPConfig {
		return mw.getInput().BigIP
	}).Should(Equal(matcher.BigIP))
	EventuallyWithOffset(1, func() []*virtual {
		return mw.getInput().Resources.Virtuals
	}).Should(ConsistOf(matcher.Resources.Virtuals))
	EventuallyWithOffset(1, func() []*pool {
		return mw.getInput().Resources.Pools
	}).Should(ConsistOf(matcher.Resources.Pools))
	EventuallyWithOffset(1, func() []*monitor {
		return mw.getInput().Resources.Monitors
	}).Should(ConsistOf(matcher.Resources.Monitors))
	EventuallyWithOffset(1, func() []*policy {
		return mw.getInput().Resources.Policies
	}).Should(ConsistOf(matcher.Resources.Policies))
}