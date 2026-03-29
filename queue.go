package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

type QueueEntry struct {
	ID      string
	OnHold  bool
	Size    int
	Date    time.Time
	Sender  string
	Subject string
	Rcpts   []string
	Reason  string
}

func loadQueue() ([]QueueEntry, error) {
	out, err := exec.Command("postqueue", "-p").Output()
	if err != nil {
		return nil, fmt.Errorf("postqueue -p: %w", err)
	}
	entries := parsePostqueue(string(out))
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date.After(entries[j].Date)
	})
	return entries, nil
}

// parsePostqueue parses the output of `postqueue -p`.
// Each message block is separated by a blank line.
// Format of first line of a block:
//
//	QUEUEID[*!]?  SIZE  DOW MON DD HH:MM:SS  SENDER
func parsePostqueue(output string) []QueueEntry {
	var entries []QueueEntry

	scanner := bufio.NewScanner(strings.NewReader(output))

	// Skip header line
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "-Queue ID-") {
			break
		}
	}

	var current *QueueEntry
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if current != nil {
				entries = append(entries, *current)
				current = nil
			}
			continue
		}

		// Lines starting with a space are reason or recipient lines
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if current == nil {
				continue
			}
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "(") {
				current.Reason = strings.Trim(trimmed, "()")
			} else if trimmed != "" {
				current.Rcpts = append(current.Rcpts, trimmed)
			}
			continue
		}

		// Deferral/error reason line (not indented, starts with '(')
		if strings.HasPrefix(line, "(") {
			if current != nil {
				current.Reason = strings.Trim(line, "()")
			}
			continue
		}

		// Skip summary line at the end (e.g. "-- 2 Kbytes in 1 Request.")
		if strings.HasPrefix(line, "--") {
			if current != nil {
				entries = append(entries, *current)
				current = nil
			}
			break
		}

		// New message block: parse the header line
		entry := parseQueueLine(line)
		if entry != nil {
			current = entry
		}
	}

	if current != nil {
		entries = append(entries, *current)
	}

	return entries
}

// parseQueueLine parses a line like:
// ABC1234*      1234 Sun Mar 29 10:00:00  sender@example.com
func parseQueueLine(line string) *QueueEntry {
	fields := strings.Fields(line)
	// Minimum: ID, size, weekday, month, day, time, sender
	if len(fields) < 7 {
		return nil
	}

	raw := fields[0]
	onHold := strings.HasSuffix(raw, "!")
	// Strip trailing status character (* or !)
	id := strings.TrimRight(raw, "*!")

	var size int
	fmt.Sscanf(fields[1], "%d", &size)

	// Date: fields[2] = weekday, [3] = month, [4] = day, [5] = HH:MM:SS
	// Use current year as postqueue doesn't show year
	dateStr := fmt.Sprintf("%s %s %s %s %d",
		fields[2], fields[3], fields[4], fields[5], time.Now().Year())
	t, err := time.Parse(time.ANSIC, dateStr)
	if err != nil {
		t = time.Time{}
	}

	sender := ""
	if len(fields) >= 7 {
		sender = fields[6]
	}

	return &QueueEntry{
		ID:     id,
		OnHold: onHold,
		Size:   size,
		Date:   t,
		Sender: sender,
	}
}
