package backend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/dingotiles/dingo-postgresql-broker/config"
	"github.com/dingotiles/dingo-postgresql-broker/state"
	"github.com/frodenas/brokerapi"
	"github.com/pborman/uuid"
	"github.com/pivotal-golang/lager"
)

type Backend struct {
	Id               string
	URI              string
	Config           *config.Backend
	AvailabilityZone string
}

type Backends []*Backend

func NewBackend(config *config.Backend) *Backend {
	return &Backend{
		Id:               config.GUID,
		Config:           config,
		AvailabilityZone: config.AvailabilityZone,
		URI:              config.URI,
	}
}

func (b Backends) AllAvailabilityZones() []string {
	azMap := map[string]string{}
	for _, backend := range b {
		azMap[backend.Config.AvailabilityZone] = backend.Config.AvailabilityZone
	}

	keys := make([]string, 0, len(azMap))
	for k := range azMap {
		keys = append(keys, k)
	}
	return keys
}

func (b Backends) AvailabilityZone(backendId string) (string, error) {
	for _, backend := range b {
		if backend.Id == backendId {
			return backend.AvailabilityZone, nil
		}
	}
	return "", errors.New(fmt.Sprintf("No backend with Id %s found", backendId))
}

func (b *Backend) ProvisionNode(clusterData state.ClusterData, logger lager.Logger) (node state.Node, err error) {
	node = state.Node{Id: uuid.New(), BackendId: b.Id}
	provisionDetails := brokerapi.ProvisionDetails{
		OrganizationGUID: clusterData.OrganizationGUID,
		PlanID:           clusterData.PlanID,
		ServiceID:        clusterData.ServiceID,
		SpaceGUID:        clusterData.SpaceGUID,
		Parameters: map[string]interface{}{
			"PATRONI_SCOPE":     clusterData.InstanceID,
			"NODE_NAME":         node.Id,
			"POSTGRES_USERNAME": clusterData.AdminCredentials.Username,
			"POSTGRES_PASSWORD": clusterData.AdminCredentials.Password,
		},
	}

	url := fmt.Sprintf("%s/v2/service_instances/%s", b.Config.URI, node.Id)
	client := &http.Client{}
	buffer := &bytes.Buffer{}

	if err = json.NewEncoder(buffer).Encode(provisionDetails); err != nil {
		logger.Error("request-node.backend-provision-encode-details", err)
		return
	}
	req, err := http.NewRequest("PUT", url, buffer)
	if err != nil {
		logger.Error("request-node.backend-provision-req", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(b.Config.Username, b.Config.Password)

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("request-node.backend-provision-resp", err)
		return
	}
	defer resp.Body.Close()

	// FIXME: If resp.StatusCode not 200 or 201, then try next
	if resp.StatusCode >= 400 {
		// FIXME: allow return of this error to end user
		return state.Node{}, errors.New("unknown plan")
	}
	return
}

func (b *Backend) DeprovisionNode(node state.Node, logger lager.Logger) (err error) {
	url := fmt.Sprintf("%s/v2/service_instances/%s", b.URI, node.Id)
	client := &http.Client{}
	buffer := &bytes.Buffer{}

	deleteDetails := brokerapi.DeprovisionDetails{
		PlanID:    node.PlanId,
		ServiceID: node.ServiceId,
	}

	if err = json.NewEncoder(buffer).Encode(deleteDetails); err != nil {
		logger.Error("remove-node.backend.encode", err)
		return err
	}
	req, err := http.NewRequest("DELETE", url, buffer)
	if err != nil {
		logger.Error("remove-node.backend.new-req", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(b.Config.Username, b.Config.Password)

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("remove-node.backend.do", err)
		return err
	}
	defer resp.Body.Close()

	return
}