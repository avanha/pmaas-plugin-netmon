package netinterface

import (
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/avanha/pmaas-plugin-netmon/entities"
	"github.com/avanha/pmaas-spi/common"
	"github.com/avanha/pmaas-spi/tracking"
)

type stub struct {
	log                    *slog.Logger
	id                     string
	closeFn                func() error
	entityWrapperReference atomic.Pointer[common.ThreadSafeEntityWrapper[entities.NetworkInterface]]
}

func newNetInterfaceStub(
	log *slog.Logger,
	id string,
	entityWrapper *common.ThreadSafeEntityWrapper[entities.NetworkInterface]) *stub {
	instance := &stub{
		log: log,
		id:  id,
	}

	instance.entityWrapperReference.Store(entityWrapper)

	instance.closeFn = func() error {
		if instance.entityWrapperReference.CompareAndSwap(entityWrapper, nil) {
			instance.closeFn = nil
			return nil
		}

		return fmt.Errorf("failed to clear entity wrapper, current value does not match expected value")
	}

	return instance
}

func (s *stub) close() {
	closeFn := s.closeFn

	if closeFn == nil {
		return
	}

	err := closeFn()

	if err != nil {
		s.log.Error("failed to close netinterface stub", "id", s.id, "error", err)
	}
}

func (s *stub) Data() tracking.DataSample {
	return common.ThreadSafeEntityWrapperExecValueFunc(
		s.entityWrapperReference.Load(),
		func(target entities.NetworkInterface) tracking.DataSample { return target.Data() })
}

func (s *stub) TrackingConfig() tracking.Config {
	return common.ThreadSafeEntityWrapperExecValueFunc(
		s.entityWrapperReference.Load(),
		func(target entities.NetworkInterface) tracking.Config { return target.TrackingConfig() })
}
