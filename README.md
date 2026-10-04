# CHZZK/YouTube Video Downloader

CHZZK와 YouTube의 영상을 한 앱에서 탐색하고 다운로드하기 위한 Windows 데스크톱 애플리케이션입니다.

현재 `v0.1`은 CHZZK 검색·URL 다운로드와 Windows Portable 배포 기반을 제공하며, `v0.2`에서 YouTube 검색·통합 URL 다운로드·YouTube MP3 다운로드를 추가하는 것을 목표로 합니다.

## 현재 구현 범위

- 비공식 치지직 JSON API 기반 채널 검색
- 검색 결과 offset 기반 추가 조회
- 치지직 VOD URL 파싱 및 videoNo 추출
- videoNo 기반 단일 VOD 정보 조회
- 채널 검색 결과 선택과 저장 액션 분리
- 채널 저장/삭제 및 channelId 중복 방지
- 저장 채널 선택 UI
- 채널 검색 / VOD URL 직접 입력 / 다운로드 탭 Shell
- 비공식 치지직 API 기반 채널 VOD 목록 조회
- VOD 메타데이터 및 페이지 정보 정규화
- videoNo 기반 표준 VOD URL 생성
- 선택한 저장 채널의 VOD 카드 목록 표시
- VOD 썸네일, 제목, 게시일, 재생시간, 조회수, 카테고리/태그 표시
- 다음 페이지가 있을 때 `더 보기`로 VOD 목록 추가 조회
- 초기/추가 VOD 조회 실패 시 동일 요청 재시도
- page 병합 시 `videoNo` 기준 중복 제거
- 채널 전환 시 이전 VOD/선택 상태 분리
- Phase 3에서 사용할 다운로드 대상 VOD 선택 상태 관리
- yt-dlp / ffmpeg / ffprobe 실행 파일 탐색 및 버전 확인
- yt-dlp 단일 VOD 기본 명령 생성
- stdout/stderr 라인 스트리밍 및 exit code 오류 처리
- 메모리 기반 DownloadTask Registry 및 FIFO Queue
- queued/running videoNo 중복 등록 방지
- 작업 완료/실패/취소 후 다음 Queue 자동 실행
- GetDownloadTasks 기반 전체 작업 조회
- queued/running 작업 취소 및 pending Queue 제거
- 실패/취소 후 다음 Queue 자동 지속 실행
- 취소/완료 경합 시 cancelled 상태 우선 보장

Phase 1의 저장 채널은 현재 앱 실행 중 메모리에 보관합니다. 앱 재실행 후에도 유지되는 영속 저장은 이후 SQLite Phase에서 추가합니다.

## v0.2 목표

v0.2의 제품명은 **CHZZK/YouTube Video Downloader**입니다. 기존 CHZZK 동작을 유지하면서 YouTube를 동일한 다운로드 경험에 추가합니다.

- CHZZK 검색은 현재 채널 검색과 VOD 목록 흐름을 유지합니다.
- YouTube 검색은 CHZZK 검색과 분리된 전용 화면으로 제공합니다.
- `URL 직접 입력`은 CHZZK와 YouTube URL을 한 입력창에서 받고 Provider를 자동 판별합니다.
- URL 또는 검색 결과는 공통 Media 모델로 정규화한 뒤 동일한 Download Queue로 전달합니다.
- YouTube는 기존 영상 다운로드에 더해 **MP3 다운로드**를 제공합니다.
- 영상 다운로드의 해상도·컨테이너·동시 다운로드·가속 설정은 가능한 범위에서 기존 정책을 재사용합니다.
- MP3 작업은 영상 작업과 구분해 Queue·진행 상태·완료/실패 이력에 표시합니다.
- 기존 CHZZK 사용자 설정, 북마크, 다운로드 이력과 런타임 데이터가 앱 이름 변경 때문에 유실되지 않도록 데이터 경로 호환 또는 마이그레이션 정책을 먼저 확정합니다.
- Portable 파일명에는 `/`를 사용할 수 없으므로 v0.2 배포 파일은 `CHZZK-YouTube-Video-Downloader-v{version}-portable.exe` 형식을 사용합니다.

기본 탐색 구조는 다음을 목표로 합니다.

```text
[CHZZK 검색] [YouTube 검색] [URL 직접 입력] [다운로드] [설정]
```

v0.2의 상세 구현 순서와 완료 조건은 [`docs/DEVELOPMENT_PLAN-0.2.md`](./docs/DEVELOPMENT_PLAN-0.2.md)를 기준으로 관리합니다.


## 기술 스택

- Wails v2.15
- Go 1.23+
- React 19
- TypeScript
- Vite 5
- Tailwind CSS v4
- Ant Design v5
- Yarn 4

yt-dlp, ffmpeg, ffprobe 실행 기반을 구성했으며 이후 SQLite를 연동합니다.

## 개발 환경

Windows 10/11을 기본 개발 및 배포 환경으로 합니다.

필수 도구:

- Go 1.23 이상
- Node.js 24 이상
- Corepack / Yarn
- Wails CLI v2.15.0
- Microsoft WebView2 Runtime
- yt-dlp
- ffmpeg / ffprobe

Wails CLI 설치:

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
```

Yarn 활성화:

```powershell
corepack enable
```

프론트엔드 의존성 설치:

```powershell
cd frontend
yarn install
cd ..
```

## 다운로드 도구 준비 및 탐색

Windows amd64에서는 사용자가 yt-dlp / ffmpeg / ffprobe를 별도로 설치할 필요가 없도록 다운로드 도구를 앱 executable에 bundle합니다. Wails build hook의 작업 디렉터리에 의존하지 않도록 도구 준비는 프로젝트 wrapper script에서 수행합니다.

`scripts/dev.ps1` 또는 `scripts/build-windows.ps1`이 실행 전에 `scripts/prepare-windows-tools.ps1`을 절대경로로 호출합니다. 스크립트는 다음 파일을 준비합니다.

- yt-dlp 공식 Windows standalone `yt-dlp.exe`
- FFmpeg Windows x64 LGPL static build의 `ffmpeg.exe`
- 동일 build의 `ffprobe.exe`

다운로드한 release asset은 공급자가 제공하는 SHA-256 checksum으로 검증한 뒤 Go executable에 embed됩니다. 바이너리 자체는 Git에 commit하지 않습니다.

앱 실행 시 embedded 파일은 다음 관리 디렉터리에 자동 추출됩니다.

```text
%LOCALAPPDATA%\CHZZK Video Downloader\tools
```

EXE가 있는 폴더에는 런타임 파일을 쓰지 않습니다. 읽기 전용 폴더에 EXE를 두어도 사용자 데이터 폴더에 쓰기 권한이 있으면 실행할 수 있습니다.

도구 탐색 우선순위:

1. `CHZZK_DOWNLOADER_TOOLS_DIR` 환경 변수
2. 앱이 관리하는 LocalAppData bundle 디렉터리
3. 앱 실행 파일 옆 `tools` 디렉터리
4. 앱 실행 파일과 같은 디렉터리
5. 시스템 `PATH`

환경 변수와 PATH는 개발/고급 사용자용 override 및 fallback으로 유지합니다.

## 개발 실행

Windows에서는 아래 스크립트를 권장합니다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1
```

최초 실행에서는 Windows 다운로드 도구를 내려받고 checksum을 검증하므로 시간이 걸릴 수 있습니다. 이후에는 준비된 bundle을 재사용합니다. 또한 `frontend/dist`가 없거나 비어 있으면 `scripts/dev.ps1`이 먼저 `frontend`에서 `yarn build`를 실행해 `main.go`의 `//go:embed all:frontend/dist`가 컴파일 가능한 상태를 만든 뒤 `wails dev`를 시작합니다.

직접 `wails dev`를 실행하면 이 사전 검사를 거치지 않습니다. 직접 실행하려면 다운로드 도구 준비와 함께 `frontend/dist`에 빌드 결과가 있어야 하므로, 비어 있다면 먼저 `cd frontend; yarn build; cd ..`를 실행해야 합니다. 일반 개발에서는 `scripts/dev.ps1`을 단일 진입점으로 사용합니다.

도구를 최신 release로 강제 갱신하려면:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1 -RefreshTools
```

## 검증

치지직 API 및 다운로드 실행 계층 단위 테스트:

```powershell
go test ./internal/chzzk
go test ./internal/downloader
```

프론트엔드 타입 검사:

```powershell
cd frontend
yarn typecheck
```

프론트엔드 프로덕션 빌드:

```powershell
yarn build
```

Go 포맷:

```powershell
cd ..
gofmt -w app.go main.go internal/chzzk/*.go internal/downloader/*.go internal/settings/*.go internal/persistence/*.go
```

전체 Wails 빌드:

```powershell
wails build
```

빌드 결과는 기본적으로 `build/bin`에 생성됩니다.

## Windows WebView2

Wails v2 Windows 애플리케이션은 Microsoft WebView2 Runtime을 사용합니다. Portable 빌드는 `-webview2 embed`로 공식 부트스트래퍼를 EXE에 포함합니다. 기존 Runtime이 있으면 바로 실행하며, 없거나 너무 오래된 경우 공식 Runtime 설치를 안내합니다. 부트스트래퍼는 Runtime 본체가 아니므로 이 경우 인터넷 연결이 필요하며, 설치를 취소하면 앱을 실행할 수 없습니다.

앱 자체에는 설치 프로그램이 없지만 WebView2가 없는 PC에는 Runtime 설치가 필요합니다. Runtime까지 내장하는 완전 오프라인 배포는 현재 범위에 포함하지 않습니다.

WebView2 데이터는 `%LOCALAPPDATA%\CHZZK Video Downloader\webview2`에 저장합니다. 배포 버전에 따라 EXE 파일명이 바뀌어도 같은 경로를 사용합니다. 기존 기본 경로의 WebView2 캐시는 이 경로로 이관하지 않으며, 앱 설정·북마크·다운로드 기록은 기존 SQLite 경로를 그대로 사용합니다.

## 개발 계획

버전별 개발 계획과 진행 상태는 `docs/` 아래에서 관리합니다.

- v0.1: [`docs/DEVELOPMENT_PLAN-0.1.md`](./docs/DEVELOPMENT_PLAN-0.1.md)
- v0.2: [`docs/DEVELOPMENT_PLAN-0.2.md`](./docs/DEVELOPMENT_PLAN-0.2.md)

## Phase 3~4 다운로드 동작

Phase 4-A 기준으로 Phase 3의 단일 다운로드 엔진을 FIFO Download Queue가 감싸며 여러 VOD 작업을 순서대로 관리할 수 있습니다.

- 현재 한 번에 하나의 VOD만 실행하며 이후 작업은 queued 상태로 대기합니다.
- 다운로드 시작 전에 yt-dlp, ffmpeg, ffprobe 상태를 확인합니다.
- 기본 저장 위치는 사용자 홈의 `Downloads/CHZZK Video Downloader`입니다.
- 진행률, 다운로드 크기, 전체 크기, 속도, ETA를 `download:state` 이벤트로 React에 전달합니다.
- 다운로드 중에는 취소할 수 있습니다.
- 완료 시 ffmpeg 후처리까지 끝난 최종 파일 경로를 표시합니다.
- Phase 4-A부터 여러 VOD를 FIFO Queue에 등록할 수 있으며 실제 동시 실행은 1개로 유지합니다.
- queued/running 작업을 개별 취소할 수 있습니다. 취소가 승인되면 목록에서 즉시 제거되며, 완료 작업은 폴더 열기와 목록 삭제를 지원합니다.

## Phase 4 Scheduler 기준

Phase 4-D부터 Queue는 `maxConcurrent`를 런타임에 변경할 수 있으며, 현재 앱 기본값은 3입니다. Phase 5 Settings UI에서 1~8 범위로 변경할 수 있습니다.

- 값을 늘리면 사용 가능한 슬롯만큼 대기 작업을 즉시 실행합니다.
- 값을 줄여도 이미 실행 중인 작업은 중단하지 않습니다.
- 이후 작업부터 변경된 동시 실행 제한을 적용합니다.
- Phase 5의 동시 다운로드 수 설정은 기존 Queue를 재생성하지 않고 이 Scheduler 설정에 연결합니다.

## Phase 5 Settings 기준

Settings는 SQLite에 저장되며 앱 재시작 후 자동으로 복원됩니다. 다운로드 옵션 Snapshot 및 Scheduler 적용 정책은 Phase 5-B 기준을 유지합니다.

기본 설정:

- 다운로드 경로: `Downloads/CHZZK Video Downloader`
- 해상도: `best`
- 출력 포맷: `mp4`
- 다운로드 가속: `standard` (fragment 2개)
- 다운로드 속도 제한: `0 MB/s` (제한 없음)
- 동시 다운로드 수: `3`

지원 해상도는 `best / 2160p / 1440p / 1080p / 720p`, 출력 포맷은 `mp4 / mkv / webm`이며 동시 다운로드 수는 `1~8` 범위입니다.

## Phase 5-B 다운로드 설정 적용

Queue에 VOD를 추가하는 순간 다운로드 경로, 해상도, 출력 포맷, 다운로드 가속, 다운로드 속도 제한을 Snapshot으로 고정합니다. 이후 Settings를 변경해도 이미 queued/running인 작업은 기존 옵션을 유지하며 새로 등록한 작업부터 새 값을 사용합니다.

해상도 선택은 yt-dlp `--format`의 최대 height 조건으로 적용합니다.

- best: `bv*+ba/b`
- 2160p: `bv*[height<=2160]+ba/b[height<=2160]`
- 1440p: `bv*[height<=1440]+ba/b[height<=1440]`
- 1080p: `bv*[height<=1080]+ba/b[height<=1080]`
- 720p: `bv*[height<=720]+ba/b[height<=720]`

출력 포맷은 `mp4 / mkv / webm`을 지원하며 yt-dlp의 `--merge-output-format`과 `--remux-video`를 함께 사용합니다. 다운로드 속도 제한은 각 작업별 MB/s 값을 1,000,000 bytes/s로 변환해 yt-dlp `--limit-rate`에 적용하며, `0`이면 옵션을 생략합니다. 동시 다운로드 수는 Snapshot이 아니라 Queue Scheduler 전역 설정으로 즉시 반영됩니다.

## Phase 5-C Settings UI

앱 헤더 우측의 `설정` 버튼에서 우측 Drawer를 열 수 있습니다.

Drawer에서는 다음 값을 편집합니다.

- 다운로드 폴더: Wails native directory dialog로 선택
- 해상도: 최고 화질 / 2160p / 1440p / 1080p / 720p 이하
- 출력 포맷: MP4 / MKV / WebM
- 다운로드 가속: 안정 (1) / 기본 (2) / 고속 (4) / 초고속 (8)
- 다운로드 속도 제한: 0 이상, MB/s 단위 (0은 제한 없음)
- 동시 다운로드 수: 1~8

Drawer가 열릴 때마다 백엔드의 현재 설정을 다시 조회합니다. 저장 시 프론트 Form Validation 후 `UpdateSettings`를 호출하며, 현재는 SQLite에 저장되어 앱 재시작 후에도 유지됩니다.

## Phase 5-D Settings 안정화

Phase 5-D에서 Settings와 Queue의 적용 시점을 최종 확정했습니다.

- 다운로드 경로/해상도/출력 포맷/다운로드 가속/다운로드 속도 제한은 Queue 등록 순간 Snapshot으로 고정합니다.
- 실행 중이거나 대기 중인 작업은 이후 Settings 변경의 영향을 받지 않습니다.
- 변경 후 새로 등록한 작업부터 새 다운로드 옵션을 사용합니다.
- 동시 다운로드 수는 Scheduler 전역 설정으로 즉시 반영됩니다.
- 동시 다운로드 수를 낮춰도 현재 실행 중인 작업은 종료하지 않습니다.
- 지원하는 5개 해상도와 3개 컨테이너의 15개 조합을 동일한 Command Builder 경로로 처리합니다.

설정 영속화는 `internal/settings.StorageRecord` v4를 기준으로 합니다. 저장 필드는 `schemaVersion`, `downloadDir`, `resolution`, `outputFormat`, `downloadAcceleration`, `downloadRateLimitMBps`, `maxConcurrentDownloads`, `theme`이며 이전 버전 설정은 누락 필드를 기본값으로 호환 복원합니다.

## Phase 6-P SQLite Persistence

Phase 6의 Persistence만 먼저 구현합니다. Windows 패키징 및 외부 도구 번들링은 아직 변경하지 않습니다.

데이터베이스 기본 위치:

```text
<OS 사용자 설정 디렉터리>/CHZZK Video Downloader/data.sqlite3
```

SQLite에는 다음 데이터를 저장합니다.

- 저장 채널
- 다운로드 작업/진행 상태/완료 이력
- Settings

앱 시작 시 저장 채널과 Settings를 복원하며, 이전 실행에서 `queued` 또는 `running` 상태로 남은 다운로드는 실제 프로세스가 사라졌으므로 `cancelled` 상태로 복구합니다. cancelled 항목은 다운로드 탭에서 숨기고, 완료/실패 이력만 표시합니다.

Schema는 `schema_migrations`와 migration version으로 관리합니다. Settings는 `StorageRecord v2`를 단일 row로 저장하고 Light/Dark 테마까지 복원하며 현재 Validation을 다시 수행합니다.

SQLite driver는 Windows에서 CGO 없이 사용할 수 있는 `modernc.org/sqlite`를 사용합니다. 현재 실행 환경은 외부 Go module 다운로드가 차단되어 실제 driver 기반 전체 테스트와 `go mod tidy`는 Windows/네트워크 가능 환경에서 추가 확인이 필요합니다.

## Phase 6 UX Refinement

Persistence 다음 작업은 제품 UX 정리입니다. 기존 다운로드/API/SQLite 동작은 유지하면서 다음 순서로 화면을 다듬습니다.

1. App Shell / Navigation
2. Channel / VOD 탐색
3. Download Manager
4. Settings / Feedback / Accessibility

Windows 패키징과 yt-dlp/ffmpeg/ffprobe 배포 전략은 Phase 7로 이동했습니다.


## URL 직접 입력 동작

`URL 직접 입력` 탭은 채널 URL이 아니라 치지직 VOD URL 전용입니다.

```text
https://chzzk.naver.com/video/{videoNo}
```

URL 입력 영역은 상단에 고정되고, 조회된 단일 VOD는 **좌측 16:9 썸네일 / 우측 영상 정보**의 상세 레이아웃으로 표시합니다. 우측에는 제목, 채널, 게시일, 재생시간, 조회수, 태그와 다운로드 추가 CTA를 표시합니다. 결과 영역은 독립 스크롤되며 900px 이하에서는 썸네일 위 / 영상 정보 아래의 1열로 전환됩니다.


## 라이선스

이 Repository에서 작성한 프로젝트 소스 코드는 **MIT License**로 배포합니다.

- 프로젝트 라이선스: [LICENSE](./LICENSE)
- 제3자 구성요소 및 배포 도구 고지: [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)
- 저작권 표기: `Copyright (c) 2026 MoonKim`

프로젝트의 MIT License는 yt-dlp, FFmpeg/ffprobe, Microsoft Edge WebView2 및 Go/Frontend 의존성의 개별 라이선스를 변경하지 않습니다. 특히 Windows Portable 빌드에 포함되는 공식 `yt-dlp.exe`는 upstream 안내 기준 GPLv3+ 구성요소를 포함한 PyInstaller 배포본이며, FFmpeg/ffprobe는 현재 빌드 스크립트에서 `win64-lgpl` 배포물을 사용합니다. 정확한 구성과 출처는 `THIRD_PARTY_NOTICES.md` 및 각 릴리스의 `release-metadata.json`을 기준으로 확인합니다.

Portable 릴리스 생성 시 `LICENSE`와 `THIRD_PARTY_NOTICES.md`를 함께 복사하며, 검증 스크립트는 두 파일과 project license metadata가 누락되면 릴리스를 실패 처리합니다.

## Windows Portable EXE 배포

Windows에서는 다운로드 도구의 상태 확인·영상 다운로드/처리·취소용 백그라운드 프로세스를 콘솔 창 없이 실행합니다. 진행률과 오류 로그는 앱에서 계속 확인할 수 있습니다. 다른 PC에서 실행할 때도 이 설정이 적용되려면 수정된 코드로 Portable EXE를 다시 빌드해 교체합니다.

Windows amd64에서 프로젝트 루트의 아래 명령으로 빌드합니다. Go/Wails CLI, Node.js, Yarn이 설치되어 있어야 합니다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
```

버전은 `wails.json`의 `info.productVersion`을 기준으로 합니다. 기본 실행은 패치를 1 증가시키며, 두 JSON 버전과 EXE 파일명·배포 metadata를 함께 맞춥니다. 현재 저장 버전 `0.1.0`에서 기본 빌드가 성공하면 `0.1.1`이 됩니다.

| 옵션 | 버전 변경 예시 (`1.2.3` 기준) |
| --- | --- |
| 기본 실행 | `1.2.4` |
| `-Minor` | `1.3.0` |
| `-Major` | `2.0.0` |
| `-NoVersionBump` | `1.2.3` 유지 |

```powershell
# 마이너 증가
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1 -Minor
# 메이저 증가
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1 -Major
# 같은 버전으로 다시 빌드
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1 -NoVersionBump
```

버전 옵션은 하나만 지정할 수 있으며 각 옵션에 `-RefreshTools`를 함께 사용할 수 있습니다. 마이너 증가 시 패치는 0으로, 메이저 증가 시 마이너·패치는 0으로 초기화합니다. 기본 패치는 마이너로 자동 올림되지 않습니다.

빌드 전에 두 파일의 버전이 다르면 중단합니다. 빌드·메타데이터 검사·최종 배포 검증 중 실패하면 두 JSON 파일을 변경 전 바이트로 복원합니다. 성공하면 새 버전이 두 파일에 유지되므로 이후 commit에 함께 포함합니다. `-NoVersionBump`는 두 버전 파일을 쓰지 않고 같은 버전의 산출물을 다시 생성합니다.

매 빌드 전에 `scripts/windows-info.json`을 `build/windows/info.json`으로 복사해 Windows 메타데이터 템플릿을 갱신합니다. 기존 로컬 `info.json`은 덮어쓰므로 제품 메타데이터는 `wails.json`의 `info`에서 관리합니다. 파일·제품 버전의 숫자 리소스와 문자열 `FileVersion`/`ProductVersion`을 함께 기록합니다. 문자열 `FileVersion`은 Windows PowerShell의 .NET Framework가 다른 메타데이터까지 빈 값으로 읽는 문제를 방지하는 데 필요합니다.

`Portable EXE product metadata does not match wails.json` 오류에는 검사한 EXE 경로, 불일치 필드의 예상값과 실제값이 표시됩니다. 빈 값은 `<empty>`로 표시합니다. 실패하면 버전 파일을 복원하므로 실패한 EXE의 버전과 복원된 `wails.json` 버전이 다를 수 있습니다. 수정 반영 후 `-NoVersionBump`로 버전을 유지해 재빌드할 수 있으며, 로컬 `info.json`을 수동 수정할 필요는 없습니다.

```text
build/portable/v0.1.1/
  CHZZK-Video-Downloader-v0.1.1-portable.exe
  CHZZK-Video-Downloader-v0.1.1-portable.exe.sha256
  release-metadata.json
  LICENSE
  THIRD_PARTY_NOTICES.md
```

사용자는 **Portable EXE 하나를 내려받아 실행**할 수 있습니다. 릴리스에는 EXE와 함께 SHA-256, release metadata, 프로젝트 MIT `LICENSE`, 제3자 고지 `THIRD_PARTY_NOTICES.md`를 게시합니다. 이 문서 파일들은 실행 의존 파일은 아니지만 재배포 시 함께 제공하는 것을 기준으로 합니다. 앱 설치/제거, Setup EXE, NSIS, ZIP, UPX 압축은 사용하지 않습니다. 코드 서명도 현재 적용하지 않습니다.

빌드 스크립트는 준비된 도구 3종의 SHA-256을 다시 확인한 뒤 빌드합니다. 빌드 결과의 제품명·버전·개인 배포자 `MoonKim`·저작권을 확인하고, EXE의 SHA-256 및 도구 bundle manifest를 기록합니다. 도구가 누락되거나 손상되면 빌드를 중단합니다.

최신 다운로드 도구로 패치를 증가시킨 새 배포본을 만들려면:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1 -RefreshTools
```

도구는 Go EXE에 내장되며, 첫 Toolchain 확인 시 `%LOCALAPPDATA%\CHZZK Video Downloader\tools`로 추출합니다. 동일 bundle 재실행 시 기존 파일을 재사용하고, 누락된 파일이 있으면 내장본에서 복원합니다. EXE만 다른 폴더로 옮기거나 교체해도 설정·북마크·다운로드 기록은 `%APPDATA%\CHZZK Video Downloader\data.sqlite3`에 유지됩니다. 앱에 도구 자동 업데이트 UI는 아직 없으며, 새 bundle의 EXE로 교체하면 해당 내장 도구로 갱신됩니다. 직접 갱신한 managed 도구는 동일 bundle 재실행에서는 유지되지만 bundle이 바뀌면 내장본으로 교체됩니다.

배포 파일 단일 amd64 PE EXE 여부 및 무결성 확인:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\verify-portable.ps1 -ReleaseDir .\build\portable\v0.1.1
```

패키징 스크립트 계약 검증(실제 Wails/도구 실행을 fixture로 대체):

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\test-portable.ps1
```

실제 배포 전에 Windows 10/11에서 EXE만 별도 폴더로 복사하여 실행하고, 도구 PATH 없이 다운로드·병합·취소·재시작을 확인해야 합니다. 한글/공백 경로, 기존 설정 유지, WebView2 미설치 시 안내, Windows 파일 속성도 확인합니다. 서명되지 않은 EXE에는 Windows SmartScreen 확인 화면이 표시될 수 있습니다.

현재 bundle 대상은 Windows amd64입니다. Windows arm64는 별도 작업으로 취급합니다. Third-party 도구 출처와 라이선스 안내는 `THIRD_PARTY_NOTICES.md`를 참고합니다.


## Phase 6 UX — Shell / Channel Search / Theme

상단 헤더는 제품 UI 기준으로 다음 3영역만 표시합니다.

```text
[CHZZK Video Downloader] [채널 검색 | URL 직접 입력 | 다운로드] [설정 아이콘]
```

기존 본문 Tabs와 Phase/Wails 개발 표시는 제거했습니다.

채널 검색 화면은 헤더 아래 남은 높이를 사용하는 2열 Workspace로 구성합니다.

- 좌측: `검색 / 북마크` 내부 탭
  - 검색 탭: 검색 입력은 고정하고 검색 결과 목록만 스크롤
  - 북마크 탭: 북마크 목록만 스크롤
- 우측: 선택한 채널의 VOD 전용 패널
  - 채널/VOD 요약 헤더는 고정
  - VOD 카드 목록만 독립 스크롤

앱 본문 자체가 중첩 스크롤을 만들지 않도록 채널 검색 Workspace에서는 좌·우 패널이 각각 스크롤을 담당합니다.

Settings Drawer의 **화면 > 테마**에서 `라이트 / 다크`를 선택할 수 있습니다. 기본값은 Dark이며 선택값은 SQLite에 저장되어 앱 재실행 후에도 유지됩니다. 기존 Settings DB는 migration v2에서 Dark 테마로 자동 승격됩니다.


## Download Manager UX

다운로드 탭은 헤더 아래 남은 높이를 사용하는 단일 Workspace입니다.

- 상단: 기능 중심의 다운로드 준비 상태 + 진행/완료/실패 compact 요약
- 본문: `진행 중 / 완료 / 실패` 섹션
- 진행 중: 진행률, 다운로드 크기, 속도, 남은 시간, 취소
- 완료: 결과 경로, `폴더 열기`, `목록에서 삭제`
- 실패: 오류 내용을 compact하게 표시

취소 요청이 승인되면 Task는 즉시 cancelled 상태로 전환되고 UI 목록에서 제거됩니다. 실제 다운로드 프로세스가 완전히 종료되기 전까지 Scheduler slot은 유지하므로 동시 다운로드 제한은 깨지지 않습니다.

`목록에서 삭제`는 다운로드된 파일을 삭제하지 않고 앱의 이력만 제거합니다. 다운로드 도구 상태 메시지는 사용자에게 외부 프로그램명을 직접 노출하지 않고 영상 다운로드/파일 저장 기능의 사용 가능 여부로 표시합니다.


## 다운로드 오류 처리

사용자에게 실행 도구의 raw stderr를 그대로 노출하지 않고 주요 다운로드 실패 원인을 구분합니다.

- 로그인/연령 제한/접근 권한이 필요한 콘텐츠에서 HTTP 401 또는 Unauthorized가 발생하면:
  - `로그인이 필요한 콘텐츠입니다. 연령 제한 또는 접근 권한이 필요한 영상일 수 있습니다.`
- 취소 후 남은 부분 다운로드 상태와 fragment가 충돌하는 오류가 감지되면:
  - `이전 다운로드의 임시 데이터와 충돌했습니다. 다운로드를 처음부터 다시 시도해 주세요.`

취소 후 같은 VOD를 다시 다운로드할 때 이전 fragment를 이어받지 않도록 다운로드 명령에 `--no-continue`와 `--no-keep-fragments`를 사용합니다. 따라서 취소 후 재시도는 부분 파일을 이어받는 대신 처음부터 새로 다운로드합니다.

실패한 다운로드 항목은 다운로드 탭에서 `목록에서 삭제`로 이력만 제거할 수 있으며, 실제 파일은 삭제하지 않습니다.
