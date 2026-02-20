package s7

import (
	"testing"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

func TestBuildS7ContiguousRanges_Empty(t *testing.T) {
	ranges := buildS7ContiguousRanges(nil, DefaultS7BatchConfig())
	if ranges != nil {
		t.Errorf("expected nil, got %d ranges", len(ranges))
	}
}

func TestBuildS7ContiguousRanges_SingleTag(t *testing.T) {
	tag := &domain.Tag{ID: "t1", DataType: domain.DataTypeInt16}
	parsed := []s7ParsedTag{
		{tag: tag, area: domain.S7AreaDB, dbNumber: 1, offset: 10, byteCount: 2},
	}

	ranges := buildS7ContiguousRanges(parsed, DefaultS7BatchConfig())

	if len(ranges) != 1 {
		t.Fatalf("expected 1 range, got %d", len(ranges))
	}
	r := ranges[0]
	if r.area != domain.S7AreaDB || r.dbNumber != 1 {
		t.Errorf("wrong area/db: %s/%d", r.area, r.dbNumber)
	}
	if r.startOffset != 10 || r.totalBytes != 2 {
		t.Errorf("wrong range: start=%d total=%d", r.startOffset, r.totalBytes)
	}
	if len(r.tags) != 1 {
		t.Fatalf("expected 1 tag in range, got %d", len(r.tags))
	}
	if r.tags[0].offset != 0 {
		t.Errorf("expected relative offset 0, got %d", r.tags[0].offset)
	}
}

func TestBuildS7ContiguousRanges_MergeAdjacent(t *testing.T) {
	// Two adjacent tags in DB1: offset 0 (2 bytes) and offset 2 (2 bytes)
	tags := []s7ParsedTag{
		{tag: &domain.Tag{ID: "t1"}, area: domain.S7AreaDB, dbNumber: 1, offset: 0, byteCount: 2},
		{tag: &domain.Tag{ID: "t2"}, area: domain.S7AreaDB, dbNumber: 1, offset: 2, byteCount: 2},
	}

	ranges := buildS7ContiguousRanges(tags, DefaultS7BatchConfig())

	if len(ranges) != 1 {
		t.Fatalf("expected 1 merged range, got %d", len(ranges))
	}
	r := ranges[0]
	if r.startOffset != 0 || r.totalBytes != 4 {
		t.Errorf("expected range [0, 4 bytes], got [%d, %d bytes]", r.startOffset, r.totalBytes)
	}
	if len(r.tags) != 2 {
		t.Fatalf("expected 2 tags in range, got %d", len(r.tags))
	}
	// First tag at relative offset 0, second at relative offset 2
	if r.tags[0].offset != 0 || r.tags[1].offset != 2 {
		t.Errorf("wrong relative offsets: %d, %d", r.tags[0].offset, r.tags[1].offset)
	}
}

func TestBuildS7ContiguousRanges_MergeWithGap(t *testing.T) {
	// Two tags with a 10-byte gap (within default MaxGapBytes=32)
	tags := []s7ParsedTag{
		{tag: &domain.Tag{ID: "t1"}, area: domain.S7AreaDB, dbNumber: 1, offset: 0, byteCount: 4},
		{tag: &domain.Tag{ID: "t2"}, area: domain.S7AreaDB, dbNumber: 1, offset: 14, byteCount: 4},
	}

	ranges := buildS7ContiguousRanges(tags, DefaultS7BatchConfig())

	if len(ranges) != 1 {
		t.Fatalf("expected 1 merged range (gap=10 <= 32), got %d", len(ranges))
	}
	r := ranges[0]
	// Range should span from 0 to 14+4=18
	if r.startOffset != 0 || r.totalBytes != 18 {
		t.Errorf("expected range [0, 18 bytes], got [%d, %d bytes]", r.startOffset, r.totalBytes)
	}
}

func TestBuildS7ContiguousRanges_SplitOnLargeGap(t *testing.T) {
	// Two tags with a 50-byte gap (exceeds default MaxGapBytes=32)
	tags := []s7ParsedTag{
		{tag: &domain.Tag{ID: "t1"}, area: domain.S7AreaDB, dbNumber: 1, offset: 0, byteCount: 4},
		{tag: &domain.Tag{ID: "t2"}, area: domain.S7AreaDB, dbNumber: 1, offset: 54, byteCount: 4},
	}

	ranges := buildS7ContiguousRanges(tags, DefaultS7BatchConfig())

	if len(ranges) != 2 {
		t.Fatalf("expected 2 separate ranges (gap=50 > 32), got %d", len(ranges))
	}
	if ranges[0].startOffset != 0 || ranges[0].totalBytes != 4 {
		t.Errorf("range[0]: expected [0, 4], got [%d, %d]", ranges[0].startOffset, ranges[0].totalBytes)
	}
	if ranges[1].startOffset != 54 || ranges[1].totalBytes != 4 {
		t.Errorf("range[1]: expected [54, 4], got [%d, %d]", ranges[1].startOffset, ranges[1].totalBytes)
	}
}

func TestBuildS7ContiguousRanges_SplitOnMaxBytes(t *testing.T) {
	// Two tags where merging would exceed MaxBytesPerRange
	config := S7BatchConfig{MaxBytesPerRange: 20, MaxGapBytes: 100}
	tags := []s7ParsedTag{
		{tag: &domain.Tag{ID: "t1"}, area: domain.S7AreaDB, dbNumber: 1, offset: 0, byteCount: 8},
		{tag: &domain.Tag{ID: "t2"}, area: domain.S7AreaDB, dbNumber: 1, offset: 15, byteCount: 8},
	}

	ranges := buildS7ContiguousRanges(tags, config)

	// Merged would be 23 bytes (0 to 23), exceeds MaxBytesPerRange=20
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges (merged=23 > max=20), got %d", len(ranges))
	}
}

func TestBuildS7ContiguousRanges_DifferentAreas(t *testing.T) {
	// Tags in different areas should never merge
	tags := []s7ParsedTag{
		{tag: &domain.Tag{ID: "db1"}, area: domain.S7AreaDB, dbNumber: 1, offset: 0, byteCount: 2},
		{tag: &domain.Tag{ID: "m1"}, area: domain.S7AreaM, dbNumber: 0, offset: 0, byteCount: 2},
	}

	ranges := buildS7ContiguousRanges(tags, DefaultS7BatchConfig())

	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges (different areas), got %d", len(ranges))
	}
}

func TestBuildS7ContiguousRanges_DifferentDBs(t *testing.T) {
	// Tags in different DBs should never merge
	tags := []s7ParsedTag{
		{tag: &domain.Tag{ID: "db1"}, area: domain.S7AreaDB, dbNumber: 1, offset: 0, byteCount: 2},
		{tag: &domain.Tag{ID: "db2"}, area: domain.S7AreaDB, dbNumber: 2, offset: 0, byteCount: 2},
	}

	ranges := buildS7ContiguousRanges(tags, DefaultS7BatchConfig())

	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges (different DBs), got %d", len(ranges))
	}
}

func TestBuildS7ContiguousRanges_UnsortedInput(t *testing.T) {
	// Tags given in reverse order should still merge correctly
	tags := []s7ParsedTag{
		{tag: &domain.Tag{ID: "t3"}, area: domain.S7AreaDB, dbNumber: 1, offset: 8, byteCount: 4},
		{tag: &domain.Tag{ID: "t1"}, area: domain.S7AreaDB, dbNumber: 1, offset: 0, byteCount: 2},
		{tag: &domain.Tag{ID: "t2"}, area: domain.S7AreaDB, dbNumber: 1, offset: 4, byteCount: 4},
	}

	ranges := buildS7ContiguousRanges(tags, DefaultS7BatchConfig())

	if len(ranges) != 1 {
		t.Fatalf("expected 1 merged range, got %d", len(ranges))
	}
	r := ranges[0]
	if r.startOffset != 0 || r.totalBytes != 12 {
		t.Errorf("expected range [0, 12 bytes], got [%d, %d bytes]", r.startOffset, r.totalBytes)
	}
	if len(r.tags) != 3 {
		t.Fatalf("expected 3 tags in range, got %d", len(r.tags))
	}
}

func TestBuildS7ContiguousRanges_OverlappingTags(t *testing.T) {
	// Tags at same offset (e.g., bool at DB1.DBX0.0 and int16 at DB1.DBW0)
	tags := []s7ParsedTag{
		{tag: &domain.Tag{ID: "bool"}, area: domain.S7AreaDB, dbNumber: 1, offset: 0, bitOffset: 0, byteCount: 1},
		{tag: &domain.Tag{ID: "word"}, area: domain.S7AreaDB, dbNumber: 1, offset: 0, bitOffset: 0, byteCount: 2},
	}

	ranges := buildS7ContiguousRanges(tags, DefaultS7BatchConfig())

	if len(ranges) != 1 {
		t.Fatalf("expected 1 range for overlapping tags, got %d", len(ranges))
	}
	r := ranges[0]
	if r.totalBytes != 2 {
		t.Errorf("expected 2 total bytes (larger tag), got %d", r.totalBytes)
	}
	if len(r.tags) != 2 {
		t.Errorf("expected 2 tags in range, got %d", len(r.tags))
	}
}

func TestBuildS7ContiguousRanges_ManyTagsSameDB(t *testing.T) {
	// Simulate 10 consecutive Int16 tags in DB1, offsets 0, 2, 4, ..., 18
	var tags []s7ParsedTag
	for i := 0; i < 10; i++ {
		tags = append(tags, s7ParsedTag{
			tag:       &domain.Tag{ID: "t" + string(rune('0'+i))},
			area:      domain.S7AreaDB,
			dbNumber:  1,
			offset:    i * 2,
			byteCount: 2,
		})
	}

	ranges := buildS7ContiguousRanges(tags, DefaultS7BatchConfig())

	// All 10 tags are contiguous: 0..20 = 20 bytes, well within 1024 limit
	if len(ranges) != 1 {
		t.Fatalf("expected 1 merged range for 10 contiguous tags, got %d", len(ranges))
	}
	if ranges[0].totalBytes != 20 {
		t.Errorf("expected 20 total bytes, got %d", ranges[0].totalBytes)
	}
	if len(ranges[0].tags) != 10 {
		t.Errorf("expected 10 tags in range, got %d", len(ranges[0].tags))
	}
}
