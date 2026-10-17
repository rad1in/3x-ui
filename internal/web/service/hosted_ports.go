package service

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

var (
	ErrHostedPanelPortRange = errors.New("invalid hosted panel ports")
	ErrHostedPanelPortTaken = errors.New("port already in use on this server")
)

func validPort(p int) bool { return p >= 1 && p <= 65535 }

// CheckHostedPanelPorts refuses an address or inbound range that collides with
// the master panel, its inbounds, or another hosted panel on this server.
func CheckHostedPanelPorts(db *gorm.DB, p *model.HostedPanel) error {
	switch {
	case !validPort(p.WebPort) || !validPort(p.SubPort) || p.WebPort == p.SubPort:
		return fmt.Errorf("%w: panel and subscription need two different ports", ErrHostedPanelPortRange)
	case !validPort(p.PortMin) || !validPort(p.PortMax) || p.PortMin > p.PortMax:
		return fmt.Errorf("%w: the inbound port range is required", ErrHostedPanelPortRange)
	case inRange(p.WebPort, p.PortMin, p.PortMax) || inRange(p.SubPort, p.PortMin, p.PortMax):
		return fmt.Errorf("%w: the panel ports must sit outside its inbound range", ErrHostedPanelPortRange)
	}

	settings := &SettingService{}
	webPort, err := settings.GetPort()
	if err != nil {
		return err
	}
	subPort, err := settings.GetSubPort()
	if err != nil {
		return err
	}
	for _, own := range []struct {
		port int
		what string
	}{{webPort, "this panel"}, {subPort, "this panel's subscription"}, {reservedAPIPort(), "the Xray API"}} {
		if claims(p, own.port) {
			return fmt.Errorf("%w: %d (%s)", ErrHostedPanelPortTaken, own.port, own.what)
		}
	}

	var ib model.Inbound
	err = db.Where("node_id IS NULL AND (port = ? OR port = ? OR port BETWEEN ? AND ?)",
		p.WebPort, p.SubPort, p.PortMin, p.PortMax).First(&ib).Error
	if err == nil {
		return fmt.Errorf("%w: %d (inbound %s)", ErrHostedPanelPortTaken, ib.Port, ib.Tag)
	} else if !database.IsNotFound(err) {
		return err
	}

	var others []model.HostedPanel
	if err := db.Where("id <> ?", p.Id).Find(&others).Error; err != nil {
		return err
	}
	for _, o := range others {
		if claims(p, o.WebPort) || claims(p, o.SubPort) || claims(&o, p.WebPort) || claims(&o, p.SubPort) ||
			(p.PortMin <= o.PortMax && o.PortMin <= p.PortMax) {
			return fmt.Errorf("%w: hosted panel '%s' (%d, %d, %d-%d)",
				ErrHostedPanelPortTaken, o.Name, o.WebPort, o.SubPort, o.PortMin, o.PortMax)
		}
	}
	return nil
}

func inRange(port, lo, hi int) bool { return port >= lo && port <= hi }

func claims(p *model.HostedPanel, port int) bool {
	return port == p.WebPort || port == p.SubPort || inRange(port, p.PortMin, p.PortMax)
}

// hostedPanelOwning returns the hosted panel whose address or inbound range holds port.
func hostedPanelOwning(db *gorm.DB, port int) (*model.HostedPanel, error) {
	var p model.HostedPanel
	err := db.Where("web_port = ? OR sub_port = ? OR (port_min <= ? AND port_max >= ?)", port, port, port, port).
		First(&p).Error
	if database.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func checkPortsFreeOfHostedPanels(ports ...int) error {
	for _, port := range ports {
		owner, err := hostedPanelOwning(database.GetDB(), port)
		if err != nil {
			return err
		}
		if owner != nil {
			return fmt.Errorf("%w: %d (hosted panel '%s')", ErrHostedPanelPortTaken, port, owner.Name)
		}
	}
	return nil
}
