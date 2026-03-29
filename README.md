# mqv — Mail Queue Viewer

A quick-and-dirty TUI for browsing the Postfix deferred queue, built mutt-style.

I wanted this tool for a long time. Claude finally made it happen.


## What it does

`mqv` gives you a keyboard-driven terminal interface to inspect your Postfix mail queue — read messages, browse attachments, save EML files — without leaving the terminal.

```
  ABC1234     Mar 29 10:00   sender@example.com      Subject line here
  DEF5678     Mar 28 14:32   other@example.com       Another message
  ...
```


## Requirements

- Go 1.21+
- `postqueue` and `postcat` in `$PATH` (i.e. Postfix installed)
- Run as a user with read access to the Postfix queue
- Optional: `w3m` for rendering HTML parts as plain text


## Install

```bash
git clone https://github.com/nemunaire/mqv
cd mqv
go build -o mqv .
./mqv
```

Or install directly:

```bash
go install github.com/nemunaire/mqv@latest
```

## Screens & keybindings

### Queue list

| Key        | Action                              |
|------------|-------------------------------------|
| `↑` / `↓` | Navigate messages                   |
| `Enter`    | Open message                        |
| `r`        | Refresh queue                       |
| `F`        | Flush queue (`postqueue -f`)        |
| `q`        | Quit                                |

Subjects are fetched lazily in parallel via `postcat` as the list loads; a progress bar tracks completion.

### Message view

| Key             | Action                          |
|-----------------|---------------------------------|
| `↑↓` / `Space` / `PgUp` / `PgDn` | Scroll      |
| `H`             | Toggle full headers / short headers |
| `s`             | Save raw EML to `~/QUEUEID.eml` |
| `F`             | Requeue message (`postsuper -r`) |
| `v`             | Browse MIME parts               |
| `q`             | Back to queue list              |

Headers are RFC 2047 decoded. The body is decoded from quoted-printable or base64. HTML parts are rendered via `w3m` if available, otherwise shown as-is.

### MIME parts browser

| Key        | Action                              |
|------------|-------------------------------------|
| `↑↓` / `j/k` | Navigate parts                   |
| `Enter`    | View text part inline               |
| `s`        | Save part to file (prompts for name) |
| `q` / `Esc`| Back to message view                |

### Part view

| Key             | Action              |
|-----------------|---------------------|
| `↑↓` / `Space` / `PgUp` / `PgDn` | Scroll  |
| `q` / `Esc`     | Back to parts list  |


## How it works

1. Runs `postqueue -p` and parses the output to build the queue list.
2. For each entry, fetches the subject via `postcat -qh MSGID` in parallel.
3. On `Enter`, loads the full message with `postcat -qbh MSGID` and renders it in a scrollable viewport.
4. MIME parsing is done in pure Go (`mime/multipart`, `mime/quotedprintable`) — no external dependencies for message decoding.
5. Charset decoding handles ISO-8859-1, Windows-1252, and anything in the IANA index via `golang.org/x/text`.


## Stack

- [bubbletea](https://github.com/charmbracelet/bubbletea) — Elm-architecture TUI framework
- [bubbles](https://github.com/charmbracelet/bubbles) — list, viewport, progress, textinput components
- [lipgloss](https://github.com/charmbracelet/lipgloss) — terminal styling
- [x/text](https://pkg.go.dev/golang.org/x/text) — charset decoding


## License

MIT
