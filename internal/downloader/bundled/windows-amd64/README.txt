This directory is populated at Windows build time by scripts/prepare-windows-tools.ps1.

Generated files are intentionally ignored by Git:
- yt-dlp.exe
- ffmpeg.exe
- ffprobe.exe
- bundle-manifest.generated.json

The Windows amd64 Wails pre-build hook downloads and verifies these files before Go compilation.
