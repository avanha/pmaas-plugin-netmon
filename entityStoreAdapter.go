package netmon

import (
	"github.com/avanha/pmaas-plugin-netmon/internal/common"
	"github.com/avanha/pmaas-spi"
)

type entityStoreAdapter struct {
	parent *plugin
}

func (esa *entityStoreAdapter) GetStatusAndEntities() (common.StatusAndEntities, error) {
	return spi.ExecValueFunctionOnPluginGoRoutine(
		esa.parent.container,
		esa.parent.getStatusAndEntities,
		func() common.StatusAndEntities { return common.StatusAndEntities{} },
		"unable to get status and entities")
}
