package netmon

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/avanha/pmaas-plugin-netmon/config"
	"github.com/avanha/pmaas-plugin-netmon/data"
	"github.com/avanha/pmaas-plugin-netmon/entities"
	"github.com/avanha/pmaas-plugin-netmon/internal/common"
	"github.com/avanha/pmaas-plugin-netmon/internal/host"
	"github.com/avanha/pmaas-plugin-netmon/internal/http"
	"github.com/avanha/pmaas-plugin-netmon/internal/monitoring"
	"github.com/avanha/pmaas-plugin-netmon/internal/netinterface"
	"github.com/avanha/pmaas-spi"
	"github.com/avanha/pmaas-spi/tracking"
)

type plugin struct {
	config        config.PluginConfig
	container     spi.IPMAASContainer
	ctx           context.Context
	cancel        context.CancelFunc
	monitors      sync.WaitGroup
	hosts         []*host.Host
	entityCounter int
	httpHandler   *http.Handler
}

type Plugin interface {
	spi.IPMAASPlugin2
}

func NewPlugin(config config.PluginConfig) Plugin {
	fmt.Printf("New, config: %v\n", config)
	instance := &plugin{
		config:      config,
		httpHandler: http.NewHandler(),
	}

	return instance
}

func (p *plugin) Init(container spi.IPMAASContainer) {
	p.container = container
	p.processConfig()
	p.httpHandler.Init(p.container, &entityStoreAdapter{parent: p})
}

func (p *plugin) Start() {
	fmt.Printf("%T Starting...\n", p)
	p.registerEntities()
	p.startMonitoringGoRoutines()
}

func (p *plugin) Stop() {}

func (p *plugin) StopAsync() chan func() {
	fmt.Printf("%T Stopping...\n", p)
	p.cancel()

	// We don't want to block on the pluginRunner goroutine, so start a new goroutine to wait
	// on the WaitGroup and issue a callback once that's done.
	stopEvents := make(chan func())
	go func() {
		p.monitors.Wait()
		stopEvents <- p.onMonitoringGoRoutinesStopped
		close(stopEvents)
	}()
	return stopEvents
}

func (p *plugin) processConfig() {
	defaultTrackingConfig := tracking.Config{
		TrackingMode:        tracking.ModePoll,
		PollIntervalSeconds: 60,
	}
	for _, configuredHost := range p.config.Hosts {
		hostInstance := host.NewHost(
			fmt.Sprintf("Host_%v", p.nextEntityId()),
			configuredHost)
		p.hosts = append(p.hosts, hostInstance)
		for key, configuredNetInterface := range configuredHost.NetInterfaces {
			trackingConfig := defaultTrackingConfig.Clone()
			trackingConfig.Name = strings.Replace(
				fmt.Sprintf("host_%s_if_%s",
					hostInstance.Name(), configuredNetInterface.TrackingName()),
				"-", "_", -1)
			trackingConfig.Schema = tracking.Schema{
				DataStructType:     data.NetInterfaceDataType,
				InsertArgFactoryFn: data.NetInterfaceDataToInsertArgs,
			}
			netInterfaceInstance := netinterface.CreateNetInterface(
				hostInstance.Id(),
				fmt.Sprintf("NetworkInterface_%v", p.nextEntityId()),
				trackingConfig,
				*configuredNetInterface)
			hostInstance.AddNetInterface(key, netInterfaceInstance)
		}
	}
}

func (p *plugin) onMonitoringGoRoutinesStopped() {
	fmt.Printf("%T Monitoring goroutines stopped, deregistering entities\n", p)
	p.deregisterEntities()
	fmt.Printf("%T Stopped\n", p)
}

func (p *plugin) nextEntityId() int {
	p.entityCounter = p.entityCounter + 1
	return p.entityCounter
}

func (p *plugin) startMonitoringGoRoutines() {
	updateHostFunction := func(hostInstance *host.Host, data common.HostData) {
		err := p.container.EnqueueOnPluginGoRoutine(func() {
			p.updateHost(hostInstance, &data)
		})

		if err != nil {
			fmt.Printf("Error enqueuing updateHost callback: %s\n", err)
		}
	}

	p.monitors = sync.WaitGroup{}
	p.ctx, p.cancel = context.WithCancel(context.Background())

	for _, hostInstance := range p.hosts {
		monitoringTask := monitoring.CreateTask(p.ctx, hostInstance, updateHostFunction)
		p.monitors.Go(monitoringTask.Run)
	}
}

func (p *plugin) registerEntities() {
	for _, hostInstance := range p.hosts {
		hostName := hostInstance.Name()
		hostPmaasId, err := p.container.RegisterEntity(hostInstance.Id(), entities.HostType, hostName, nil)

		if err != nil {
			fmt.Printf("Error registering %s: %s\n", err)
			continue
		}

		hostInstance.SetPmaasEntityId(hostPmaasId)

		for networkInterfaceKey, networkInterfaceInstance := range hostInstance.NetInterfaces() {
			networkInterfaceInstance.SetHostPmaasEntityId(hostPmaasId)
			networkInterfaceName := getInterfaceName(hostInstance, networkInterfaceKey)

			// This lambda captures the plugin instance and the networkInterfaceInstance
			// and passes it to the entity manager.  However, entities are deregistered on plugin
			// stop, so this is OK.
			var stubFactoryFn spi.EntityStubFactoryFunc = func() (any, error) {
				return networkInterfaceInstance.GetStub(p.container), nil
			}
			interfacePmaasId, err := p.container.RegisterEntity(
				networkInterfaceInstance.Id(),
				entities.NetworkInterfaceType,
				networkInterfaceName,
				stubFactoryFn)

			if err != nil {
				fmt.Printf("Error registering %s: %s\n", networkInterfaceName, err)
				continue
			}

			networkInterfaceInstance.SetPmaasEntityId(interfacePmaasId)
			networkInterfaceInstance.RegisterConfiguredListeners(p.container)
		}
	}
}

func (p *plugin) deregisterEntities() {
	for _, hostInstance := range p.hosts {
		for networkInterfaceKey, networkInterfaceInstance := range hostInstance.NetInterfaces() {
			networkInterfaceInstance.DeregisterConfiguredListeners(p.container)
			err := p.container.DeregisterEntity(networkInterfaceInstance.PmaasEntityId())

			if err == nil {
				networkInterfaceInstance.ClearPmaasEntityId()
			} else {
				fmt.Printf("Error deregistering %s: %s\n",
					getInterfaceName(hostInstance, networkInterfaceKey), err)
			}

			networkInterfaceInstance.CloseStubIfPresent()
		}

		err := p.container.DeregisterEntity(hostInstance.PmaasEntityId())

		if err == nil {
			hostInstance.ClearPmaasEntityId()
		} else {
			fmt.Printf("Error deregistering %s: %s\n", hostInstance.Name(), err)
		}

	}
}

func getInterfaceName(hostInstance *host.Host, networkInterfaceKey string) string {
	return fmt.Sprintf("host_%s_interface_%s", hostInstance.Name(), networkInterfaceKey)
}

func (p *plugin) updateHost(hostInstance *host.Host, data *common.HostData) {
	fmt.Printf("updateHost %s with %v\n", hostInstance.Name(), data)
	events := make([]any, 0, 10)
	hostInstance.Update(data, &events)

	// Broadcast accumulated events
	for _, event := range events {
		p.broadcastEvent(hostInstance.PmaasEntityId(), event)
	}
}

func (p *plugin) broadcastEvent(sourceEntityId string, event any) {
	err := p.container.BroadcastEvent(sourceEntityId, event)
	if err != nil {
		fmt.Printf("%T Error broadcasting event %V: %V", p, event, err)
	}
}

func (p *plugin) getStatusAndEntities() common.StatusAndEntities {
	hostData := make([]data.HostData, len(p.hosts))

	for i := 0; i < len(p.hosts); i++ {
		hostData[i] = p.hosts[i].Data()
		hostData[i].NetInterfaceDataList = make([]data.NetInterfaceData, 0)

		for _, netInterface := range p.hosts[i].NetInterfaces() {
			hostData[i].NetInterfaceDataList = append(hostData[i].NetInterfaceDataList, netInterface.InterfaceData())
		}
	}

	return common.StatusAndEntities{
		Hosts: hostData,
	}
}
