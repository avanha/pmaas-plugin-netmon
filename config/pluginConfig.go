package config

type PluginConfig struct {
	Hosts []*Host
}

// AddHost adds a host to monitor, and returns it so it can be adjusted. It is a pointer to the host the plugin will use, so
// it stays so however many more hosts are added.
func (c *PluginConfig) AddHost(name string, ipAddress string) *Host {
	host := &Host{
		Name:               name,
		IpAddress:          ipAddress,
		PingEnabled:        true,
		PingTimeoutSeconds: 10,
		PingCount:          4,
		PingUseIcmp:        false,
		SnmpEnabled:        true,
		NetInterfaces:      make(map[string]*NetInterface),
	}

	c.Hosts = append(c.Hosts, host)

	return host
}
