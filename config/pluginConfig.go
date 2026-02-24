package config

type PluginConfig struct {
	Hosts []Host
}

func (c *PluginConfig) AddHost(name string, ipAddress string) *Host {
	host := Host{
		Name:               name,
		IpAddress:          ipAddress,
		PingEnabled:        true,
		PingTimeoutSeconds: 10,
		PingCount:          3,
		PingUseIcmp:        false,
		SnmpEnabled:        true,
		NetInterfaces:      make(map[string]*NetInterface),
	}

	c.Hosts = append(c.Hosts, host)

	return &c.Hosts[len(c.Hosts)-1]
}
