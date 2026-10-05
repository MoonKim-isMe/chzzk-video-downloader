# CHZZK Video Downloader

CHZZK의 채널과 VOD를 조회하고 영상을 다운로드할 수 있는 Windows 데스크톱 애플리케이션입니다.

검색 결과 또는 VOD URL에서 다운로드 작업을 추가할 수 있으며, 다운로드 진행 상태와 완료/실패 이력을 앱에서 관리할 수 있습니다. Windows Portable EXE 배포를 기준으로 하며 영상 다운로드에 필요한 도구는 애플리케이션에 포함됩니다.

## 주요 기능

- CHZZK 채널 검색
- 자주 사용하는 채널 북마크
- 선택한 채널의 VOD 목록 조회 및 추가 조회
- CHZZK VOD URL 직접 입력
- VOD 썸네일, 제목, 게시일, 재생시간, 조회수, 카테고리 및 태그 표시
- 여러 다운로드 작업 Queue 등록
- 동시 다운로드 수 설정
- 다운로드 진행률, 다운로드 크기, 속도, 남은 시간 표시
- 진행 중인 다운로드 취소
- 완료 항목 폴더 열기 및 목록 삭제
- 실패 항목 오류 확인 및 목록 삭제
- 다운로드 해상도 및 출력 포맷 설정
- 다운로드 가속 설정
- 다운로드 속도 제한 설정
- Light / Dark 테마
- 설정, 북마크 및 다운로드 이력 SQLite 저장
- 앱 재실행 후 사용자 설정 및 이력 복원

## 다운로드 설정

설정 화면에서 다음 항목을 변경할 수 있습니다.

| 항목 | 지원 값 |
| --- | --- |
| 다운로드 폴더 | 사용자가 선택한 폴더 |
| 해상도 | 최고 화질, 2160p, 1440p, 1080p, 720p 이하 |
| 출력 포맷 | MP4, MKV, WebM |
| 다운로드 가속 | 안정, 기본, 고속, 초고속 |
| 다운로드 속도 제한 | 0 이상 MB/s, 0은 제한 없음 |
| 동시 다운로드 수 | 1~8 |
| 테마 | Light, Dark |

다운로드 경로, 해상도, 출력 포맷, 다운로드 가속, 다운로드 속도 제한은 작업을 Queue에 추가하는 시점의 설정을 사용합니다. 이미 대기 중이거나 다운로드 중인 작업은 이후 설정 변경의 영향을 받지 않습니다.

동시 다운로드 수는 전체 Download Queue에 즉시 적용됩니다. 값을 낮춰도 이미 실행 중인 다운로드는 중단하지 않고, 이후 작업부터 변경된 제한을 적용합니다.

## 사용 방법

Windows Portable EXE를 실행한 뒤 다음 방식으로 영상을 다운로드할 수 있습니다.

1. **채널 검색**에서 CHZZK 채널을 검색합니다.
2. 채널을 선택해 VOD 목록을 확인합니다.
3. 원하는 VOD를 다운로드 목록에 추가합니다.
4. 또는 **URL 직접 입력**에서 아래 형식의 CHZZK VOD URL을 입력합니다.

```text
https://chzzk.naver.com/video/{videoNo}
```

5. **다운로드** 화면에서 진행 상태를 확인합니다.
6. 완료된 작업은 폴더를 열거나 앱의 완료 목록에서 삭제할 수 있습니다.

목록 삭제는 다운로드된 영상 파일을 삭제하지 않고 앱의 다운로드 이력만 제거합니다.

## 지원 환경

- Windows 10 / 11
- Windows amd64
- Microsoft WebView2 Runtime

Portable EXE에는 yt-dlp, ffmpeg, ffprobe가 포함됩니다. 앱 실행 시 필요한 도구는 사용자 데이터 영역으로 자동 준비되므로 일반 사용자가 별도로 설치할 필요는 없습니다.

WebView2 Runtime이 설치되어 있지 않거나 버전이 너무 오래된 경우 Windows에서 Runtime 설치가 필요할 수 있습니다.

## 사용자 데이터 위치

사용자 데이터와 실행 도구는 EXE가 있는 폴더가 아니라 Windows 사용자 영역에 저장됩니다.

```text
%APPDATA%\CHZZK Video Downloader\data.sqlite3
%LOCALAPPDATA%\CHZZK Video Downloader\tools
%LOCALAPPDATA%\CHZZK Video Downloader\webview2
```

- `data.sqlite3`: 설정, 북마크, 다운로드 이력
- `tools`: 앱에 포함된 다운로드/미디어 처리 도구
- `webview2`: WebView2 사용자 데이터

Portable EXE를 다른 폴더로 이동하거나 새 버전으로 교체해도 위 사용자 데이터는 그대로 유지됩니다.

## 기술 스택

- Wails v2.15
- Go 1.23+
- React 19
- TypeScript
- Vite 5
- Tailwind CSS v4
- Ant Design v5
- Yarn 4
- SQLite
- yt-dlp
- ffmpeg / ffprobe

## 개발 환경 준비

Windows 10/11을 기본 개발 환경으로 합니다.

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

Windows에서는 프로젝트 루트에서 아래 스크립트를 실행하는 방식을 권장합니다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1
```

최초 실행 시 Windows용 다운로드 도구를 내려받고 SHA-256 checksum을 검증합니다. 이후에는 준비된 파일을 재사용합니다.

`frontend/dist`가 없거나 비어 있으면 개발 스크립트가 먼저 프론트엔드를 빌드한 뒤 `wails dev`를 실행합니다.

다운로드 도구를 최신 release로 다시 준비하려면:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1 -RefreshTools
```

직접 `wails dev`를 실행하려면 다운로드 도구와 `frontend/dist`가 먼저 준비되어 있어야 합니다. 일반 개발에서는 `scripts/dev.ps1`을 개발 실행 진입점으로 사용하는 것을 권장합니다.

## 검증

Go 테스트:

```powershell
go test ./...
```

프론트엔드 타입 검사:

```powershell
cd frontend
yarn typecheck
```

프론트엔드 프로덕션 빌드:

```powershell
yarn build
cd ..
```

전체 Wails 빌드:

```powershell
wails build
```

## Windows Portable 빌드

프로젝트 루트에서 다음 명령으로 Windows Portable 배포 파일을 생성합니다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
```

버전은 `wails.json`의 `info.productVersion`을 기준으로 하며 기본 실행은 패치 버전을 1 증가시킵니다.

| 옵션 | 동작 |
| --- | --- |
| 기본 실행 | 패치 버전 증가 |
| `-Minor` | 마이너 버전 증가, 패치 0 초기화 |
| `-Major` | 메이저 버전 증가, 마이너/패치 0 초기화 |
| `-NoVersionBump` | 현재 버전 유지 |
| `-RefreshTools` | 최신 다운로드 도구를 다시 준비 |

사용 예시:

```powershell
# 기본 패치 증가 빌드
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1

# 마이너 버전 증가
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1 -Minor

# 메이저 버전 증가
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1 -Major

# 같은 버전으로 다시 빌드
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1 -NoVersionBump

# 최신 다운로드 도구로 빌드
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1 -RefreshTools
```

빌드 결과는 다음 형식으로 생성됩니다.

```text
build/portable/v{version}/
  CHZZK-Video-Downloader-v{version}-portable.exe
  CHZZK-Video-Downloader-v{version}-portable.exe.sha256
  release-metadata.json
  LICENSE
  THIRD_PARTY_NOTICES.md
```

배포 파일 검증:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\verify-portable.ps1 -ReleaseDir .\build\portable\v{version}
```

패키징 스크립트 검증:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\test-portable.ps1
```

실제 배포 전에는 Windows 10/11 환경에서 Portable EXE만 별도 폴더로 복사한 뒤 실행, 다운로드, 병합, 취소, 재시작과 사용자 데이터 유지 여부를 확인하는 것을 권장합니다.

## 라이선스

이 Repository에서 작성한 프로젝트 소스 코드는 **MIT License**로 배포합니다.

- 프로젝트 라이선스: [LICENSE](./LICENSE)
- 제3자 구성요소 및 배포 도구 고지: [THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md)
- 저작권 표기: `Copyright (c) 2026 MoonKim`

yt-dlp, FFmpeg/ffprobe, Microsoft Edge WebView2 및 기타 의존성은 각각의 라이선스를 따릅니다. 자세한 출처와 라이선스 정보는 `THIRD_PARTY_NOTICES.md`를 확인해 주세요.
