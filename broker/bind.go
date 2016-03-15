package broker

import (
	"fmt"
	"strconv"

	"github.com/dingotiles/patroni-broker/serviceinstance"
	"github.com/frodenas/brokerapi"
)

// CredentialsHash represents the set of binding credentials returned
type CredentialsHash struct {
	Host              string `json:"host,omitempty"`
	Port              int64  `json:"port,omitempty"`
	Name              string `json:"name,omitempty"`
	Username          string `json:"username,omitempty"`
	Password          string `json:"password,omitempty"`
	URI               string `json:"uri,omitempty"`
	JDBCURI           string `json:"jdbcUrl,omitempty"`
	SuperuserUsername string `json:"superuser_username,omitempty"`
	SuperuserPassword string `json:"superuser_password,omitempty"`
	SuperuserURI      string `json:"superuser_uri,omitempty"`
	SuperuserJDBCURI  string `json:"superuser_jdbcUrl,omitempty"`
}

// Bind returns access credentials for a service instance
func (bkr *Broker) Bind(instanceID string, bindingID string, details brokerapi.BindDetails) (brokerapi.BindingResponse, error) {
	cluster := serviceinstance.NewCluster(instanceID, brokerapi.ProvisionDetails{}, bkr.EtcdClient, bkr.Config, bkr.Logger)

	key := fmt.Sprintf("/routing/allocation/%s", cluster.InstanceID)
	resp, err := cluster.EtcdClient.Get(key, false, false)
	if err != nil {
		bkr.Logger.Error("bind.routing-allocation.get", err)
		return brokerapi.BindingResponse{}, fmt.Errorf("Internal error: no published port for provisioned cluster")
	}
	publicPort, err := strconv.ParseInt(resp.Node.Value, 10, 64)
	if err != nil {
		bkr.Logger.Error("bind.routing-allocation.parse-int", err)
		return brokerapi.BindingResponse{}, fmt.Errorf("Internal error: published port is not an integer (%s)", resp.Node.Value)
	}

	routerHost := bkr.Config.Router.Hostname
	appUsername := "replicator"
	appPassword := "replicator"
	uri := fmt.Sprintf("postgres://%s:%s@%s:%d/postgres", appUsername, appPassword, routerHost, publicPort)
	jdbc := fmt.Sprintf("jdbc:postgresql://%s:%d/postgres?username=%s&password=%s", routerHost, publicPort, appUsername, appPassword)
	superuserUsername := "postgres"
	superuserPassword := "starkandwayne"
	superuserURI := fmt.Sprintf("postgres://%s:%s@%s:%d/postgres", superuserUsername, superuserPassword, routerHost, publicPort)
	superuserJDBCURI := fmt.Sprintf("jdbc:postgresql://%s:%d/postgres?username=%s&password=%s", routerHost, publicPort, superuserUsername, superuserPassword)
	return brokerapi.BindingResponse{
		Credentials: CredentialsHash{
			Host:              routerHost,
			Port:              publicPort,
			Username:          appUsername,
			Password:          appPassword,
			URI:               uri,
			JDBCURI:           jdbc,
			SuperuserUsername: superuserUsername,
			SuperuserPassword: superuserPassword,
			SuperuserURI:      superuserURI,
			SuperuserJDBCURI:  superuserJDBCURI,
		},
	}, nil
}
