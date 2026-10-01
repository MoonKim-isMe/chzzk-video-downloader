//go:build windows && amd64

package downloader

import "embed"

// The Windows build hook populates this directory with verified binaries before compilation.
// Keeping the directory itself in the repository lets ordinary Go tooling compile even before
// the generated binaries are prepared.
//
//go:embed bundled/windows-amd64
var windowsToolBundle embed.FS

func ensureBundledTools() error {
	targetDir, err := managedToolsDir()
	if err != nil {
		return err
	}
	return materializeBundledTools(windowsToolBundle, "bundled/windows-amd64", targetDir)
}
