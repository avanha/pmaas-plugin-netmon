package netmon

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/avanha/pmaas-plugin-netmon/config"
	"github.com/avanha/pmaas-plugin-netmon/data"
	"github.com/avanha/pmaas-plugin-netmon/entities"
	"github.com/avanha/pmaas-plugin-netmon/internal/common"
	"github.com/avanha/pmaas-plugin-netmon/internal/host"
	"github.com/avanha/pmaas-plugin-netmon/internal/http"
	"github.com/avanha/pmaas-plugin-netmon/internal/monitoring"
	"github.com/avanha/pmaas-plugin-netmon/internal/netinterface"
	"github.com/avanha/pmaas-spi"
	"github.com/avanha/pmaas-spi/alert"
	"github.com/avanha/pmaas-spi/environment"
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

	// watches hold the alert for each host that is to have one, by host name, and needs the name of the host each waits for
	// to be reachable, for those that do. Owned by the plugin goroutine.
	watches map[string]*alert.OfflineWatch
	needs   map[string]string
}

func NewPluginConfig() config.PluginConfig {
	return config.PluginConfig{}
}

type Plugin interface {
	spi.IPMAASPlugin
}

func NewPlugin(config config.PluginConfig) Plugin {
	fmt.Printf("New, config: %v\n", config)
	instance := &plugin{
		config:      config,
		httpHandler: http.NewHandler(),
		watches:     make(map[string]*alert.OfflineWatch),
		needs:       make(map[string]string),
	}

	return instance
}

func (p *plugin) ShortName() string {
	return "netmon"
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

func (p *plugin) Stop() chan func() {
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
		hostTrackingConfig := defaultTrackingConfig.Clone()
		hostTrackingConfig.Name = fmt.Sprintf(
			"host_%s",
			strings.Replace(configuredHost.Name, "-", "_", -1))
		hostTrackingConfig.Schema = tracking.Schema{
			DataStructType:     data.HostDataType,
			InsertArgFactoryFn: data.HostDataToInsertArgs,
		}
		hostInstance := host.NewHost(
			fmt.Sprintf("Host_%v", p.nextEntityId()),
			*configuredHost, hostTrackingConfig, p.container)

		if after, severity, wanted := configuredHost.UnreachableAlert(); wanted {
			p.watches[configuredHost.Name] = p.unreachableWatch(configuredHost, after, severity)

			if other := configuredHost.AlertDependency(); other != nil {
				if !slices.Contains(p.config.Hosts, other) {
					panic(fmt.Sprintf("%T: the alert for host %q waits for host %q, which is not one of the plugin's hosts",
						p, configuredHost.Name, other.Name))
				}

				p.needs[configuredHost.Name] = other.Name
			}
		}

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

		// This lambda captures the plugin instance and the hostInstance
		// and passes it to the entity manager.  However, entities are deregistered on plugin
		// stop, so this is OK.
		var hostStubFactoryFn spi.EntityStubFactoryFunc = func() (any, error) {
			return hostInstance.GetStub(p.container), nil
		}
		hostPmaasId, err := p.container.RegisterEntity(
			hostInstance.Id(),
			entities.HostType,
			hostName,
			hostStubFactoryFn)

		if err != nil {
			fmt.Printf("Error registering %s: %v\n", hostName, err)
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

		hostInstance.CloseStubIfPresent()
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

	p.checkUnreachableAlert(hostInstance, data.LastUpdateTime)
}

// unreachableWatch holds the alert about a host being unreachable: the shared offline alert, worded for a host.
func (p *plugin) unreachableWatch(configuredHost *config.Host, after time.Duration, severity alert.SeverityFunc) *alert.OfflineWatch {
	name, address := configuredHost.Name, configuredHost.IpAddress

	return alert.WhenOffline(after, severity).
		Watch(p.ShortName(), "host-unreachable:"+name, name).
		WithText(func(since time.Time, down time.Duration) (string, string) {
			return "Host unreachable: " + name,
				fmt.Sprintf("%s (%s) has not been reachable since %s, %s ago.",
					name, address, since.Format("2006-01-02 3:04 PM"), down.Round(time.Second))
		})
}

// reachableSince is when the named host last became reachable, if it is reachable now.
func (p *plugin) reachableSince(name string) (time.Time, bool) {
	for _, h := range p.hosts {
		if h.Name() != name {
			continue
		}

		hostData := h.HostData()

		return hostData.LastReachabilityChangeTime, hostData.Reachability == data.ReachabilityReachable
	}

	return time.Time{}, false
}

// checkUnreachableAlert raises, escalates or clears the alert about a host being unreachable, if there is to be one, after
// a poll made at the given time. Runs on the plugin goroutine.
func (p *plugin) checkUnreachableAlert(hostInstance *host.Host, pollTime time.Time) {
	watch := p.watches[hostInstance.Name()]
	if watch == nil {
		return
	}

	hostData := hostInstance.HostData()
	since := hostData.LastUnreachableStartTime

	connectivity := environment.ConnectivityUnknown

	switch hostData.Reachability {
	case data.ReachabilityReachable:
		connectivity = environment.ConnectivityOnline
	case data.ReachabilityUnreachable:
		connectivity = environment.ConnectivityOffline
	}

	if needed, waits := p.needs[hostInstance.Name()]; waits && connectivity == environment.ConnectivityOffline {
		reachableSince, reachable := p.reachableSince(needed)
		if !reachable {
			// Not known to be up: whatever is wrong with this host is no news, so the alert is left as it is.
			connectivity = environment.ConnectivityUnknown
		} else if reachableSince.After(since) {
			since = reachableSince
		}
	}

	if err := watch.Check(p.container, connectivity, since, pollTime); err != nil {
		fmt.Printf("%T unable to update the alert for %s: %v\n", p, hostInstance.Name(), err)
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
		hostData[i] = p.hosts[i].HostData()
		hostData[i].NetInterfaceDataList = make([]data.NetInterfaceData, 0)

		for _, netInterface := range p.hosts[i].NetInterfaces() {
			hostData[i].NetInterfaceDataList = append(hostData[i].NetInterfaceDataList, netInterface.InterfaceData())
		}
	}

	return common.StatusAndEntities{
		Hosts: hostData,
	}
}
