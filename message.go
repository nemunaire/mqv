package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/ianaindex"
)

// MessagePart holds metadata and raw decoded bytes for a single MIME leaf part.
type MessagePart struct {
	Index     int
	MediaType string
	CT        string // full Content-Type header value (for charset info)
	Name      string
	Size      int
	Data      []byte
}

// extractParts parses raw EML and returns a flat list of all MIME leaf parts.
func extractParts(raw string) []MessagePart {
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		return nil
	}
	ct := msg.Header.Get("Content-Type")
	cte := strings.ToLower(strings.TrimSpace(msg.Header.Get("Content-Transfer-Encoding")))
	cd := msg.Header.Get("Content-Disposition")
	var parts []MessagePart
	counter := 0
	collectParts(ct, cte, cd, msg.Body, &parts, &counter)
	return parts
}

func collectParts(ct, cte, cd string, r io.Reader, out *[]MessagePart, n *int) {
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		mediaType = "text/plain"
		params = map[string]string{}
	}

	var decoded io.Reader
	switch cte {
	case "quoted-printable":
		decoded = quotedprintable.NewReader(r)
	case "base64":
		decoded = base64.NewDecoder(base64.StdEncoding, r)
	default:
		decoded = r
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return
		}
		mr := multipart.NewReader(decoded, boundary)
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			partCT := p.Header.Get("Content-Type")
			partCTE := strings.ToLower(strings.TrimSpace(p.Header.Get("Content-Transfer-Encoding")))
			partCD := p.Header.Get("Content-Disposition")
			collectParts(partCT, partCTE, partCD, p, out, n)
		}
		return
	}

	data, _ := io.ReadAll(decoded)
	*n++
	*out = append(*out, MessagePart{
		Index:     *n,
		MediaType: mediaType,
		CT:        ct,
		Name:      partName(params, cd),
		Size:      len(data),
		Data:      data,
	})
}

// savePart writes data to the named file in the current working directory.
func savePart(data []byte, name string) (string, error) {
	if name == "" {
		name = "attachment"
	}
	path := filepath.Base(name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

var wordDecoder = &mime.WordDecoder{
	CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
		// Fast path for the most common legacy charsets
		switch strings.ToLower(charset) {
		case "iso-8859-1", "latin-1":
			return charmap.ISO8859_1.NewDecoder().Reader(input), nil
		case "windows-1252", "cp1252":
			return charmap.Windows1252.NewDecoder().Reader(input), nil
		}
		// Fall back to IANA index for everything else
		enc, err := ianaindex.MIME.Encoding(charset)
		if err != nil {
			return nil, fmt.Errorf("unsupported charset %q: %w", charset, err)
		}
		return enc.NewDecoder().Reader(input), nil
	},
}

// fetchHeaders runs postcat -qh and returns only the headers.
func fetchHeaders(id string) (string, error) {
	out, err := exec.Command("postcat", "-qh", id).Output()
	if err != nil {
		return "", fmt.Errorf("postcat -qh %s: %w", id, err)
	}
	return string(out), nil
}

// fetchMessage runs postcat -qbh and returns the raw EML content.
func fetchMessage(id string) (string, error) {
	out, err := exec.Command("postcat", "-qbh", id).Output()
	if err != nil {
		return "", fmt.Errorf("postcat -qbh %s: %w", id, err)
	}
	return string(out), nil
}

// extractSubject scans raw EML output for the Subject field and decodes it.
// It handles folded headers (RFC 2822 continuation lines) and RFC 2047 encoded words.
// We do not stop at blank lines because postcat output may include envelope records
// separated from the RFC 2822 headers by a blank line.
func extractSubject(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(strings.ToLower(line), "subject:") {
			continue
		}
		value := line[len("subject:"):]
		// Collect folded continuation lines (start with whitespace)
		for j := i + 1; j < len(lines); j++ {
			if len(lines[j]) > 0 && (lines[j][0] == ' ' || lines[j][0] == '\t') {
				value += " " + strings.TrimSpace(lines[j])
			} else {
				break
			}
		}
		value = strings.TrimSpace(value)
		if decoded, err := wordDecoder.DecodeHeader(value); err == nil {
			return decoded
		}
		return value
	}
	return "(no subject)"
}

// renderMessage parses raw EML and returns a human-readable string suitable
// for display in the viewport. Headers are RFC 2047 decoded, and the body is
// decoded from quoted-printable or base64 transfer encoding.
// When fullHeaders is true all headers are shown (sorted); otherwise only the
// curated subset (Date, From, To, Cc, Reply-To, Subject) is displayed.
func renderMessage(raw string, fullHeaders bool) string {
	msg, err := mail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		// postcat may prefix envelope lines before RFC 2822 headers; fall back.
		return raw
	}

	var sb strings.Builder

	if fullHeaders {
		names := make([]string, 0, len(msg.Header))
		for name := range msg.Header {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			for _, val := range msg.Header[name] {
				decoded, err := wordDecoder.DecodeHeader(val)
				if err != nil {
					decoded = val
				}
				sb.WriteString(name)
				sb.WriteString(": ")
				sb.WriteString(decoded)
				sb.WriteByte('\n')
			}
		}
	} else {
		for _, name := range []string{"Date", "From", "To", "Cc", "Reply-To", "Subject"} {
			val := msg.Header.Get(name)
			if val == "" {
				continue
			}
			decoded, err := wordDecoder.DecodeHeader(val)
			if err != nil {
				decoded = val
			}
			sb.WriteString(headerKeyStyle.Render(name + ":"))
			sb.WriteString(" ")
			sb.WriteString(decoded)
			sb.WriteByte('\n')
		}
	}
	sb.WriteByte('\n')

	// Decode and write the body.
	ct := msg.Header.Get("Content-Type")
	cte := strings.ToLower(strings.TrimSpace(msg.Header.Get("Content-Transfer-Encoding")))
	cd := msg.Header.Get("Content-Disposition")
	body := renderPart(ct, cte, cd, msg.Body)
	sb.WriteString(body)

	return sb.String()
}

// renderPart decodes a single MIME part (or the top-level message body) given
// its Content-Type, Content-Transfer-Encoding, and Content-Disposition header values.
func renderPart(ct, cte, cd string, r io.Reader) string {
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		mediaType = "text/plain"
		params = map[string]string{}
	}

	// Decode transfer encoding first.
	var decoded io.Reader
	switch cte {
	case "quoted-printable":
		decoded = quotedprintable.NewReader(r)
	case "base64":
		decoded = base64.NewDecoder(base64.StdEncoding, r)
	default:
		decoded = r
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return "[multipart message with missing boundary]\n"
		}
		return renderMultipart(multipart.NewReader(decoded, boundary), mediaType)
	}

	// Non-text, non-multipart parts are binary attachments — show a placeholder.
	if !strings.HasPrefix(mediaType, "text/") {
		data, _ := io.ReadAll(decoded)
		name := partName(params, cd)
		size := len(data)
		label := fmt.Sprintf("\n[ Content-Type: %s", mediaType)
		if name != "" {
			label += " - Filename: " + name
		}
		if size > 0 {
			label += fmt.Sprintf(" - Size: %s", formatSize(size))
		}
		label += " ]"
		return attachStyle.Render(label) + "\n\n"
	}

	// Decode charset for text parts.
	if charset, ok := params["charset"]; ok && charset != "" {
		if cr, err := wordDecoder.CharsetReader(charset, decoded); err == nil {
			decoded = cr
		}
	}

	data, err := io.ReadAll(decoded)
	if err != nil {
		return fmt.Sprintf("[error reading body: %v]\n", err)
	}

	if mediaType == "text/html" {
		return renderHTML(string(data))
	}
	return string(data)
}

// partName extracts a filename/name from Content-Type params or Content-Disposition.
func partName(ctParams map[string]string, cd string) string {
	if n := ctParams["name"]; n != "" {
		if decoded, err := wordDecoder.DecodeHeader(n); err == nil {
			return decoded
		}
		return n
	}
	if cd != "" {
		if _, cdParams, err := mime.ParseMediaType(cd); err == nil {
			if fn := cdParams["filename"]; fn != "" {
				if decoded, err := wordDecoder.DecodeHeader(fn); err == nil {
					return decoded
				}
				return fn
			}
		}
	}
	return ""
}

// formatSize formats a byte count as a human-readable string.
func formatSize(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f kB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// renderMultipart walks multipart parts, preferring text/plain. Nested
// multipart/* is handled recursively. Parts in mixed/related messages are
// separated by a colored rule.
func renderMultipart(mr *multipart.Reader, parentType string) string {
	type candidate struct {
		mediaType string
		content   string
	}
	var parts []candidate

	for {
		p, err := mr.NextPart()
		if err != nil { // io.EOF or real error
			break
		}
		partCT := p.Header.Get("Content-Type")
		partCTE := strings.ToLower(strings.TrimSpace(p.Header.Get("Content-Transfer-Encoding")))
		partCD := p.Header.Get("Content-Disposition")
		text := renderPart(partCT, partCTE, partCD, p)

		mt, _, _ := mime.ParseMediaType(partCT)
		parts = append(parts, candidate{mt, text})
	}

	if parentType == "multipart/alternative" {
		// Prefer plain text.
		for _, c := range parts {
			if c.mediaType == "text/plain" {
				return c.content
			}
		}
		// Fall back to first non-empty part.
		for _, c := range parts {
			if strings.TrimSpace(c.content) != "" {
				return c.content
			}
		}
		return ""
	}

	// For mixed/related/etc, join non-empty parts with a colored separator.
	var sb strings.Builder
	first := true
	for _, c := range parts {
		if strings.TrimSpace(c.content) == "" {
			continue
		}
		if !first {
			label := "\n── " + c.mediaType + " "
			rule := label + strings.Repeat("─", max(0, 60-len(label)))
			sb.WriteString(partSepStyle.Render(rule))
			sb.WriteByte('\n')
		}
		sb.WriteString(c.content)
		if !strings.HasSuffix(c.content, "\n") {
			sb.WriteByte('\n')
		}
		first = false
	}
	return sb.String()
}

// renderHTML renders HTML content to plain text using w3m if available,
// otherwise returns the raw HTML.
func renderHTML(html string) string {
	w3m, err := exec.LookPath("w3m")
	if err != nil {
		return html
	}

	f, err := os.CreateTemp("", "postfixmutt-*.html")
	if err != nil {
		return html
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(html); err != nil {
		f.Close()
		return html
	}
	f.Close()

	out, err := exec.Command(w3m, "-T", "text/html", "-dump", "-o", "display_link_number=1", f.Name()).Output()
	if err != nil {
		return html
	}
	return string(out)
}

