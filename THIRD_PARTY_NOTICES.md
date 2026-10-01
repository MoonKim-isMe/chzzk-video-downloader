# Third-Party Download Tools

The Windows amd64 build of CHZZK Video Downloader bundles command-line tools used as separate processes.

## yt-dlp

- Project: yt-dlp
- Source: https://github.com/yt-dlp/yt-dlp
- Windows binary: official `yt-dlp.exe` release asset
- Release files include their applicable third-party licenses.

## FFmpeg / ffprobe

- Project: FFmpeg
- Source: https://ffmpeg.org/
- Windows build provider used by the build script: BtbN FFmpeg-Builds
- Build variant: Windows x86_64 LGPL static build
- Build source: https://github.com/BtbN/FFmpeg-Builds

The build preparation script downloads the release artifacts over HTTPS and verifies each downloaded archive/binary using the SHA-256 checksum published with that release before embedding the executables into the application binary.
