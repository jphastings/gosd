package image

import (
	"slices"
	"testing"
)

// A file's content ranges are contiguous in practice — a freshly formatted
// FAT32 volume allocates each boot file in one run — so the fragmented case
// below is the only place the arithmetic that carves gosd.toml's reserved
// [env] region out of them is exercised. Getting it wrong would hand a
// downloader byte ranges that overwrite the rest of the file.
func TestSpanOfRanges(t *testing.T) {
	file := []ByteRange{
		{OffsetBytes: 1000, LengthBytes: 100},
		{OffsetBytes: 5000, LengthBytes: 100},
	}

	for _, tc := range []struct {
		name string
		req  RangeRequest
		want []ByteRange
	}{
		{
			name: "no span asks for the whole file",
			req:  RangeRequest{Path: "gosd.toml"},
			want: file,
		},
		{
			name: "a span inside the first range",
			req:  RangeRequest{Path: "gosd.toml", OffsetBytes: 10, LengthBytes: 20},
			want: []ByteRange{{OffsetBytes: 1010, LengthBytes: 20}},
		},
		{
			name: "a span crossing both ranges",
			req:  RangeRequest{Path: "gosd.toml", OffsetBytes: 90, LengthBytes: 30},
			want: []ByteRange{{OffsetBytes: 1090, LengthBytes: 10}, {OffsetBytes: 5000, LengthBytes: 20}},
		},
		{
			name: "a span starting exactly at the second range",
			req:  RangeRequest{Path: "gosd.toml", OffsetBytes: 100, LengthBytes: 100},
			want: []ByteRange{{OffsetBytes: 5000, LengthBytes: 100}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := spanOfRanges(file, tc.req, 200)
			if err != nil {
				t.Fatalf("spanOfRanges: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("spanOfRanges(%+v) = %+v, want %+v", tc.req, got, tc.want)
			}
		})
	}
}

func TestSpanOfRangesRefusesASpanPastTheFilesContent(t *testing.T) {
	file := []ByteRange{{OffsetBytes: 1000, LengthBytes: 100}}

	if _, err := spanOfRanges(file, RangeRequest{Path: "gosd.toml", OffsetBytes: 50, LengthBytes: 100}, 100); err == nil {
		t.Fatal("spanOfRanges past the end of the file = nil error, want a refusal rather than ranges covering another file's bytes")
	}
}
