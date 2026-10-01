# CHZZK Video Downloader

치지직 채널을 검색하거나 채널 URL을 직접 입력하고, 채널의 VOD를 yt-dlp로 내려받기 위한 Windows 데스크톱 애플리케이션입니다.

현재는 **Phase 6 — Persistence 완료 후 UX Refinement**를 진행 중입니다.

## 현재 구현 범위

- 비공식 치지직 JSON API 기반 채널 검색
- 검색 결과 offset 기반 추가 조회
- 치지직 채널 URL 파싱 및 channelId 추출
- channelId 기반 실제 채널 정보 조회
- 검색/URL 입력 결과를 공통 Channel 모델로 처리
- 채널 저장/삭제 및 channelId 중복 방지
- 저장 채널 선택 UI
- 채널 검색 / URL 직접 입력 / 다운로드 탭 Shell
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

## 다운로드 도구 탐색

Phase 3-A부터 앱은 `yt-dlp`, `ffmpeg`, `ffprobe` 실행 파일을 다음 순서로 찾습니다.

1. `CHZZK_DOWNLOADER_TOOLS_DIR` 환경 변수로 지정한 디렉터리
2. 앱 실행 파일 옆 `tools` 디렉터리
3. 앱 실행 파일과 같은 디렉터리
4. 시스템 `PATH`

개발 환경에서는 PATH에 등록하거나 다음처럼 도구 디렉터리를 지정할 수 있습니다.

```powershell
$env:CHZZK_DOWNLOADER_TOOLS_DIR = "C:\\Tools\\chzzk-video-downloader"
wails dev
```

최종 배포 시 도구를 어떤 위치에 포함하고 업데이트할지는 Windows 패키징 Phase에서 확정합니다.

## 개발 실행

```powershell
wails dev
```

Wails가 Vite 개발 서버와 Go 애플리케이션을 함께 실행합니다.

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

Wails v2 Windows 애플리케이션은 WebView2 Runtime을 사용합니다. Windows 11에는 일반적으로 포함되어 있으며, 최종 패키징 단계에서 WebView2 부트스트래퍼 포함 정책을 확정합니다.

## 개발 계획

세부 Phase와 진행 상태는 [`DEVELOPMENT_PLAN.md`](./DEVELOPMENT_PLAN.md)를 기준으로 관리합니다.

## Phase 3~4 다운로드 동작

Phase 4-A 기준으로 Phase 3의 단일 다운로드 엔진을 FIFO Download Queue가 감싸며 여러 VOD 작업을 순서대로 관리할 수 있습니다.

- 현재 한 번에 하나의 VOD만 실행하며 이후 작업은 queued 상태로 대기합니다.
- 다운로드 시작 전에 yt-dlp, ffmpeg, ffprobe 상태를 확인합니다.
- 기본 저장 위치는 사용자 홈의 `Downloads/CHZZK Video Downloader`입니다.
- 진행률, 다운로드 크기, 전체 크기, 속도, ETA를 `download:state` 이벤트로 React에 전달합니다.
- 다운로드 중에는 취소할 수 있습니다.
- 완료 시 ffmpeg 후처리까지 끝난 최종 파일 경로를 표시합니다.
- Phase 4-A부터 여러 VOD를 FIFO Queue에 등록할 수 있으며 실제 동시 실행은 1개로 유지합니다.
- queued/running 작업을 개별 취소할 수 있으며 다운로드 탭에서 전체 Task 상태를 실시간으로 확인합니다.

## Phase 4 Scheduler 기준

Phase 4-D부터 Queue는 `maxConcurrent`를 런타임에 변경할 수 있습니다. 현재 앱 기본값은 1이며 UI 설정은 아직 제공하지 않습니다.

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
- 동시 다운로드 수: `1`

지원 해상도는 `best / 2160p / 1440p / 1080p / 720p`, 출력 포맷은 `mp4 / mkv / webm`이며 동시 다운로드 수는 `1~8` 범위입니다.

## Phase 5-B 다운로드 설정 적용

Queue에 VOD를 추가하는 순간 다운로드 경로, 해상도, 출력 포맷을 Snapshot으로 고정합니다. 이후 Settings를 변경해도 이미 queued/running인 작업은 기존 옵션을 유지하며 새로 등록한 작업부터 새 값을 사용합니다.

해상도 선택은 yt-dlp `--format`의 최대 height 조건으로 적용합니다.

- best: `bv*+ba/b`
- 2160p: `bv*[height<=2160]+ba/b[height<=2160]`
- 1440p: `bv*[height<=1440]+ba/b[height<=1440]`
- 1080p: `bv*[height<=1080]+ba/b[height<=1080]`
- 720p: `bv*[height<=720]+ba/b[height<=720]`

출력 포맷은 `mp4 / mkv / webm`을 지원하며 yt-dlp의 `--merge-output-format`과 `--remux-video`를 함께 사용합니다. 동시 다운로드 수는 Snapshot이 아니라 Queue Scheduler 전역 설정으로 즉시 반영됩니다.

## Phase 5-C Settings UI

앱 헤더 우측의 `설정` 버튼에서 우측 Drawer를 열 수 있습니다.

Drawer에서는 다음 값을 편집합니다.

- 다운로드 폴더: Wails native directory dialog로 선택
- 해상도: 최고 화질 / 2160p / 1440p / 1080p / 720p 이하
- 출력 포맷: MP4 / MKV / WebM
- 동시 다운로드 수: 1~8

Drawer가 열릴 때마다 백엔드의 현재 설정을 다시 조회합니다. 저장 시 프론트 Form Validation 후 `UpdateSettings`를 호출하며, 설정 영속화 전인 Phase 5에서는 앱 재시작 시 기본값으로 초기화됩니다.

## Phase 5-D Settings 안정화

Phase 5-D에서 Settings와 Queue의 적용 시점을 최종 확정했습니다.

- 다운로드 경로/해상도/출력 포맷은 Queue 등록 순간 Snapshot으로 고정합니다.
- 실행 중이거나 대기 중인 작업은 이후 Settings 변경의 영향을 받지 않습니다.
- 변경 후 새로 등록한 작업부터 새 다운로드 옵션을 사용합니다.
- 동시 다운로드 수는 Scheduler 전역 설정으로 즉시 반영됩니다.
- 동시 다운로드 수를 낮춰도 현재 실행 중인 작업은 종료하지 않습니다.
- 지원하는 5개 해상도와 3개 컨테이너의 15개 조합을 동일한 Command Builder 경로로 처리합니다.

Phase 6 설정 영속화는 `internal/settings.StorageRecord` v1을 기준으로 구현합니다. 저장 필드는 `schemaVersion`, `downloadDir`, `resolution`, `outputFormat`, `maxConcurrentDownloads`이며 복원 시 현재 Settings Validation을 다시 수행합니다.

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

앱 시작 시 저장 채널과 Settings를 복원하며, 이전 실행에서 `queued` 또는 `running` 상태로 남은 다운로드는 실제 프로세스가 사라졌으므로 `cancelled` 상태로 복구합니다. 완료/실패/취소 이력은 다운로드 탭에 계속 표시됩니다.

Schema는 `schema_migrations`와 migration version으로 관리합니다. Settings는 `StorageRecord v1`을 단일 row로 저장하고 복원 시 현재 Validation을 다시 수행합니다.

SQLite driver는 Windows에서 CGO 없이 사용할 수 있는 `modernc.org/sqlite`를 사용합니다. 현재 실행 환경은 외부 Go module 다운로드가 차단되어 실제 driver 기반 전체 테스트와 `go mod tidy`는 Windows/네트워크 가능 환경에서 추가 확인이 필요합니다.

## Phase 6 UX Refinement

Persistence 다음 작업은 제품 UX 정리입니다. 기존 다운로드/API/SQLite 동작은 유지하면서 다음 순서로 화면을 다듬습니다.

1. App Shell / Navigation
2. Channel / VOD 탐색
3. Download Manager
4. Settings / Feedback / Accessibility

Windows 패키징과 yt-dlp/ffmpeg/ffprobe 배포 전략은 Phase 7로 이동했습니다.
