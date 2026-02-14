package http

import (
	"embed"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"text/template"
	"time"

	"github.com/avanha/pmaas-plugin-netmon/data"
	"github.com/avanha/pmaas-plugin-netmon/internal/common"
	"github.com/avanha/pmaas-spi"
)

//go:embed content/static content/templates
var contentFS embed.FS

func RenderHostInterface(interfaceWithRenderer *hostInterfaceWithRenderer) (string, error) {
	return interfaceWithRenderer.Renderer(&interfaceWithRenderer.NetInterface)
}

func RenderGraph(data ...[]uint64) (string, error) {
	result := "<div class=\"bar-graph\">"
	maxValue := uint64(0)

	for i := 0; i < len(data); i++ {
		seriesMax := slices.Max(data[i])
		if seriesMax > maxValue {
			maxValue = seriesMax
		}
	}

	for i := 0; i < len(data[0]); i++ {
		result += "<div class=\"bar-container\">"
		result += fmt.Sprintf("<span class=\"data-label\">Tx: %s Rx: %s</span>",
			FormatBits(data[0][i]), FormatBits(data[1][i]))

		for j := 0; j < len(data); j++ {
			result += fmt.Sprintf("<div class=\"bar series-%d\" style=\"height: %d%%;\"> </div>",
				j,
				int(float32(data[j][i])/float32(maxValue)*float32(100)))

		}

		result += "<div class=\"bar-overlay\"> </div>"
		result += "</div>"
	}

	result += "</div>"

	return result, nil
}

var dataSizeSuffixes = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB"}
var dataRateSuffixes = []string{"b", "kb", "Mb", "Gb", "Tb", "Pb", "Eb", "Zb", "Yb"}

func FormatBits(bytes uint64) string {
	const unit uint64 = 1000

	bits := bytes * 8

	if bits < unit {
		return fmt.Sprintf("%d b", bits)
	}

	div, exp := unit, 0

	for n := bits / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %s", float64(bits)/float64(div), dataRateSuffixes[exp+1])
}

func FormatBytes(bytes uint64) string {
	const unit uint64 = 1024

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := unit, 0

	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), dataSizeSuffixes[exp+1])
}

func FormatDuration(duration time.Duration) string {
	days := duration / (24 * time.Hour)
	duration %= 24 * time.Hour
	daysWord := "Days"

	if days == 1 {
		daysWord = "Day"
	}

	hours := duration / time.Hour
	duration %= time.Hour
	hoursWord := "Hours"

	if hours == 1 {
		hoursWord = "Hour"
	}

	minutes := duration / time.Minute
	duration %= time.Minute
	minutesWord := "Minutes"

	if minutes == 1 {
		minutesWord = "Minute"
	}

	seconds := duration / time.Second
	secondsWord := "Seconds"

	if seconds == 1 {
		secondsWord = "Second"
	}

	return fmt.Sprintf(
		"%d %s %d %s %d %s %d %s",
		days, daysWord, hours, hoursWord, minutes, minutesWord, seconds, secondsWord)
}

var hostTemplate = spi.TemplateInfo{
	Name:   "host",
	Paths:  []string{"templates/host.htmlt"},
	Styles: slices.Concat([]string{"css/host.css"}, netInterfaceTemplate.Styles),
	FuncMap: template.FuncMap{
		"RenderHostInterface": RenderHostInterface,
		"FormatDuration":      FormatDuration,
	},
}

var netInterfaceTemplate = spi.TemplateInfo{
	Name:   "net_interface",
	Paths:  []string{"templates/net_interface.htmlt"},
	Styles: []string{"css/net_interface.css"},
	FuncMap: template.FuncMap{
		"FormatBytes": FormatBytes,
		"FormatBits":  FormatBits,
		"RenderGraph": RenderGraph,
	},
}

type Handler struct {
	container   spi.IPMAASContainer
	entityStore common.EntityStore
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Init(container spi.IPMAASContainer, entityStore common.EntityStore) {
	h.container = container
	h.entityStore = entityStore
	container.ProvideContentFS(&contentFS, "content")
	container.EnableStaticContent("static")
	container.AddRoute("/plugins/netmon/", h.handleHttpListRequest)
	container.RegisterEntityRenderer(
		reflect.TypeOf((*hostWithInterfaces)(nil)).Elem(),
		h.hostDataRendererFactory)
	container.RegisterEntityRenderer(
		reflect.TypeOf((*data.NetInterfaceData)(nil)).Elem(),
		h.netInterfaceDataRendererFactory)
}

func (h *Handler) handleHttpListRequest(writer http.ResponseWriter, request *http.Request) {
	result, err := h.entityStore.GetStatusAndEntities()

	if err != nil {
		panic(fmt.Errorf("netmon handleHttpListRequest: Error retrieving entities: %w", err))
	}

	sort.SliceStable(result.Hosts, func(i, j int) bool {
		return result.Hosts[i].Name < result.Hosts[j].Name
	})

	interfaceRenderer, err := h.netInterfaceDataRendererFactory()

	if err != nil {
		panic(fmt.Errorf("netmon handleHttpListRequest: Error retrieving NetInterfaceData renderer: %w", err))
	}

	// Convert the slice of structs to a slice of any
	entityListSize := len(result.Hosts)
	entityPointers := make([]any, entityListSize)

	for i := 0; i < entityListSize; i++ {
		host := hostWithInterfaces{
			HostData:       result.Hosts[i],
			RelativeUptime: time.Duration(result.Hosts[i].UptimeSeconds) * time.Second,
			Interfaces:     make([]*hostInterfaceWithRenderer, len(result.Hosts[i].NetInterfaceDataList)),
		}
		interfaceListSize := len(result.Hosts[i].NetInterfaceDataList)

		for j := 0; j < interfaceListSize; j++ {
			host.Interfaces[j] = &hostInterfaceWithRenderer{
				NetInterface: result.Hosts[i].NetInterfaceDataList[j],
				Renderer:     interfaceRenderer.RenderFunc,
			}
		}

		sort.SliceStable(host.Interfaces, func(i, j int) bool {
			return host.Interfaces[i].NetInterface.Name < host.Interfaces[j].NetInterface.Name
		})

		entityPointers[i] = &host
	}

	h.container.RenderList(
		writer,
		request,
		spi.RenderListOptions{
			Title: "netmon",
			//Header: &result.Status,
		},
		entityPointers)
}

func (h *Handler) hostDataRendererFactory() (spi.EntityRenderer, error) {
	return spi.TemplateBasedRendererFactory(
		h.container,
		&hostTemplate,
		func(entity any) bool {
			_, ok := entity.(*hostWithInterfaces)
			return ok
		},
		"*hostWithInterfaces")
}

func (h *Handler) netInterfaceDataRendererFactory() (spi.EntityRenderer, error) {
	return spi.TemplateBasedRendererFactory(
		h.container,
		&netInterfaceTemplate,
		func(entity any) bool {
			_, ok := entity.(*data.NetInterfaceData)
			return ok
		},
		"*data.NetInterfaceData")
}
