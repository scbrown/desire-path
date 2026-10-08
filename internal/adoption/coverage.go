package adoption

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// Coverage qualifies outcome rates with the source offers that actually emitted.
// Malformed lines are whole-file counts; they cannot be assigned to a window
// whose timestamp they do not safely carry.
type Coverage struct {
	SourceOffers int `json:"source_offers"`
	Registered   int `json:"registered"`
	Missing      int `json:"missing"`
	DuplicateIDs int `json:"duplicate_ids"`
	Malformed    int `json:"malformed_whole_file"`
	Oversized    int `json:"oversized_whole_file"`
}

// Coverage reads the original log without reconstructing use from transcripts.
// A damaged or oversized line is counted and skipped, not turned into EOF.
func (s *Store) Coverage(ctx context.Context, path string, start, end time.Time) (Coverage, error) {
	out := Coverage{}
	f, err := os.Open(path)
	if err != nil {
		return out, errors.New("source offer log unavailable")
	}
	defer f.Close()
	r := bufio.NewReader(f)
	seen := map[string]bool{}
	for {
		if ctx.Err() != nil {
			return out, errors.New("source coverage incomplete: deadline")
		}
		line, oversized, readErr := boundedLine(r, 1<<20)
		if len(line) == 0 && !oversized && readErr == io.EOF {
			break
		}
		if readErr != nil && readErr != io.EOF {
			return out, errors.New("source coverage read failed")
		}
		if oversized {
			out.Malformed++
			out.Oversized++
		} else {
			var offer struct {
				ID        string    `json:"event_id"`
				Timestamp time.Time `json:"timestamp"`
				Shown     bool      `json:"signpost_shown"`
				Paths     []string  `json:"payload_paths"`
			}
			if json.Unmarshal(line, &offer) != nil {
				out.Malformed++
			} else if offer.Shown && len(offer.Paths) > 0 {
				if offer.ID == "" || offer.Timestamp.IsZero() {
					out.Malformed++
				} else if !offer.Timestamp.Before(start) && offer.Timestamp.Before(end) {
					id := s.hash("event", offer.ID)
					if seen[id] {
						out.DuplicateIDs++
					} else {
						seen[id] = true
						out.SourceOffers++
						if len(seen) > maxEvents {
							return out, errors.New("source coverage exceeds bounded experiment size")
						}
						var found int
						err := s.db.QueryRowContext(ctx, `SELECT 1 FROM payloads WHERE id=?`, id).Scan(&found)
						if err == sql.ErrNoRows {
							out.Missing++
						} else if err != nil {
							return out, errors.New("source coverage query failed")
						} else {
							out.Registered++
						}
					}
				}
			}
		}
		if readErr == io.EOF {
			break
		}
	}
	return out, nil
}

func boundedLine(r *bufio.Reader, max int) ([]byte, bool, error) {
	var line []byte
	oversized := false
	for {
		chunk, err := r.ReadSlice('\n')
		if !oversized {
			if len(line)+len(chunk) > max {
				oversized = true
				line = nil
			} else {
				line = append(line, chunk...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return line, oversized, err
	}
}
