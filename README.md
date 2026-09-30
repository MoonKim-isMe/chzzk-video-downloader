# CHZZK Video Downloader

치지직 채널과 VOD를 탐색하고 yt-dlp를 통해 영상을 내려받기 위한 Windows 데스크톱 애플리케이션입니다.

현재는 Phase 0 프로젝트 기반 구성 단계입니다.

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

프론트엔드 타입 검사:

```powershell
cd frontend
yarn typecheck
```

프론트엔드 프로덕션 빌드:

```powershell
yarn build
```

Go 포맷 확인:

```powershell
gofmt -w app.go main.go
```

전체 Wails 빌드:

```powershell
cd ..
wails build
```

빌드 결과는 기본적으로 `build/bin`에 생성됩니다.

## Windows WebView2

Wails v2 Windows 애플리케이션은 WebView2 Runtime을 사용합니다. Windows 11에는 일반적으로 포함되어 있으며, 최종 패키징 단계에서 WebView2 부트스트래퍼 포함 정책을 확정합니다.

## 개발 계획

세부 Phase와 진행 상태는 [`DEVELOPMENT_PLAN.md`](./DEVELOPMENT_PLAN.md)를 기준으로 관리합니다.
