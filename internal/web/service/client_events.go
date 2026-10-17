package service

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func publishClientEvent(t eventbus.EventType, d eventbus.ClientEventData) {
	if eventBus == nil || d.Email == "" {
		return
	}
	eventBus.Publish(eventbus.Event{Type: t, Source: d.Email, Data: &d, Timestamp: time.Now()})
}

type clientState struct {
	data   eventbus.ClientEventData
	enable bool
}

// snapshotClients reads the given clients as stored now, with their usage.
// Operations compare a snapshot from before with one from after and publish
// what actually changed, which also covers partly failed batch operations.
func snapshotClients(emails []string) map[string]clientState {
	out := map[string]clientState{}
	if len(emails) == 0 {
		return out
	}
	db := database.GetDB()
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var recs []model.ClientRecord
		if db.Where("email IN ?", batch).Find(&recs).Error != nil {
			continue
		}
		var traffics []xray.ClientTraffic
		db.Select("email", "up", "down").Where("email IN ?", batch).Find(&traffics)
		usage := make(map[string][2]int64, len(traffics))
		for _, t := range traffics {
			usage[t.Email] = [2]int64{t.Up, t.Down}
		}
		for _, r := range recs {
			u := usage[r.Email]
			out[r.Email] = clientState{
				data: eventbus.ClientEventData{
					Email: r.Email, SubID: r.SubID, Up: u[0], Down: u[1], Total: r.TotalGB, ExpiryTime: r.ExpiryTime,
				},
				enable: r.Enable,
			}
		}
	}
	return out
}

func publishClientChanges(before, after map[string]clientState) {
	for email, a := range after {
		b, existed := before[email]
		switch {
		case !existed:
			publishClientEvent(eventbus.EventClientCreated, a.data)
		case !b.enable && a.enable:
			publishClientEvent(eventbus.EventClientEnabled, a.data)
		case b.enable && !a.enable:
			publishClientEvent(eventbus.EventClientDisabled, a.data)
		}
	}
	for email, b := range before {
		if _, still := after[email]; !still {
			publishClientEvent(eventbus.EventClientDeleted, b.data)
		}
	}
}

// trackClients snapshots emails and returns the call that publishes their changes.
func trackClients(emails ...string) func() {
	if eventBus == nil {
		return func() {}
	}
	before := snapshotClients(emails)
	return func() { publishClientChanges(before, snapshotClients(emails)) }
}

// clientEventFromTraffic is a client as the traffic tick sees it.
// publishTrafficResets reports clients whose usage was zeroed, as they stand now.
func publishTrafficResets(emails []string) {
	if eventBus == nil {
		return
	}
	for _, st := range snapshotClients(emails) {
		publishClientEvent(eventbus.EventClientTrafficReset, st.data)
	}
}

func clientEventFromTraffic(t xray.ClientTraffic) eventbus.ClientEventData {
	return eventbus.ClientEventData{
		Email: t.Email, SubID: t.SubId, Up: t.Up, Down: t.Down, Total: t.Total, ExpiryTime: t.ExpiryTime,
	}
}

// publishTickEvents sends a committed traffic tick's cut-offs and renewals;
// the tick's rows lack the sub id, so it comes from the clients table.
func publishTickEvents(b *trafficMutationBatch) {
	if eventBus == nil || (len(b.events) == 0 && len(b.renewedEmails) == 0) {
		return
	}
	emails := append([]string(nil), b.renewedEmails...)
	for _, e := range b.events {
		emails = append(emails, e.Source)
	}
	states := snapshotClients(emails)
	for _, e := range b.events {
		if d, ok := e.Data.(*eventbus.ClientEventData); ok && d.SubID == "" {
			d.SubID = states[e.Source].data.SubID
		}
		eventBus.Publish(e)
	}
	for _, email := range b.renewedEmails {
		if st, ok := states[email]; ok {
			publishClientEvent(eventbus.EventClientRenewed, st.data)
		}
	}
}
