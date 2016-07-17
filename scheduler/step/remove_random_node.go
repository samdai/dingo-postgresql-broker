package step

import (
	"fmt"
	"math/rand"

	"github.com/dingotiles/dingo-postgresql-broker/broker/structs"
	"github.com/dingotiles/dingo-postgresql-broker/scheduler/cells"
	"github.com/dingotiles/dingo-postgresql-broker/state"
	"github.com/pivotal-golang/lager"
)

// RemoveRandomNode instructs cluster to delete a node, starting with replicas
type RemoveRandomNode struct {
	clusterModel *state.ClusterModel
	cells        cells.Cells
	logger       lager.Logger
}

// NewStepRemoveRandomNode creates a StepRemoveRandomNode command
func NewStepRemoveRandomNode(clusterModel *state.ClusterModel, cells cells.Cells, logger lager.Logger) Step {
	return RemoveRandomNode{clusterModel: clusterModel, cells: cells, logger: logger}
}

// StepType prints the type of step
func (step RemoveRandomNode) StepType() string {
	return "RemoveRandomNode"
}

// Perform runs the Step action to modify the Cluster
func (step RemoveRandomNode) Perform() (err error) {
	logger := step.logger

	// 1. Get list of replicas and pick a random one; else pick a random master
	nodes := step.clusterModel.Nodes()
	nodeToRemove := randomReplicaNode(nodes)

	cell := step.cells.Get(nodeToRemove.CellGUID)
	if cell == nil {
		err = fmt.Errorf("Internal error: node assigned to a cell that no longer exists (%s)", nodeToRemove.CellGUID)
		logger.Error("remove-random-node.perform", err)
		return
	}

	logger.Info("remove-random-node.perform", lager.Data{
		"instance-id": step.clusterModel.InstanceID(),
		"node-uuid":   nodeToRemove.ID,
		"cell":        cell.GUID,
	})

	err = cell.DeprovisionNode(nodeToRemove, logger)
	if err != nil {
		return nil
	}

	err = step.clusterModel.RemoveNode(nodeToRemove)
	if err != nil {
		logger.Error("remove-random-node.nodes-delete", err)
	}
	return
}

// currently random any node, doesn't have to be a replica
func randomReplicaNode(nodes []*structs.Node) *structs.Node {
	n := rand.Intn(len(nodes))
	return nodes[n]
}
