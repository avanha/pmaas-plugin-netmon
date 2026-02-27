module github.com/avanha/pmaas-plugin-netmon

go 1.25

toolchain go1.25.1

require github.com/avanha/pmaas-common v0.0.0

require github.com/avanha/pmaas-spi v0.0.2

require (
	github.com/gosnmp/gosnmp v1.42.1
	github.com/prometheus-community/pro-bing v0.8.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
)
