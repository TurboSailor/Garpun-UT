package uxbridge

import (
	"fmt"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"

	"pulse/backend/internal/gfdi"
)

// MPRIS2 is the only media control interface Lomiri exposes. Players come and
// go, so the active one is resolved on every read instead of being cached.

const (
	mprisPrefix = "org.mpris.MediaPlayer2."
	mprisPath   = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	ifaceMPRIS  = "org.mpris.MediaPlayer2"
	ifacePlayer = "org.mpris.MediaPlayer2.Player"
)

// Music control commands as numbered by GFDI MUSIC_CONTROL.
const (
	MusicTogglePlayPause uint8 = 0
	MusicNext            uint8 = 1
	MusicPrevious        uint8 = 2
	MusicVolumeUp        uint8 = 3
	MusicVolumeDown      uint8 = 4
	MusicPlay            uint8 = 5
	MusicPause           uint8 = 6
	MusicSkipForward     uint8 = 7
	MusicSkipBackwards   uint8 = 8
)

// volumeStep is how much one watch volume press moves the player, on the 0..1
// MPRIS scale.
const volumeStep = 0.1

type musicSource struct {
	conn *dbus.Conn

	mu sync.Mutex
}

func (m *musicSource) close() {
	if m.conn != nil {
		m.conn.Close()
	}
}

func (b *Bridge) startMusic() error {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return fmt.Errorf("uxbridge: session bus: %w", err)
	}
	if err := conn.Auth(nil); err != nil {
		conn.Close()
		return fmt.Errorf("uxbridge: session bus auth: %w", err)
	}
	if err := conn.Hello(); err != nil {
		conn.Close()
		return fmt.Errorf("uxbridge: session bus hello: %w", err)
	}
	b.music = &musicSource{conn: conn}
	return nil
}

// player returns the bus name of the player to control: the one that is
// playing, else the first that reports itself paused, else any.
func (m *musicSource) player() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var names []string
	if err := m.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return "", fmt.Errorf("uxbridge: list bus names: %w", err)
	}
	var paused, any string
	for _, n := range names {
		if !strings.HasPrefix(n, mprisPrefix) {
			continue
		}
		if any == "" {
			any = n
		}
		switch m.str(n, ifacePlayer, "PlaybackStatus") {
		case "Playing":
			return n, nil
		case "Paused":
			if paused == "" {
				paused = n
			}
		}
	}
	if paused != "" {
		return paused, nil
	}
	if any != "" {
		return any, nil
	}
	return "", fmt.Errorf("uxbridge: no mpris player on the session bus")
}

func (m *musicSource) prop(name, iface, prop string) (dbus.Variant, error) {
	var v dbus.Variant
	err := m.conn.Object(name, mprisPath).
		Call("org.freedesktop.DBus.Properties.Get", 0, iface, prop).Store(&v)
	return v, err
}

func (m *musicSource) str(name, iface, prop string) string {
	v, err := m.prop(name, iface, prop)
	if err != nil {
		return ""
	}
	s, _ := v.Value().(string)
	return s
}

func (m *musicSource) float(name, iface, prop string) float64 {
	v, err := m.prop(name, iface, prop)
	if err != nil {
		return 0
	}
	f, _ := v.Value().(float64)
	return f
}

// mprisTrack is the subset of xesam metadata the watch displays.
type mprisTrack struct {
	Title    string
	Artist   string
	Album    string
	Duration int64 // seconds
}

// parseMPRISMetadata decodes the a{sv} Metadata property. Artist is a list in
// the spec; players routinely send a bare string instead.
func parseMPRISMetadata(md map[string]dbus.Variant) mprisTrack {
	var t mprisTrack
	if v, ok := md["xesam:title"]; ok {
		t.Title, _ = v.Value().(string)
	}
	if v, ok := md["xesam:album"]; ok {
		t.Album, _ = v.Value().(string)
	}
	if v, ok := md["xesam:artist"]; ok {
		switch a := v.Value().(type) {
		case []string:
			t.Artist = strings.Join(a, ", ")
		case string:
			t.Artist = a
		}
	}
	if v, ok := md["mpris:length"]; ok {
		switch l := v.Value().(type) {
		case int64:
			t.Duration = l / 1_000_000
		case uint64:
			t.Duration = int64(l) / 1_000_000
		case float64:
			t.Duration = int64(l) / 1_000_000
		}
	}
	return t
}

// MusicMetadata snapshots the active player as GFDI music entity values, ready
// for MUSIC_CONTROL_ENTITY_UPDATE. An absent player yields nil.
func (b *Bridge) MusicMetadata() []gfdi.MusicEntityValue {
	if b.music == nil {
		return nil
	}
	name, err := b.music.player()
	if err != nil {
		b.log.Debug("uxbridge: music metadata unavailable", "err", err)
		return nil
	}

	var md map[string]dbus.Variant
	if v, err := b.music.prop(name, ifacePlayer, "Metadata"); err == nil {
		md, _ = v.Value().(map[string]dbus.Variant)
	}
	track := parseMPRISMetadata(md)

	playing := 0
	if b.music.str(name, ifacePlayer, "PlaybackStatus") == "Playing" {
		playing = 1
	}
	rate := 0.0
	if playing == 1 {
		rate = 1.0
	}

	identity := b.music.str(name, ifaceMPRIS, "Identity")
	if identity == "" {
		identity = strings.TrimPrefix(name, mprisPrefix)
	}

	return []gfdi.MusicEntityValue{
		{Entity: gfdi.EntityTrack, Attribute: gfdi.TrackTitle, Value: track.Title},
		{Entity: gfdi.EntityTrack, Attribute: gfdi.TrackArtist, Value: track.Artist},
		{Entity: gfdi.EntityTrack, Attribute: gfdi.TrackAlbum, Value: track.Album},
		{Entity: gfdi.EntityTrack, Attribute: gfdi.TrackDuration, Value: fmt.Sprintf("%d", track.Duration)},
		{Entity: gfdi.EntityPlayer, Attribute: gfdi.PlayerName, Value: identity},
		// Watch expects "<playing>,<rate>,<position seconds>".
		{Entity: gfdi.EntityPlayer, Attribute: gfdi.PlayerPlaybackInfo,
			Value: fmt.Sprintf("%d,%.1f,%.3f", playing, rate, float64(b.music.position(name))/1e6)},
		{Entity: gfdi.EntityPlayer, Attribute: gfdi.PlayerVolume,
			Value: fmt.Sprintf("%.2f", b.music.float(name, ifacePlayer, "Volume"))},
	}
}

func (m *musicSource) position(name string) int64 {
	v, err := m.prop(name, ifacePlayer, "Position")
	if err != nil {
		return 0
	}
	p, _ := v.Value().(int64)
	return p
}

// MusicCommand applies a GFDI music control command to the active player.
func (b *Bridge) MusicCommand(cmd uint8) error {
	if b.music == nil {
		return fmt.Errorf("uxbridge: media control unavailable")
	}
	name, err := b.music.player()
	if err != nil {
		return err
	}
	obj := b.music.conn.Object(name, mprisPath)

	var method string
	switch cmd {
	case MusicTogglePlayPause:
		method = "PlayPause"
	case MusicNext, MusicSkipForward:
		method = "Next"
	case MusicPrevious, MusicSkipBackwards:
		method = "Previous"
	case MusicPlay:
		method = "Play"
	case MusicPause:
		method = "Pause"
	case MusicVolumeUp, MusicVolumeDown:
		vol := b.music.float(name, ifacePlayer, "Volume")
		if cmd == MusicVolumeUp {
			vol += volumeStep
		} else {
			vol -= volumeStep
		}
		vol = min(max(vol, 0), 1)
		set := obj.Call("org.freedesktop.DBus.Properties.Set", 0, ifacePlayer, "Volume", dbus.MakeVariant(vol))
		if set.Err != nil {
			return fmt.Errorf("uxbridge: mpris set volume: %w", set.Err)
		}
		return nil
	default:
		return fmt.Errorf("uxbridge: unknown music command %d", cmd)
	}

	if call := obj.Call(ifacePlayer+"."+method, 0); call.Err != nil {
		return fmt.Errorf("uxbridge: mpris %s: %w", method, call.Err)
	}
	return nil
}
