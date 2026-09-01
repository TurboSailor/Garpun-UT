package uxbridge

import (
	"time"

	"pulse/backend/internal/garmin"
)

// The only place in this package that knows a Garmin watch exists. Everything
// above deals in plain Notification values; this file adapts them to what
// garmin.Session asks for when the watch requests notification attributes.

// Lookup answers garmin.Hooks.NotificationContent: the watch has seen a
// notification id and now wants its text. A retracted or evicted notification
// yields nil, which the session reports as unavailable.
func (b *Bridge) Lookup(id int32) *garmin.NotificationContent {
	n, live := b.find(id)
	if !live {
		return nil
	}
	return n.GarminContent()
}

// GarminContent converts one notification to the watch representation.
func (n Notification) GarminContent() *garmin.NotificationContent {
	appID := n.AppID
	if appID == "" {
		appID = n.Source
	}
	return &garmin.NotificationContent{
		ID:            n.ID,
		AppIdentifier: appID,
		Title:         n.Title,
		Subtitle:      n.AppName,
		Message:       n.Body,
		Date:          time.UnixMilli(n.TsMs),
		Actions:       n.Actions,
		Category:      n.Category,
	}
}
