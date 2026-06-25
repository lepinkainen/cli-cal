package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
	"gopkg.in/yaml.v3"
)

// defaultDays is used when neither the -days flag nor the config sets a value.
const defaultDays = 2

// Version is injected at build time via -ldflags "-X main.Version=...".
var Version = "dev"

// Event is one calendar occurrence within the viewing window.
type Event struct {
	Cal      string
	Summary  string
	Location string
	Start    time.Time
	End      time.Time
	AllDay   bool
}

// Calendar is a named feed URL from the config file.
type Calendar struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Config is the YAML configuration document.
type Config struct {
	Days      int        `yaml:"days"`
	Calendars []Calendar `yaml:"calendars"`
}

func main() {
	var (
		cfgPath     string
		days        int
		showVersion bool
	)
	flag.StringVar(&cfgPath, "config", xdgConfigPath(), "path to YAML config")
	flag.IntVar(&days, "days", 0, "number of days to show starting today (overrides config)")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()

	if showVersion {
		fmt.Println("cal", Version)
		return
	}

	// First run with no config: write a sample and exit so the user can edit it.
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := writeSampleConfig(cfgPath); err != nil {
			fmt.Fprintln(os.Stderr, "could not write sample config:", err)
			os.Exit(1)
		}
		fmt.Printf("wrote sample config to %s — edit it and re-run\n", cfgPath)
		return
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	if len(cfg.Calendars) == 0 {
		fmt.Fprintln(os.Stderr, "no calendars defined in", cfgPath)
		os.Exit(1)
	}

	// Days precedence: -days flag > config days > built-in default.
	resolvedDays := defaultDays
	if cfg.Days > 0 {
		resolvedDays = cfg.Days
	}
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "days" {
			resolvedDays = days
		}
	})

	now := time.Now()
	windowStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	windowEnd := windowStart.AddDate(0, 0, resolvedDays)

	events, errs := fetchAll(cfg.Calendars, windowStart, windowEnd)

	sort.Slice(events, func(i, j int) bool {
		if events[i].Start.Equal(events[j].Start) {
			return events[i].Summary < events[j].Summary
		}
		return events[i].Start.Before(events[j].Start)
	})

	render(os.Stdout, events, windowStart, resolvedDays)

	for _, e := range errs {
		fmt.Fprintln(os.Stderr, warnStyle.Render("! "+e))
	}
}

// xdgConfigPath returns the config file location following the XDG spec:
// $XDG_CONFIG_HOME/cal/config.yaml, falling back to ~/.config/cal/config.yaml.
// (os.UserConfigDir is avoided: on darwin it returns ~/Library/Application Support.)
func xdgConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".config", "cal", "config.yaml")
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "cal", "config.yaml")
}

// loadConfig reads and parses the YAML config, normalizing calendar URLs.
func loadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // config path is user-supplied by design
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	for i := range cfg.Calendars {
		cfg.Calendars[i].URL = normalizeURL(cfg.Calendars[i].URL)
	}
	return cfg, nil
}

const sampleConfig = `# cal configuration

# Number of days to show, starting today.
days: 2

# Calendars to read. URLs may be http(s) or webcal:// ICS feeds.
calendars:
  - name: Example
    url: https://example.com/calendar.ics
`

// writeSampleConfig creates the parent directory and writes a commented sample.
func writeSampleConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	// 0600: config may hold calendar URLs with embedded API keys.
	return os.WriteFile(path, []byte(sampleConfig), 0o600)
}

// normalizeURL maps webcal scheme to https for fetching.
func normalizeURL(u string) string {
	if rest, ok := strings.CutPrefix(u, "webcal://"); ok {
		return "https://" + rest
	}
	return u
}

// fetchAll downloads and parses every calendar concurrently.
func fetchAll(cals []Calendar, start, end time.Time) (events []Event, errs []string) {
	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	for _, c := range cals {
		wg.Add(1)
		go func(c Calendar) {
			defer wg.Done()
			evs, err := fetchCalendar(c, start, end)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", c.Name, err))
				return
			}
			events = append(events, evs...)
		}(c)
	}
	wg.Wait()
	return events, errs
}

func fetchCalendar(c Calendar, start, end time.Time) ([]Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}

	cal, err := ics.ParseCalendar(strings.NewReader(sanitizeICS(string(body))))
	if err != nil {
		return nil, err
	}

	var out []Event
	for _, ve := range cal.Events() {
		out = append(out, expandEvent(c.Name, ve, start, end)...)
	}
	return out, nil
}

// contentLineStart matches a valid ICS content line beginning: a property
// name (letters/digits/hyphen) immediately followed by ':' or ';'.
var contentLineStart = func(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
			continue
		}
		return i > 0 && (c == ':' || c == ';')
	}
	return false
}

// sanitizeICS repairs feeds (notably Apple's) whose multi-line property values
// are folded inconsistently — continuation lines that lost their leading space
// would otherwise crash the strict parser. It rebuilds logical lines: proper
// space-folded continuations are unfolded, and stray non-content lines are
// appended to the previous logical line. Output is re-emitted CRLF-joined with
// no folding so the parser sees clean one-line properties.
func sanitizeICS(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")

	var logical []string
	for _, l := range lines {
		switch {
		case l == "":
			// Preserve blank lines as boundaries.
			logical = append(logical, "")
		case l[0] == ' ' || l[0] == '\t':
			// Proper fold: drop one leading whitespace, join to previous.
			if n := len(logical); n > 0 {
				logical[n-1] += l[1:]
			}
		case contentLineStart(l):
			logical = append(logical, l)
		default:
			// Stray continuation (broken fold): append verbatim to previous.
			if n := len(logical); n > 0 {
				logical[n-1] += l
			} else {
				logical = append(logical, l)
			}
		}
	}
	return strings.Join(logical, "\r\n")
}

// expandEvent yields occurrences of a VEVENT that fall in [start, end),
// expanding recurrence rules when present.
func expandEvent(calName string, ve *ics.VEvent, start, end time.Time) []Event {
	dtStart, err := ve.GetStartAt()
	if err != nil {
		return nil
	}
	allDay := isAllDay(ve)

	var dur time.Duration
	if dtEnd, endErr := ve.GetEndAt(); endErr == nil && dtEnd.After(dtStart) {
		dur = dtEnd.Sub(dtStart)
	} else if allDay {
		dur = 24 * time.Hour
	}

	summary := ""
	if p := ve.GetProperty(ics.ComponentPropertySummary); p != nil {
		summary = strings.TrimSpace(p.Value)
	}
	location := ""
	if p := ve.GetProperty(ics.ComponentPropertyLocation); p != nil {
		location = strings.TrimSpace(p.Value)
	}

	mk := func(s time.Time) Event {
		return Event{Cal: calName, Summary: summary, Location: location, Start: s, End: s.Add(dur), AllDay: allDay}
	}

	rruleProp := ve.GetProperty(ics.ComponentPropertyRrule)
	if rruleProp == nil {
		if dtStart.Before(end) && dtStart.Add(maxDur(dur, time.Nanosecond)).After(start) {
			return []Event{mk(dtStart)}
		}
		return nil
	}

	// Recurring: expand within window.
	opt, err := rrule.StrToROptionInLocation(rruleProp.Value, dtStart.Location())
	if err != nil {
		// Fall back to single instance.
		if dtStart.Before(end) {
			return []Event{mk(dtStart)}
		}
		return nil
	}
	opt.Dtstart = dtStart
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return nil
	}

	var out []Event
	for _, occ := range r.Between(start.Add(-dur), end, true) {
		if occ.Before(end) {
			out = append(out, mk(occ))
		}
	}
	return out
}

func isAllDay(ve *ics.VEvent) bool {
	p := ve.GetProperty(ics.ComponentPropertyDtStart)
	if p == nil {
		return false
	}
	// VALUE=DATE params indicate an all-day event.
	if v, ok := p.ICalParameters["VALUE"]; ok {
		for _, x := range v {
			if strings.EqualFold(x, "DATE") {
				return true
			}
		}
	}
	return len(p.Value) == 8 // YYYYMMDD
}

func maxDur(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
