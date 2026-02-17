package config

type PluginConfig struct {
	Hosts []Host
}

func (c *PluginConfig) AddHost(name string, ipAddress string) Host {
	host := Host{
		Name:          name,
		IpAddress:     ipAddress,
		NetInterfaces: make(map[string]*NetInterface),
	}

	c.Hosts = append(c.Hosts, host)

	return host
}
