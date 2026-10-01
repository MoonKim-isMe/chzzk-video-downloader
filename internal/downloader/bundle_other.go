//go:build !windows || !amd64

package downloader

func ensureBundledTools() error {
	return nil
}
