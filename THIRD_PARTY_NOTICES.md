# Third-Party Notices

The source code authored for **CHZZK Video Downloader** in this repository is
licensed under the MIT License. See [LICENSE](./LICENSE).

Third-party software, libraries, runtime components, trademarks, services, and
downloaded content are not relicensed by the repository's MIT License. Their
respective license terms continue to apply.

## Bundled download tools

The Windows amd64 Portable build embeds the following command-line tools inside
the application executable. At runtime they are extracted to the application's
managed tools directory and executed as separate processes.

### yt-dlp

- Component: official Windows standalone `yt-dlp.exe`
- Project page: https://pypi.org/project/yt-dlp/
- Artifact source used by the build script: official yt-dlp Windows release
- Project source license: Unlicense
- Distributed Windows executable: **GPL-3.0-or-later**
- Reason: upstream states that PyInstaller-bundled executables contain GPLv3+
  licensed code, so the combined executable is distributed under GPLv3+
- Modification status: this project does not modify yt-dlp
- Verification: the downloaded executable is checked against the SHA-256 value
  published with the same upstream release
- Exact artifact URL and SHA-256 are recorded in the generated
  `release-metadata.json` bundle manifest

The yt-dlp executable remains subject to its own license terms. The MIT License
for this repository does not replace or weaken those terms. For the license and
corresponding-source details applicable to a particular yt-dlp release, use the
upstream release's Licensing section and `THIRD_PARTY_LICENSES.txt`.

### FFmpeg / ffprobe

- Project: FFmpeg
- Project page: https://ffmpeg.org/
- Windows build provider used by the build script: BtbN FFmpeg-Builds
- Artifact: `ffmpeg-master-latest-win64-lgpl.zip`
- Build variant: Windows x86_64 LGPL static build
- Applicable FFmpeg license for this selected build family:
  **LGPL-2.1-or-later**
- Modification status: this project does not modify FFmpeg or ffprobe
- Verification: the downloaded archive is checked against the SHA-256 value
  published with the same build release
- Exact archive URL and executable SHA-256 values are recorded in the generated
  `release-metadata.json` bundle manifest

FFmpeg can become GPL or non-redistributable depending on configure options and
linked libraries. This project intentionally downloads the provider's
`win64-lgpl` artifact. If the FFmpeg artifact or provider is changed, its
license conditions must be reviewed again before release.

FFmpeg license information:
https://ffmpeg.org/legal.html

## Microsoft Edge WebView2

The Windows Portable build uses Wails with `-webview2 embed`. This embeds the
Microsoft Edge WebView2 Evergreen Runtime bootstrapper, not the complete
WebView2 Runtime.

- Component: Microsoft Edge WebView2 Runtime bootstrapper
- Distribution documentation:
  https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution
- License/redistribution terms: Microsoft terms applicable to WebView2
- The bootstrapper and installed WebView2 Runtime are not covered by this
  repository's MIT License

When WebView2 Runtime is missing, the bootstrapper may download and install the
appropriate Evergreen Runtime from Microsoft.

## Principal application dependencies

The application also incorporates open-source runtime dependencies. The exact
versions are controlled by `go.mod` / `go.sum` and
`frontend/yarn.lock`.

| Component | Role | License |
| --- | --- | --- |
| Wails v2 | Desktop application framework | MIT |
| React / React DOM | Frontend runtime | MIT |
| Ant Design | UI component library | MIT |
| @ant-design/v5-patch-for-react-19 | React 19 compatibility patch | MIT |
| modernc.org/sqlite | Go SQLite driver | BSD-3-Clause |

`modernc.org/sqlite` also contains SQLite code and additional third-party
components governed by their respective upstream notices.

Transitive dependencies remain governed by their own licenses. Redistributors
who modify the dependency graph or produce a materially different package
should review the exact versions in the lockfiles and preserve any license,
copyright, attribution, and notice files required by those dependencies.

## Services and downloaded media

CHZZK, YouTube, Microsoft, and other product or service names are the property
of their respective owners. This project is not an official client of those
services unless separately stated.

Users are responsible for ensuring that their use and downloading of media
comply with applicable law, service terms, and the rights associated with the
content being downloaded.
