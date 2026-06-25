# cal

A fast, read-only CLI calendar. Point it at your CalDAV / ICS feeds and it
prints the next few days of events in a clean, colorful layout — then quits.

- **Read-only.** Never creates, edits, or deletes anything in your calendars.
- **Parallel.** All feeds are fetched concurrently.
- **Handles real feeds.** Expands recurring events (`RRULE`) and repairs the
  quirky line-folding Apple's iCloud feeds emit.

## "Screenshot"

```text
 📅  Calendar — next 2 days

Fri, Jun 26  ·  today
─────────────────────
  09:00–09:30   Standup                        (Work)
  all day       Joe's birthday                (Family)
  13:00–14:00   Lunch with Sam                 (Personal)
                @ Café Esplanad
  20:00–21:00   The Bear - 4x03                 (Sonarr TV)

Sat, Jun 27  ·  tomorrow
────────────────────────
  all day       Cottage weekend                (Family)
  all day       Dune: Part Three (Cinema)       (Radarr Movies)
  11:30–12:30   Dentist                         (Personal)
```

## Install

**From a GitHub release** — download the archive for your platform from the
[releases page](https://github.com/lepinkainen/cli-cal/releases), extract, and
drop `cal` somewhere on your `PATH`.

**From source:**

```sh
git clone https://github.com/lepinkainen/cli-cal
cd cli-cal
task build      # -> ./build/cal   (or: go build -o cal .)
```

## Configuration

On first run with no config, `cal` writes a commented sample and exits. Edit it,
then run again.

Location (XDG): `$XDG_CONFIG_HOME/cal/config.yaml`, falling back to
`~/.config/cal/config.yaml`.

```yaml
# Number of days to show, starting today.
days: 2

# Calendars to read. URLs may be http(s) or webcal:// ICS feeds.
calendars:
  - name: Family
    url: webcal://p40-caldav.icloud.com/published/2/...
  - name: Work
    url: https://example.com/calendar.ics
```

## Usage

```sh
cal               # next 2 days (or whatever `days` is set to)
cal -days 7       # override the window
cal -config ./other.yaml
cal -version
```

| Flag       | Description                                         |
| ---------- | --------------------------------------------------- |
| `-days N`  | Days to show, starting today. Overrides config.     |
| `-config`  | Path to the YAML config. Overrides the XDG default. |
| `-version` | Print version and exit.                             |

Precedence for the window: `-days` flag › config `days` › built-in default (2).
