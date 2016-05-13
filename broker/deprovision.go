package broker

import (
	"fmt"

	"github.com/coreos/go-etcd/etcd"
	"github.com/frodenas/brokerapi"
	"github.com/pivotal-golang/lager"
)

// Deprovision service instance
func (bkr *Broker) Deprovision(instanceID string, deprovDetails brokerapi.DeprovisionDetails, acceptsIncomplete bool) (async bool, err error) {
	if deprovDetails.ServiceID == "" || deprovDetails.PlanID == "" {
		return false, fmt.Errorf("API error - provide service_id and plan_id as URL parameters")
	}

	logger := bkr.logger
	cluster, err := bkr.state.LoadCluster(instanceID)

	cluster.SetTargetNodeCount(0)
	clusterRequest := bkr.scheduler.NewRequest(cluster)
	bkr.scheduler.Execute(clusterRequest)

	var resp *etcd.Response
	resp, err = bkr.etcdClient.Delete(fmt.Sprintf("/serviceinstances/%s", instanceID), true)
	if err != nil {
		logger.Error("etcd-delete.serviceinstances.err", err, lager.Data{"response": resp})
	}
	resp, err = bkr.etcdClient.Delete(fmt.Sprintf("/routing/allocation/%s", instanceID), true)
	if err != nil {
		logger.Error("etcd-delete.routing-allocation.err", err, lager.Data{"response": resp})
	}

	// clear out etcd data that would eventually timeout; to allow immediate recreation if required by user
	resp, err = bkr.etcdClient.Delete(fmt.Sprintf("/service/%s/members", instanceID), true)
	if err != nil {
		logger.Error("etcd-delete.service-members.err", err, lager.Data{"response": resp})
	}
	resp, err = bkr.etcdClient.Delete(fmt.Sprintf("/service/%s/optime", instanceID), true)
	if err != nil {
		logger.Error("etcd-delete.service-optime.err", err, lager.Data{"response": resp})
	}
	resp, err = bkr.etcdClient.Delete(fmt.Sprintf("/service/%s/leader", instanceID), true)
	if err != nil {
		logger.Error("etcd-delete.service-leader.err", err, lager.Data{"response": resp})
	}
	logger.Info("etcd-delete.done")
	return false, nil
}
