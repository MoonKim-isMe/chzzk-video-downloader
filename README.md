# CHZZK Video Downloader

치지직 채널을 검색하거나 채널 URL을 직접 입력하고, 채널의 VOD를 yt-dlp로 내려받기 위한 Windows 데스크톱 애플리케이션입니다.

현재는 **Phase 1 — 채널 탐색 및 저장**까지 구현 중입니다.

## 현재 구현 범위

- 비공식 치지직 JSON API 기반 채널 검색
- 검색 결과 offset 기반 추가 조회
- 치지직 채널 URL 파싱 및 channelId 추출
- channelId 기반 실제 채널 정보 조회
- 검색/URL 입력 결과를 공통 Channel 모델로 처리
- 채널 저장/삭제 및 channelId 중복 방지
- 저장 채널 선택 UI
- 채널 검색 / URL 직접 입력 / 다운로드 탭 Shell

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

향후 yt-dlp, ffmpeg, ffprobe 및 SQLite를 연동합니다.

## 개발 환경

Windows 10/11을 기본 개발 및 배포 환경으로 합니다.

필수 도구:

- Go 1.23 이상
- Node.js 24 이상
- Corepack / Yarn
- Wails CLI v2.15.0
- Microsoft WebView2 Runtime

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

## 개발 실행

```powershell
wails dev
```

Wails가 Vite 개발 서버와 Go 애플리케이션을 함께 실행합니다.

## 검증

치지직 API 계층 단위 테스트:

```powershell
go test ./internal/chzzk
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
gofmt -w app.go main.go internal/chzzk/*.go
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
