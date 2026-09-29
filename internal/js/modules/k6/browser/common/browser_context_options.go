package common

import "fmt"

// BrowserContextOptions stores browser context options.
type BrowserContextOptions struct {
	AcceptDownloads   bool              `js:"acceptDownloads"`
	DownloadsPath     string            `js:"downloadsPath"`
	BypassCSP         bool              `js:"bypassCSP"`
	ColorScheme       ColorScheme       `js:"colorScheme"`
	DeviceScaleFactor float64           `js:"deviceScaleFactor"`
	ExtraHTTPHeaders  map[string]string `js:"extraHTTPHeaders"`
	Geolocation       *Geolocation      `js:"geolocation"`
	HasTouch          bool              `js:"hasTouch"`
	HTTPCredentials   Credentials       `js:"httpCredentials"`
	IgnoreHTTPSErrors bool              `js:"ignoreHTTPSErrors"`
	IsMobile          bool              `js:"isMobile"`
	JavaScriptEnabled bool              `js:"javaScriptEnabled"`
	Locale            string            `js:"locale"`
	Offline           bool              `js:"offline"`
	Permissions       []string          `js:"permissions"`
	ReducedMotion     ReducedMotion     `js:"reducedMotion"`
	Screen            Screen            `js:"screen"`
	TimezoneID        string            `js:"timezoneID"`
	UserAgent         string              `js:"userAgent"`
	RecordVideo       *RecordVideoOptions `js:"recordVideo"`
	Viewport          Viewport            `js:"viewport"`
}

// RecordVideoOptions holds options for video recording, matching the Playwright
// recordVideo API. Dir is the directory where videos are saved. Size optionally
// constrains the recorded video dimensions.
type RecordVideoOptions struct {
	Dir  string           `js:"dir"`
	Size *RecordVideoSize `js:"size"`
}

// RecordVideoSize holds the dimensions for recorded video frames.
type RecordVideoSize struct {
	Width  int64 `js:"width"`
	Height int64 `js:"height"`
}

// DefaultBrowserContextOptions returns the default browser context options.
func DefaultBrowserContextOptions() *BrowserContextOptions {
	return &BrowserContextOptions{
		ColorScheme:       ColorSchemeLight,
		DeviceScaleFactor: 1.0,
		ExtraHTTPHeaders:  make(map[string]string),
		JavaScriptEnabled: true,
		Locale:            DefaultLocale,
		Permissions:       []string{},
		ReducedMotion:     ReducedMotionNoPreference,
		Screen:            Screen{Width: DefaultScreenWidth, Height: DefaultScreenHeight},
		Viewport:          Viewport{Width: DefaultScreenWidth, Height: DefaultScreenHeight},
	}
}

// Geolocation represents a geolocation.
type Geolocation struct {
	Latitude  float64 `js:"latitude"`
	Longitude float64 `js:"longitude"`
	Accuracy  float64 `js:"accuracy"`
}

// Validate validates the [Geolocation].
func (g *Geolocation) Validate() error {
	if g.Longitude < -180 || g.Longitude > 180 {
		return fmt.Errorf(`invalid longitude "%.2f": precondition -180 <= LONGITUDE <= 180 failed`, g.Longitude)
	}
	if g.Latitude < -90 || g.Latitude > 90 {
		return fmt.Errorf(`invalid latitude "%.2f": precondition -90 <= LATITUDE <= 90 failed`, g.Latitude)
	}
	if g.Accuracy < 0 {
		return fmt.Errorf(`invalid accuracy "%.2f": precondition 0 <= ACCURACY failed`, g.Accuracy)
	}
	return nil
}

// GrantPermissionsOptions is used by BrowserContext.GrantPermissions.
type GrantPermissionsOptions struct {
	Origin string
}
