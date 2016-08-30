package cells

import "github.com/samdai/dingo-postgresql-broker/broker/structs"

type ClusterLoader interface {
	LoadAllRunningClusters() ([]*structs.ClusterState, error)
}
