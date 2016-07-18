package broker

import (
	"fmt"
	"net/http"
	"os"

	"github.com/dingotiles/dingo-postgresql-broker/broker/interfaces"
	"github.com/dingotiles/dingo-postgresql-broker/config"
	"github.com/dingotiles/dingo-postgresql-broker/patroni"
	"github.com/dingotiles/dingo-postgresql-broker/routing"
	"github.com/dingotiles/dingo-postgresql-broker/scheduler"
	"github.com/dingotiles/dingo-postgresql-broker/state"
	"github.com/frodenas/brokerapi"
	"github.com/pivotal-golang/lager"
)

// Broker is the core struct for the Broker webapp
type Broker struct {
	config  config.Broker
	catalog brokerapi.Catalog

	logger lager.Logger
	cells  []*config.Cell

	callbacks *Callbacks

	router    interfaces.Router
	scheduler interfaces.Scheduler
	state     interfaces.State
	patroni   interfaces.Patroni
}

// NewBroker is a constructor for a Broker webapp struct
func NewBroker(config *config.Config) (*Broker, error) {
	bkr := &Broker{
		config:  config.Broker,
		catalog: config.Catalog,
		cells:   config.Scheduler.Cells,
	}

	bkr.logger = bkr.setupLogger()
	bkr.callbacks = NewCallbacks(config.Callbacks, bkr.logger)
	var err error
	bkr.scheduler, err = scheduler.NewScheduler(config.Scheduler, bkr.logger)
	if err != nil {
		bkr.logger.Error("new-broker.new-scheduler.error", err)
		return nil, err
	}
	bkr.state, err = state.NewStateEtcd(config.Etcd, bkr.logger)
	if err != nil {
		bkr.logger.Error("new-broker.new-state.error", err)
		return nil, err
	}

	bkr.patroni, err = patroni.NewPatroni(config.Etcd, bkr.logger)
	if err != nil {
		bkr.logger.Error("new-broker.new-patroni.error", err)
		return nil, err
	}

	bkr.router, err = routing.NewRouter(config.Etcd, bkr.logger)
	if err != nil {
		bkr.logger.Error("new-broker.new-router.error", err)
		return nil, err
	}

	return bkr, nil
}

// Run starts the Martini webapp handler
func (bkr *Broker) Run() {
	credentials := brokerapi.BrokerCredentials{
		Username: bkr.config.Username,
		Password: bkr.config.Password,
	}
	port := bkr.config.Port

	brokerAPI := brokerapi.New(bkr, bkr.logger, credentials)
	http.Handle("/v2/", brokerAPI)

	adminAPI := NewAdminAPI(bkr, bkr.logger, credentials)
	http.Handle("/admin/", adminAPI)

	bkr.logger.Fatal("http-listen", http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", port), nil))
}

func (bkr *Broker) setupLogger() lager.Logger {
	logger := lager.NewLogger("dingo-postgresql-broker")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, lager.DEBUG))
	logger.RegisterSink(lager.NewWriterSink(os.Stderr, lager.ERROR))
	return logger
}

func (bkr *Broker) newLoggingSession(action string, data lager.Data) lager.Logger {
	logger := bkr.logger.Session(action, data)
	logger.Info("start")
	return logger
}

func (bkr *Broker) Cells() []*config.Cell {
	return bkr.cells
}
