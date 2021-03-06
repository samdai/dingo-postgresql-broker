package broker

import (
	"fmt"

	"github.com/samdai/dingo-postgresql-broker/broker/structs"
	"github.com/samdai/dingo-postgresql-broker/state"
	"github.com/frodenas/brokerapi"
	"github.com/pivotal-golang/lager"
)

// Update service instance
func (bkr *Broker) Update(instanceID string, updateDetails brokerapi.UpdateDetails, acceptsIncomplete bool) (async bool, err error) {
	return bkr.update(structs.ClusterID(instanceID), updateDetails, acceptsIncomplete)
}
func (bkr *Broker) update(instanceID structs.ClusterID, updateDetails brokerapi.UpdateDetails, acceptsIncomplete bool) (async bool, err error) {
	logger := bkr.newLoggingSession("update", lager.Data{"instance-id": instanceID})
	defer logger.Info("done")

	features, err := structs.ClusterFeaturesFromParameters(updateDetails.Parameters)
	if err != nil {
		logger.Error("cluster-features", err)
		return false, err
	}

	if err := bkr.assertUpdatePrecondition(instanceID, features); err != nil {
		logger.Error("preconditions.error", err)
		return false, err
	}

	clusterState, err := bkr.state.LoadCluster(instanceID)
	if err != nil {
		logger.Error("load-cluster.error", err)
		return false, err
	}
	clusterModel := state.NewClusterModel(bkr.state, clusterState)

	go func() {
		err = bkr.scheduler.RunCluster(clusterModel, features)
		if err != nil {
			logger.Error("run-cluster", err)
		}
	}()
	return true, err
}

func (bkr *Broker) assertUpdatePrecondition(instanceID structs.ClusterID, features structs.ClusterFeatures) error {
	if bkr.state.ClusterExists(instanceID) == false {
		return fmt.Errorf("Service instance %s doesn't exist", instanceID)
	}

	return bkr.scheduler.VerifyClusterFeatures(features)
}
