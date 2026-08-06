package main

import (
	"bufio"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// horizonHours bounds everything this module remembers. The status line only
// ever reports "today" and the current 5-hour block, so anything older is dead
// weight — and bounding it is what keeps the state file and the seen-set from
// growing without limit.
const horizonHours = 48

const costStateVersion = 1

// costState is the incremental scan's memory between renders.
//
// Files are keyed by path with the size and mtime last consumed, so an unchanged
// file is skipped on a stat alone and a grown file is read from its previous
// offset. Hours holds per-model token totals bucketed by the hour they landed
// in, which is granular enough to answer both "today" and "this 5h block" while
// staying tiny. Seen exists because transcripts repeat entries — 2044 assistant
// lines in this install collapse to 959 unique responses, so without it every
// figure would roughly double.
type costState struct {
	Version int                   `json:"version"`
	Files   map[string]fileCursor `json:"files"`
	Hours   map[string]hourBucket `json:"hours"`
	Seen    map[string]int64      `json:"seen"`
}

// fileCursor separates "how far we have parsed" from "how big the file was".
//
// Cursor advances only past complete, newline-terminated lines. Size is the file
// length we observed. They differ whenever a render lands between the writer
// emitting a record and emitting its newline — using Size as the resume point
// there would skip past the half-written line and lose that entry permanently.
type fileCursor struct {
	Cursor int64 `json:"cursor"`
	Size   int64 `json:"size"`
	MTime  int64 `json:"mtime"`
}

type hourBucket map[string]TokenCounts // model -> tokens

func newCostState() *costState {
	return &costState{
		Version: costStateVersion,
		Files:   map[string]fileCursor{},
		Hours:   map[string]hourBucket{},
		Seen:    map[string]int64{},
	}
}

func loadCostState(path string) *costState {
	data, err := os.ReadFile(path)
	if err != nil {
		return newCostState()
	}
	var st costState
	if err := json.Unmarshal(data, &st); err != nil || st.Version != costStateVersion {
		return newCostState()
	}
	if st.Files == nil {
		st.Files = map[string]fileCursor{}
	}
	if st.Hours == nil {
		st.Hours = map[string]hourBucket{}
	}
	if st.Seen == nil {
		st.Seen = map[string]int64{}
	}
	return &st
}

// save writes atomically: a torn state file would be silently discarded on the
// next render and force a full rescan.
func (st *costState) save(path string) {
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	tmp := path + "." + strconv.Itoa(os.Getpid()) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
	}
}

func (st *costState) prune(horizon int64) {
	for k := range st.Hours {
		if h, err := strconv.ParseInt(k, 10, 64); err == nil && h < horizon {
			delete(st.Hours, k)
		}
	}
	for k, h := range st.Seen {
		if h < horizon {
			delete(st.Seen, k)
		}
	}
}

// transcriptEntry is the narrow slice of an assistant line that matters. Unknown
// fields are ignored, so Claude adding to the format cannot break the scan.
type transcriptEntry struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	RequestID string `json:"requestId"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheCreation            *struct {
				Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
				Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		} `json:"usage"`
	} `json:"message"`
}

// scan brings the state up to date with everything written since last render.
func (st *costState) scan(root string, now time.Time) {
	horizon := now.Add(-horizonHours * time.Hour).Unix()
	horizonHour := horizon / 3600 * 3600

	files := findTranscripts(root)
	live := make(map[string]bool, len(files))

	for _, path := range files {
		live[path] = true
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		prev, known := st.Files[path]

		// A file untouched since before the horizon cannot hold a relevant
		// entry. Record where it ends and never read it — this is what keeps
		// even the very first run off the full 19.6MB.
		if fi.ModTime().Unix() < horizon {
			st.Files[path] = fileCursor{Cursor: fi.Size(), Size: fi.Size(), MTime: fi.ModTime().Unix()}
			continue
		}

		if known && prev.Size == fi.Size() && prev.MTime == fi.ModTime().Unix() {
			continue // unchanged since last render
		}

		offset := int64(0)
		if known && fi.Size() >= prev.Cursor {
			offset = prev.Cursor
		}
		// A shrunk file was rewritten; offset stays 0 and it is read in full.

		consumed := st.consume(path, offset, horizonHour)
		st.Files[path] = fileCursor{
			Cursor: consumed,
			Size:   fi.Size(),
			MTime:  fi.ModTime().Unix(),
		}
	}

	// Forget transcripts that no longer exist.
	for path := range st.Files {
		if !live[path] {
			delete(st.Files, path)
		}
	}

	st.prune(horizonHour)
}

// consume parses from offset to EOF, folding usage into hour buckets. It
// returns the offset of the end of the last complete line, which becomes the
// next render's resume point.
func (st *costState) consume(path string, offset, horizonHour int64) int64 {
	f, err := os.Open(path)
	if err != nil {
		return offset
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return offset
		}
	}

	// Transcript lines carry full message content and get very large, so read
	// them explicitly rather than with a Scanner's bounded token buffer.
	r := bufio.NewReaderSize(f, 64*1024)
	consumed := offset

	for {
		raw, err := r.ReadString('\n')
		if err != nil && len(raw) == 0 {
			break
		}
		complete := err == nil // a trailing chunk without '\n' is still being written
		if !complete {
			break
		}
		consumed += int64(len(raw))

		line := []byte(strings.TrimRight(raw, "\r\n"))
		if len(line) == 0 {
			continue
		}
		var e transcriptEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue // a partial or malformed line is skipped, never fatal
		}
		if e.Type != "assistant" || e.Message.Usage == nil {
			continue
		}

		ts, err := time.Parse(time.RFC3339Nano, e.Timestamp)
		if err != nil {
			continue
		}
		hour := ts.Unix() / 3600 * 3600
		if hour < horizonHour {
			continue // older than we care about
		}

		// Same API response can appear many times across and within files.
		key := e.Message.ID + "|" + e.RequestID
		if _, dup := st.Seen[key]; dup {
			continue
		}
		st.Seen[key] = hour

		u := e.Message.Usage
		tc := TokenCounts{
			Input:     u.InputTokens,
			Output:    u.OutputTokens,
			CacheRead: u.CacheReadInputTokens,
		}
		if u.CacheCreation != nil {
			tc.CacheWrite5m = u.CacheCreation.Ephemeral5m
			tc.CacheWrite1h = u.CacheCreation.Ephemeral1h
		} else {
			// Older entries only report the total; treat it as 5m, the cheaper
			// and far more common tier, rather than inventing a split.
			tc.CacheWrite5m = u.CacheCreationInputTokens
		}

		bucketKey := strconv.FormatInt(hour, 10)
		bucket := st.Hours[bucketKey]
		if bucket == nil {
			bucket = hourBucket{}
			st.Hours[bucketKey] = bucket
		}
		model := e.Message.Model
		if model == "" {
			model = "<unknown>"
		}
		existing := bucket[model]
		existing.add(tc)
		bucket[model] = existing
	}

	return consumed
}

func findTranscripts(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, never fail the render
		}
		if !d.IsDir() && filepath.Ext(path) == ".jsonl" {
			out = append(out, path)
		}
		return nil
	})
	return out
}
