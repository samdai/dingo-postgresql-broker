package serviceinstance_test

import (
	"fmt"

	"github.com/coreos/go-etcd/etcd"
	"github.com/dingotiles/patroni-broker/backend"
	"github.com/dingotiles/patroni-broker/config"
	"github.com/dingotiles/patroni-broker/serviceinstance"
	"github.com/frodenas/brokerapi"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
)

const small = 1
const medium = 2

// SetFakeSize is used by request_tests to set an initial cluster size before changes
func setupCluster(cluster *serviceinstance.Cluster, existingBackendGUIDs []string) {
	etcdClient := cluster.EtcdClient.(backend.FakeEtcdClient)

	nodes := make(etcd.Nodes, len(existingBackendGUIDs))
	for i, backendGUID := range existingBackendGUIDs {
		key := fmt.Sprintf("/serviceinstances/%s/nodes/instance-%d/backend", cluster.Data.InstanceID, i)
		etcdClient.GetResponses[key] = &etcd.Response{
			Node: &etcd.Node{
				Key:   key,
				Value: backendGUID,
			},
		}
		nodes[i] = &etcd.Node{Key: fmt.Sprintf("/serviceinstances/%s/nodes/instance-%d", cluster.Data.InstanceID, i)}
	}

	key := fmt.Sprintf("/serviceinstances/%s/nodes", cluster.Data.InstanceID)
	etcdClient.GetResponses[key] = &etcd.Response{
		Node: &etcd.Node{Key: key, Nodes: nodes},
	}

}

func backendGUIDs(backends []*config.Backend) []string {
	guids := make([]string, len(backends))
	for i, backend := range backends {
		guids[i] = backend.GUID
	}
	return guids
}

var _ = Describe("backend broker selection", func() {
	etcdClient := backend.NewFakeEtcdClient()
	cfg := &config.Config{}
	var logger lager.Logger
	clusterUUID := "uuid"
	var cluster *serviceinstance.Cluster
	var serviceDetails brokerapi.ProvisionDetails
	var backends []*config.Backend

	Context("two AZs", func() {
		BeforeEach(func() {
			cfg.Backends = []*config.Backend{
				&config.Backend{AvailabilityZone: "z1", GUID: "c1z1"},
				&config.Backend{AvailabilityZone: "z1", GUID: "c2z1"},
				&config.Backend{AvailabilityZone: "z1", GUID: "c3z1"},
				&config.Backend{AvailabilityZone: "z2", GUID: "c4z2"},
				&config.Backend{AvailabilityZone: "z2", GUID: "c5z2"},
				&config.Backend{AvailabilityZone: "z2", GUID: "c6z2"},
			}
		})

		It("orders backends before cluster change from 0 to 1", func() {
			cluster = serviceinstance.NewClusterFromProvisionDetails(clusterUUID, serviceDetails, etcdClient, cfg, logger)
			setupCluster(cluster, []string{})
			backends := cluster.SortedBackendsByUnusedAZs()
			Ω(backendGUIDs(backends)).To(Equal([]string{"c1z1", "c2z1", "c3z1", "c4z2", "c5z2", "c6z2"}))
		})

		Context("orders backends before cluster change from 1 to 2", func() {
			It("has list of backends with z2 first, z1 second, c1z1 last", func() {
				cluster = serviceinstance.NewClusterFromProvisionDetails(clusterUUID, serviceDetails, etcdClient, cfg, logger)
				setupCluster(cluster, []string{"c1z1"})
				backends = cluster.SortedBackendsByUnusedAZs()
				// backend broker already used is last in the list; its AZ is the last AZ
				// example expected result (z2's first, then z1's):
				//   "c4z2", "c5z2", "c6z2", "c2z1", "c3z1", "c1z1"
				Ω(backends[0].GUID).To(MatchRegexp("z2$"))
				Ω(backends[1].GUID).To(MatchRegexp("z2$"))
				Ω(backends[2].GUID).To(MatchRegexp("z2$"))
				Ω(backends[3].GUID).To(MatchRegexp("z1$"))
				Ω(backends[4].GUID).To(MatchRegexp("z1$"))
				Ω(backends[5].GUID).To(Equal("c1z1"))
			})
		})

		Context("orders backends before cluster change from 2 to 3", func() {
			It("has list of backends c1z1,c4z2 last", func() {
				cluster = serviceinstance.NewClusterFromProvisionDetails(clusterUUID, serviceDetails, etcdClient, cfg, logger)
				setupCluster(cluster, []string{"c1z1", "c4z2"})
				backends = cluster.SortedBackendsByUnusedAZs()
				// backend broker already used is last in the list; its AZ is the last AZ
				Ω(backendGUIDs(backends)).To(Equal([]string{"c2z1", "c3z1", "c5z2", "c6z2", "c1z1", "c4z2"}))
			})
		})
	})

	Context("three AZs", func() {
		BeforeEach(func() {
			cfg.Backends = []*config.Backend{
				&config.Backend{AvailabilityZone: "z1", GUID: "c1z1"},
				&config.Backend{AvailabilityZone: "z1", GUID: "c2z1"},
				&config.Backend{AvailabilityZone: "z1", GUID: "c3z1"},
				&config.Backend{AvailabilityZone: "z2", GUID: "c4z2"},
				&config.Backend{AvailabilityZone: "z2", GUID: "c5z2"},
				&config.Backend{AvailabilityZone: "z2", GUID: "c6z2"},
				&config.Backend{AvailabilityZone: "z3", GUID: "c7z3"},
				&config.Backend{AvailabilityZone: "z3", GUID: "c8z3"},
				&config.Backend{AvailabilityZone: "z3", GUID: "c9z3"},
			}
		})

		It("orders backends before cluster change from 0 to 1", func() {
			cluster = serviceinstance.NewClusterFromProvisionDetails(clusterUUID, serviceDetails, etcdClient, cfg, logger)
			setupCluster(cluster, []string{})
			backends := cluster.SortedBackendsByUnusedAZs()
			Ω(backendGUIDs(backends)).To(Equal([]string{"c1z1", "c2z1", "c3z1", "c4z2", "c5z2", "c6z2", "c7z3", "c8z3", "c9z3"}))
		})

		Context("orders backends before cluster change from 1 to 2", func() {
			It("has list of backends with z2 first, z1 second, c1z1 last", func() {
				cluster = serviceinstance.NewClusterFromProvisionDetails(clusterUUID, serviceDetails, etcdClient, cfg, logger)
				setupCluster(cluster, []string{"c1z1"})
				backends = cluster.SortedBackendsByUnusedAZs()
				// backend broker already used is last in the list; its AZ is the last AZ
				Ω(backendGUIDs(backends)).To(Equal([]string{"c4z2", "c5z2", "c6z2", "c7z3", "c8z3", "c9z3", "c2z1", "c3z1", "c1z1"}))
			})
		})

		Context("orders backends before cluster change from 2 to 3", func() {
			It("has list of backends c1z1,c4z2 last", func() {
				cluster = serviceinstance.NewClusterFromProvisionDetails(clusterUUID, serviceDetails, etcdClient, cfg, logger)
				setupCluster(cluster, []string{"c1z1", "c4z2"})
				backends = cluster.SortedBackendsByUnusedAZs()
				// backend broker already used is last in the list; its AZ is the last AZ
				Ω(len(backends)).To(Equal(9))
				Ω(backends[7].GUID).To(Equal("c1z1"))
				Ω(backends[8].GUID).To(Equal("c4z2"))
			})
		})

		Context("orders backends before cluster change from 3 to 4", func() {
			It("has list of backends c1z1,c4z2,c7z3 last", func() {
				cluster = serviceinstance.NewClusterFromProvisionDetails(clusterUUID, serviceDetails, etcdClient, cfg, logger)
				setupCluster(cluster, []string{"c1z1", "c4z2", "c7z3"})
				backends = cluster.SortedBackendsByUnusedAZs()
				// backend broker already used is last in the list; its AZ is the last AZ
				Ω(backendGUIDs(backends)).To(Equal([]string{"c2z1", "c3z1", "c5z2", "c6z2", "c8z3", "c9z3", "c1z1", "c4z2", "c7z3"}))
			})
		})
	})
})
