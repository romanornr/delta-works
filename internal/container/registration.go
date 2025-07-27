package container

import (
	"reflect"

	"github.com/romanornr/delta-works/internal/contracts"
	"github.com/romanornr/delta-works/internal/services"
	"github.com/thrasher-corp/gocryptotrader/engine"
)

type serviceRegistration struct {
	container *ServiceContainer
}

func NewServiceRegistration(container *ServiceContainer) *serviceRegistration {
	return &serviceRegistration{container: container}
}

func (sr *serviceRegistration) RegisterAllServices(settings *engine.Settings, flagset map[string]bool, questDBConfig string) error {
	// Register logger as shared resource
	loggerType := reflect.TypeOf((*contracts.Logger)(nil)).Elem()
	sr.container.RegisterSharedResource(loggerType, func(sc *ServiceContainer) (interface{}, error) {
		return NewDefaultLogger(), nil
	})

	// Register repository as shared resource
	repoType := reflect.TypeOf((*contracts.RepositoryService)(nil)).Elem()
	sr.container.RegisterSharedResource(repoType, func(sc *ServiceContainer) (interface{}, error) {
		logger, err := sc.Get(loggerType)
		if err != nil {
			return nil, err
		}
		return services.NewRepositoryService(questDBConfig, logger.(contracts.Logger))
	})

	// Register engine as shared resource
	engineType := reflect.TypeOf((*contracts.EngineService)(nil)).Elem()
	sr.container.RegisterSharedResource(engineType, func(sc *ServiceContainer) (interface{}, error) {
		logger, err := sc.Get(loggerType)
		if err != nil {
			return nil, err
		}
		return services.NewEngineService(settings, flagset, logger.(contracts.Logger))
	})

	// Register holdings service as a shared resource
	holdingsType := reflect.TypeOf((*contracts.HoldingsService)(nil)).Elem()
	sr.container.RegisterSharedResource(holdingsType, func(sc *ServiceContainer) (interface{}, error) {
		engineService, err := sc.Get(engineType)
		if err != nil {
			return nil, err
		}
		repoService, err := sc.Get(repoType)
		if err != nil {
			return nil, err
		}
		logger, err := sc.Get(loggerType)
		if err != nil {
			return nil, err
		}
		return services.NewHoldingsService(engineService.(contracts.EngineService), repoService.(contracts.RepositoryService), logger.(contracts.Logger)), nil
	})

	// Register withdrawal service as a shard resource
	withdrawalType := reflect.TypeOf((*contracts.WithdrawalService)(nil)).Elem()
	sr.container.RegisterSharedResource(withdrawalType, func(sc *ServiceContainer) (interface{}, error) {
		engineService, err := sc.Get(engineType)
		if err != nil {
			return nil, err
		}
		repoService, err := sc.Get(repoType)
		if err != nil {
			return nil, err
		}
		logger, err := sc.Get(loggerType)
		if err != nil {
			return nil, err
		}
		return services.NewWithdrawalService(engineService.(contracts.EngineService), repoService.(contracts.RepositoryService), logger.(contracts.Logger)), nil
	})

	return nil
}
