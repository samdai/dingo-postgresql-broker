package broker

import (
	"fmt"
	"reflect"

	"github.com/samdai/dingo-postgresql-broker/broker/structs"
	"github.com/samdai/dingo-postgresql-broker/state"
	"github.com/frodenas/brokerapi"
	"github.com/pivotal-golang/lager"
)

// Provision a new service instance
func (bkr *Broker) Provision(instanceID string, details brokerapi.ProvisionDetails, acceptsIncomplete bool) (resp brokerapi.ProvisioningResponse, async bool, err error) {
	return bkr.provision(structs.ClusterID(instanceID), details, acceptsIncomplete)
}

func (bkr *Broker) provision(instanceID structs.ClusterID, details brokerapi.ProvisionDetails, acceptsIncomplete bool) (resp brokerapi.ProvisioningResponse, async bool, err error) {
	if details.ServiceID == "" && details.PlanID == "" {
		return bkr.Recreate(instanceID, details, acceptsIncomplete)
	}

	logger := bkr.newLoggingSession("provision", lager.Data{"instance-id": instanceID})
	defer logger.Info("done")

	features, err := structs.ClusterFeaturesFromParameters(details.Parameters)
	if err != nil {
		logger.Error("cluster-features", err)
		return resp, false, err
	}

	if err = bkr.assertProvisionPrecondition(instanceID, features); err != nil {
		logger.Error("preconditions.error", err)
		return resp, false, err
	}

	port, err := bkr.router.AllocatePort()
	clusterState := bkr.initCluster(instanceID, port, details)
	clusterModel := state.NewClusterModel(bkr.state, clusterState)

	clusterModel.SchedulingMessage("Initializing...")

	if bkr.callbacks.Configured() {
		logger.Info("recreation-data.writing")
		bkr.callbacks.WriteRecreationData(clusterState.RecreationData())
		logger.Info("recreation-data.restoring")
		data, err := bkr.callbacks.RestoreRecreationData(instanceID)
		if err != nil {
			logger.Error("recreation-data.save-failure.error", err)
			return resp, false, err
		}
		if !reflect.DeepEqual(clusterState.RecreationData(), data) {
			err = fmt.Errorf("Cluster recreation data was not saved successfully")
			logger.Error("recreation-data.save-failure.deep-equal", err)
			return resp, false, err
		}
		logger.Info("recreation-data.success")
	}

	var existingClusterData *structs.ClusterRecreationData
	if features.CloneFromServiceName != "" {
		if bkr.backups.BaseURI == "" {
			return resp, false, fmt.Errorf("Broker missing configuration backups.base_uri to support 'clone-from' feature")
		}

		// Confirm that backup can be found before continuing asynchronously
		logger.Info("lookup-service-name.start")
		existingClusterData, err = bkr.lookupClusterDataBackupByServiceInstanceName(details.SpaceGUID, features.CloneFromServiceName, logger)
		if err != nil {
			logger.Error("lookup-service-name.error", err)
			return resp, false, err
		}
		logger.Info("lookup-service-name.success")
	}

	// Continue processing in background
	go func() {
		logger.Info("async-begin")
		defer logger.Info("async-complete")

		if existingClusterData != nil {
			clusterModel.SchedulingMessage(fmt.Sprintf("Cloning existing database %s", existingClusterData.ServiceInstanceName))
			if err := bkr.prepopulateDatabaseFromExistingClusterData(existingClusterData, instanceID, clusterModel, logger); err != nil {
				logger.Error("pre-populate-cluster", err)
				clusterModel.SchedulingError(fmt.Errorf("Unsuccessful pre-populating database from backup. Please contact administrator: %s", err.Error()))
				return
			}
		}

		if err := bkr.scheduler.RunCluster(clusterModel, features); err != nil {
			logger.Error("run-cluster", err)
			return
		}

		if err := bkr.router.AssignPortToCluster(instanceID, port); err != nil {
			logger.Error("assign-port", err)
			clusterModel.SchedulingError(fmt.Errorf("Unsuccessful mapping database to routing mesh. Please contact administrator: %s", err.Error()))
			return
		}

		bkr.fetchAndBackupServiceInstanceName(instanceID, &clusterState, logger)
	}()
	return resp, true, err
}

func (bkr *Broker) initCluster(instanceID structs.ClusterID, port int, details brokerapi.ProvisionDetails) structs.ClusterState {
	return structs.ClusterState{
		InstanceID:       instanceID,
		OrganizationGUID: details.OrganizationGUID,
		PlanID:           details.PlanID,
		ServiceID:        details.ServiceID,
		SpaceGUID:        details.SpaceGUID,
		AllocatedPort:    port,
		AdminCredentials: structs.PostgresCredentials{
			Username: "pgadmin",
			Password: NewPassword(16),
		},
		SuperuserCredentials: structs.PostgresCredentials{
			Username: "postgres",
			Password: NewPassword(16),
		},
		AppCredentials: structs.PostgresCredentials{
			Username: "appuser",
			Password: NewPassword(16),
		},
	}
}

func (bkr *Broker) assertProvisionPrecondition(instanceID structs.ClusterID, features structs.ClusterFeatures) error {
	if bkr.state.ClusterExists(instanceID) {
		return fmt.Errorf("service instance %s already exists", instanceID)
	}

	return bkr.scheduler.VerifyClusterFeatures(features)
}

// If broker has credentials for a Cloud Foundry,
// attempt to look up service instance to get its user-provided name.
// This can then be used in future to undo/recreate-from-backup when user
// only knows the name they provided; and not the internal service instance ID.
// If operation fails, that's temporarily unfortunate but might be due to credentials
// not yet having SpaceDeveloper role for the Space being used.
func (bkr *Broker) fetchAndBackupServiceInstanceName(instanceID structs.ClusterID, clusterState *structs.ClusterState, logger lager.Logger) {
	if bkr.callbacks.Configured() {
		serviceInstanceName, err := bkr.cf.LookupServiceName(instanceID)
		if err != nil {
			logger.Error("backup-name.error", err,
				lager.Data{"action-required": "Fix issue and run errand/script to update clusterdata backups to include service names"})
		}
		if serviceInstanceName == "" {
			logger.Info("backup-name.not-found")
		} else {
			clusterState.ServiceInstanceName = serviceInstanceName
			bkr.callbacks.WriteRecreationData(clusterState.RecreationData())
			data, err := bkr.callbacks.RestoreRecreationData(instanceID)
			if !reflect.DeepEqual(clusterState.RecreationData(), data) {
				logger.Error("backup-name.update-recreation-data.failure", err)
			} else {
				logger.Info("backup-name.update-recreation-data.saved", lager.Data{"name": serviceInstanceName})
			}
		}
	}
}

// Find clusterdata backup information for a space_guid/service_name, else error
func (bkr *Broker) lookupClusterDataBackupByServiceInstanceName(spaceGUID, name string, logger lager.Logger) (data *structs.ClusterRecreationData, err error) {
	if bkr.callbacks.Configured() {
		data, err = bkr.callbacks.ClusterDataFindServiceInstanceByName(spaceGUID, name)
		if err != nil {
			logger.Error("lookup-clusterdata-backup", err)
			return nil, fmt.Errorf("Could not locate backup for service instance '%s'", name)
		}
	} else {
		return nil, fmt.Errorf("Dingo PostgreSQL broker is not configured for this operation")
	}
	return
}

// If requested to pre-populate database from a backup of previous/existing database
// and updates clusterState with DB credentials
func (bkr *Broker) prepopulateDatabaseFromExistingClusterData(existingClusterData *structs.ClusterRecreationData, toInstanceID structs.ClusterID, clusterModel *state.ClusterModel, logger lager.Logger) (err error) {
	if bkr.backups.BaseURI == "" {
		return fmt.Errorf("Failed to copy existing database backup: missing backups.base_uri configuration")
	}
	fromDatabaseBackup := fmt.Sprintf("%s/%s", bkr.backups.BaseURI, existingClusterData.InstanceID)
	toDatabaseBackup := fmt.Sprintf("%s/%s", bkr.backups.BaseURI, toInstanceID)
	err = bkr.callbacks.CopyDatabaseBackup(fromDatabaseBackup, toDatabaseBackup, bkr.logger)
	if err != nil {
		logger.Error("prepopulate-database", err)
		return fmt.Errorf("Failed to copy existing database backup to new database: %s", err.Error())
	}

	return clusterModel.UpdateCredentials(existingClusterData)
}
