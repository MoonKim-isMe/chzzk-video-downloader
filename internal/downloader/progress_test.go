package downloader

import "testing"

func TestParseProgressLine(t *testing.T) {
	progress, ok := parseProgressLine(progressPrefix + "downloading\t512\t1024\tNA\t256.5\t2\t 50.0%")
	if !ok {
		t.Fatal("expected progress line")
	}
	if progress.Percent != 50 ||
		progress.DownloadedBytes != 512 ||
		progress.TotalBytes != 1024 ||
		progress.SpeedBytesPerSecond != 256.5 ||
		progress.ETASeconds != 2 ||
		progress.TotalBytesEstimated {
		t.Fatalf("unexpected progress: %#v", progress)
	}
}

func TestParseProgressLineUsesEstimatedTotal(t *testing.T) {
	progress, ok := parseProgressLine(progressPrefix + "downloading\t100\tNA\t1000\tNA\tNA\t10.0%")
	if !ok || progress.TotalBytes != 1000 || !progress.TotalBytesEstimated {
		t.Fatalf("unexpected progress: %#v", progress)
	}
}

func TestParseFinalPath(t *testing.T) {
	path, ok := parseFinalPath(finalPathPrefix + `C:\\Video\\sample.mp4`)
	if !ok || path != `C:\\Video\\sample.mp4` {
		t.Fatalf("unexpected path: %q, ok=%v", path, ok)
	}
}
