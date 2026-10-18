package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// MetricsService renders the panel's state in the Prometheus text exposition
// format (0.0.4), so no client library is needed.
type MetricsService struct {
	inbounds InboundService
	nodes    NodeService
}

type metricsWriter struct{ b strings.Builder }

func (w *metricsWriter) family(name, kind, help string) {
	fmt.Fprintf(&w.b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
}

// sample writes one line; labels are alternating name, value pairs.
func (w *metricsWriter) sample(name string, value any, labels ...string) {
	w.b.WriteString(name)
	if len(labels) > 0 {
		w.b.WriteByte('{')
		for i := 0; i+1 < len(labels); i += 2 {
			if i > 0 {
				w.b.WriteByte(',')
			}
			fmt.Fprintf(&w.b, `%s="%s"`, labels[i], escapeLabelValue(labels[i+1]))
		}
		w.b.WriteByte('}')
	}
	fmt.Fprintf(&w.b, " %v\n", value)
}

func (s *SettingService) GetMetricsEnable() (bool, error) {
	return s.getBool("metricsEnable")
}

func (s *SettingService) GetMetricsToken() (string, error) {
	return s.getString("metricsToken")
}

func escapeLabelValue(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v)
}

func boolGauge(v bool) int {
	if v {
		return 1
	}
	return 0
}

// Render builds the full exposition body from the host status and the database.
// A failing database read drops that family and is logged; the rest still renders.
func (s *MetricsService) Render(status *Status) string {
	w := &metricsWriter{}

	w.family("xui_panel_up", "gauge", "Always 1 while the panel answers.")
	w.sample("xui_panel_up", 1)
	if status != nil {
		w.family("xui_xray_up", "gauge", "1 when the Xray core process is running.")
		w.sample("xui_xray_up", boolGauge(status.Xray.State == Running))
		w.family("xui_cpu_ratio", "gauge", "Host CPU utilisation, 0 to 1.")
		w.sample("xui_cpu_ratio", status.Cpu/100)
		w.family("xui_memory_used_bytes", "gauge", "Host memory in use.")
		w.sample("xui_memory_used_bytes", status.Mem.Current)
		w.family("xui_memory_total_bytes", "gauge", "Host memory installed.")
		w.sample("xui_memory_total_bytes", status.Mem.Total)
		w.family("xui_disk_used_bytes", "gauge", "Root disk in use.")
		w.sample("xui_disk_used_bytes", status.Disk.Current)
		w.family("xui_disk_total_bytes", "gauge", "Root disk size.")
		w.sample("xui_disk_total_bytes", status.Disk.Total)
		w.family("xui_uptime_seconds", "gauge", "Host uptime.")
		w.sample("xui_uptime_seconds", status.Uptime)
	}

	var inbounds []model.Inbound
	if err := database.GetDB().Find(&inbounds).Error; err != nil {
		logger.Warning("metrics: read inbounds failed:", err)
	} else {
		tags := make(map[int]string, len(inbounds))
		w.family("xui_inbound_enabled", "gauge", "1 when the inbound is enabled.")
		for _, ib := range inbounds {
			tags[ib.Id] = ib.Tag
			w.sample("xui_inbound_enabled", boolGauge(ib.Enable), "inbound", ib.Tag, "remark", ib.Remark, "protocol", string(ib.Protocol))
		}
		w.family("xui_inbound_traffic_bytes", "counter", "Bytes carried by an inbound since its last reset.")
		for _, ib := range inbounds {
			w.sample("xui_inbound_traffic_bytes", ib.Up, "inbound", ib.Tag, "remark", ib.Remark, "protocol", string(ib.Protocol), "direction", "up")
			w.sample("xui_inbound_traffic_bytes", ib.Down, "inbound", ib.Tag, "remark", ib.Remark, "protocol", string(ib.Protocol), "direction", "down")
		}
		s.renderClients(w, tags)
	}

	online := 0
	for _, emails := range s.inbounds.GetOnlineClientsByGuid() {
		online += len(emails)
	}
	w.family("xui_online_clients", "gauge", "Clients with a live connection, across this panel and its nodes.")
	w.sample("xui_online_clients", online)

	s.renderNodes(w)
	return w.b.String()
}

func (s *MetricsService) renderClients(w *metricsWriter, tags map[int]string) {
	var clients []xray.ClientTraffic
	if err := database.GetDB().Find(&clients).Error; err != nil {
		logger.Warning("metrics: read client traffic failed:", err)
		return
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].Email < clients[j].Email })
	w.family("xui_client_traffic_bytes", "counter", "Bytes carried by a client since its last reset.")
	for _, c := range clients {
		w.sample("xui_client_traffic_bytes", c.Up, "email", c.Email, "inbound", tags[c.InboundId], "direction", "up")
		w.sample("xui_client_traffic_bytes", c.Down, "email", c.Email, "inbound", tags[c.InboundId], "direction", "down")
	}
	w.family("xui_client_traffic_limit_bytes", "gauge", "Traffic quota of a client; 0 means unlimited.")
	w.family("xui_client_enabled", "gauge", "1 when the client is enabled.")
	for _, c := range clients {
		w.sample("xui_client_traffic_limit_bytes", c.Total, "email", c.Email, "inbound", tags[c.InboundId])
	}
	for _, c := range clients {
		w.sample("xui_client_enabled", boolGauge(c.Enable), "email", c.Email, "inbound", tags[c.InboundId])
	}
}

func (s *MetricsService) renderNodes(w *metricsWriter) {
	nodes, err := s.nodes.GetAll()
	if err != nil {
		logger.Warning("metrics: read nodes failed:", err)
		return
	}
	w.family("xui_node_up", "gauge", "1 when the node answered its last heartbeat.")
	for _, n := range nodes {
		w.sample("xui_node_up", boolGauge(n.Status == "online"), "node", n.Name)
	}
	w.family("xui_node_latency_seconds", "gauge", "Heartbeat round-trip time of the node.")
	w.family("xui_node_cpu_ratio", "gauge", "Node CPU utilisation, 0 to 1.")
	w.family("xui_node_memory_ratio", "gauge", "Node memory utilisation, 0 to 1.")
	for _, n := range nodes {
		w.sample("xui_node_latency_seconds", float64(n.LatencyMs)/1000, "node", n.Name)
	}
	for _, n := range nodes {
		w.sample("xui_node_cpu_ratio", n.CpuPct/100, "node", n.Name)
	}
	for _, n := range nodes {
		w.sample("xui_node_memory_ratio", n.MemPct/100, "node", n.Name)
	}
}
