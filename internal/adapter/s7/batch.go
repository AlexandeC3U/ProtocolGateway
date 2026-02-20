// Package s7 provides S7 connection pooling with circuit breaker protection.
package s7

import (
	"sort"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// S7BatchConfig configures the address-based range merging for batch reads.
type S7BatchConfig struct {
	// MaxBytesPerRange is the maximum contiguous bytes to read in one PDU item.
	// Larger ranges read more bytes but reduce the number of PDU items.
	// Default: 1024 bytes. S7 protocol allows up to ~65535 bytes per item.
	MaxBytesPerRange int

	// MaxGapBytes is the maximum gap (in bytes) between two tags that will still
	// be merged into a single contiguous read. Reading a few extra unused bytes
	// is cheaper than using an extra PDU item.
	// Default: 32 bytes.
	MaxGapBytes int
}

// DefaultS7BatchConfig returns a S7BatchConfig with sensible defaults.
func DefaultS7BatchConfig() S7BatchConfig {
	return S7BatchConfig{
		MaxBytesPerRange: 1024,
		MaxGapBytes:      32,
	}
}

// s7ByteRange represents a contiguous byte range within a single S7 area+DB.
type s7ByteRange struct {
	area        domain.S7Area
	dbNumber    int
	startOffset int // first byte offset (inclusive)
	totalBytes  int // number of bytes to read
	tags        []s7TagInRange
}

// s7TagInRange tracks a tag's position within its parent byte range.
type s7TagInRange struct {
	tag       *domain.Tag
	offset    int // byte offset relative to range start (tag.offset - range.startOffset)
	bitOffset int
	byteCount int
}

// s7ParsedTag holds a tag with its pre-parsed address components.
type s7ParsedTag struct {
	tag       *domain.Tag
	area      domain.S7Area
	dbNumber  int
	offset    int
	bitOffset int
	byteCount int
}

// s7GroupKey is the grouping key for tags: same area and DB number.
type s7GroupKey struct {
	area     domain.S7Area
	dbNumber int
}

// buildS7ContiguousRanges groups parsed tags by (area, dbNumber), sorts by offset,
// and merges nearby tags into contiguous byte ranges using gap-filling.
//
// This is the S7 equivalent of Modbus buildContiguousRanges().
func buildS7ContiguousRanges(parsed []s7ParsedTag, config S7BatchConfig) []s7ByteRange {
	if len(parsed) == 0 {
		return nil
	}

	// Step 1: Group by (area, dbNumber)
	groups := make(map[s7GroupKey][]s7ParsedTag)
	for _, p := range parsed {
		key := s7GroupKey{area: p.area, dbNumber: p.dbNumber}
		groups[key] = append(groups[key], p)
	}

	var ranges []s7ByteRange

	// Step 2: For each group, sort by offset and merge
	for key, tags := range groups {
		// Sort by byte offset (ascending)
		sort.Slice(tags, func(i, j int) bool {
			return tags[i].offset < tags[j].offset
		})

		// Merge into contiguous ranges
		current := s7ByteRange{
			area:        key.area,
			dbNumber:    key.dbNumber,
			startOffset: tags[0].offset,
			totalBytes:  tags[0].byteCount,
			tags: []s7TagInRange{{
				tag:       tags[0].tag,
				offset:    0, // relative to range start
				bitOffset: tags[0].bitOffset,
				byteCount: tags[0].byteCount,
			}},
		}

		for i := 1; i < len(tags); i++ {
			t := tags[i]
			tagEnd := t.offset + t.byteCount
			currentEnd := current.startOffset + current.totalBytes

			gap := t.offset - currentEnd

			// Would merging exceed the max range size?
			newTotalBytes := tagEnd - current.startOffset
			if newTotalBytes < current.totalBytes {
				newTotalBytes = current.totalBytes
			}

			if gap <= config.MaxGapBytes && newTotalBytes <= config.MaxBytesPerRange {
				// Merge: extend the range
				if tagEnd > currentEnd {
					current.totalBytes = tagEnd - current.startOffset
				}
				current.tags = append(current.tags, s7TagInRange{
					tag:       t.tag,
					offset:    t.offset - current.startOffset,
					bitOffset: t.bitOffset,
					byteCount: t.byteCount,
				})
			} else {
				// Start a new range
				ranges = append(ranges, current)
				current = s7ByteRange{
					area:        key.area,
					dbNumber:    key.dbNumber,
					startOffset: t.offset,
					totalBytes:  t.byteCount,
					tags: []s7TagInRange{{
						tag:       t.tag,
						offset:    0,
						bitOffset: t.bitOffset,
						byteCount: t.byteCount,
					}},
				}
			}
		}
		ranges = append(ranges, current)
	}

	return ranges
}
