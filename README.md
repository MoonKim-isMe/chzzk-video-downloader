# CHZZK Video Downloader

치지직 채널을 검색하거나 채널 URL을 직접 입력하고, 채널의 VOD를 yt-dlp로 내려받기 위한 Windows 데스크톱 애플리케이션입니다.

현재는 **Phase 3-A — yt-dlp / ffmpeg 실행 기반**까지 구현 중입니다.

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
gofmt -w app.go main.go internal/chzzk/*.go internal/downloader/*.go
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
