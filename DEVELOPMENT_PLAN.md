# DEVELOPMENT_PLAN

## 프로젝트 목표

사용자가 치지직 채널 URL을 입력하면 채널 ID를 식별하고, 해당 채널의 VOD 목록을 조회한 뒤 선택한 영상을 yt-dlp로 다운로드하는 Windows 데스크톱 애플리케이션을 구현한다.

핵심 요구사항:

- 치지직 채널 URL 입력 및 검증
- 채널 URL에서 channelId 추출
- 등록한 채널의 동영상 목록 조회
- 선택한 동영상 다운로드
- 입력한 채널 저장
- 다운로드 디렉터리 설정
- 해상도 설정
- 출력 포맷 설정

## 기술 기준

- Desktop: Wails v2
- Backend: Go
- Frontend: React 19 + TypeScript + Vite
- UI: Tailwind CSS v4 + Ant Design v5
- Package Manager: Yarn
- Download Engine: yt-dlp + ffmpeg + ffprobe
- Persistence: SQLite
- Primary Target: Windows 10/11

## Phase 0 — 프로젝트 기반 구성

- [x] FND-1. Wails v2 + Go 기본 애플리케이션 구조 구성
- [x] FND-2. React 19 + TypeScript + Vite 프론트엔드 구성
- [x] FND-3. Tailwind CSS v4 + Ant Design v5 기본 UI 환경 구성
- [x] FND-4. 개발/빌드 명령과 저장소 기본 설정 구성
- [x] FND-5. 최소 앱 Shell과 Go 애플리케이션 바인딩 기반 구성
- [ ] FND-6. 가능한 범위의 포맷/정적 검증 및 빌드 절차 확인

### Phase 0 검증 현황

완료:

- Go 소스 gofmt 적용
- wails.json, package.json, tsconfig.json JSON 파싱 확인
- 외부 의존성이 없는 App 코드 Go 컴파일 확인
- Windows 로컬 개발/빌드 절차 README 기록

현재 실행 환경 제약으로 미완료:

- yarn install
- yarn typecheck
- yarn build
- wails build

외부 패키지 다운로드가 가능한 Windows 개발 환경에서 위 검증을 완료한 뒤 FND-6을 완료 처리한다.

### Phase 0 완료 조건

- Wails v2 프로젝트 구성이 저장소에 존재한다.
- Go 애플리케이션 엔트리포인트와 Wails App 객체가 존재한다.
- React/TypeScript/Vite 프론트엔드가 구성되어 있다.
- Tailwind CSS v4와 Ant Design v5를 사용할 수 있는 구성이 존재한다.
- Yarn을 기준으로 프론트엔드 명령이 정의되어 있다.
- Windows 로컬 개발 및 빌드 절차가 README에 기록되어 있다.
- 다운로드/API/SQLite 비즈니스 기능은 Phase 0에 포함하지 않는다.

## Phase 1 — 채널 URL 등록 및 저장

채널명 검색은 지원하지 않는다. 사용자가 치지직 채널 URL을 직접 입력하며, Phase 1에서는 HTML 크롤링이나 치지직 내부 검색 API를 사용하지 않는다.

지원하는 기본 입력 형식:

```text
https://chzzk.naver.com/{channelId}
```

저장 시 원본 입력값을 그대로 기준으로 삼지 않고 정규화된 채널 URL과 추출한 channelId를 기준으로 중복을 판단한다.

- [ ] CH-1. 치지직 채널 URL 파싱, 정규화 및 channelId 추출 구현
- [ ] CH-2. 잘못된 도메인, 잘못된 경로, channelId 누락 등 URL Validation 구현
- [ ] CH-3. 채널 URL 입력 및 등록 UI 구현
- [ ] CH-4. 저장 채널 추가/삭제 및 channelId 기준 중복 방지 구현
- [ ] CH-5. 저장 채널 목록 및 선택 UI 구현

### Phase 1 API 원칙

- 채널을 찾기 위한 이름 검색 API는 사용하지 않는다.
- URL에서 channelId를 직접 추출하므로 채널 등록 자체에는 외부 API가 필요하지 않다.
- 채널명, 채널 이미지, 팔로워 수 등 부가 정보가 필요할 경우 치지직 공식 Open API 사용을 우선한다.
- 공식 채널 정보 조회는 Client 인증이 필요하므로 Client Secret을 애플리케이션 바이너리에 하드코딩하지 않는다.
- 공식 API 인증 정보 관리 방식이 확정되기 전까지 부가 정보 조회는 Phase 1의 필수 완료 조건으로 두지 않는다.

## Phase 2 — 채널 VOD 목록

- [ ] VOD-1. channelId 기준 채널별 VOD 목록 조회 구현
- [ ] VOD-2. VOD 메타데이터 모델 정의
- [ ] VOD-3. VOD 목록 UI 구현
- [ ] VOD-4. 페이지네이션 또는 연속 조회 처리

## Phase 3 — 단일 VOD 다운로드

- [ ] DL-1. yt-dlp 실행 경로 및 프로세스 래퍼 구현
- [ ] DL-2. 선택한 치지직 VOD 다운로드 구현
- [ ] DL-3. ffmpeg/ffprobe 연동 기반 구성
- [ ] DL-4. 다운로드 오류 처리 구현

## Phase 4 — Download Manager

- [ ] DM-1. 다운로드 Queue 구현
- [ ] DM-2. 다운로드 진행률/속도/ETA 파싱
- [ ] DM-3. 다운로드 취소 처리
- [ ] DM-4. 완료/실패 상태 관리
- [ ] DM-5. 중복 다운로드 방지

## Phase 5 — Settings

- [ ] SET-1. 다운로드 디렉터리 선택
- [ ] SET-2. 해상도 선택
- [ ] SET-3. 출력 포맷 선택
- [ ] SET-4. 동시 다운로드 수 설정
- [ ] SET-5. 설정 UI 구현

## Phase 6 — Persistence 및 Windows 패키징

- [ ] DB-1. SQLite 초기화 및 마이그레이션 구조 구성
- [ ] DB-2. 저장 채널 영속화
- [ ] DB-3. 다운로드 이력 영속화
- [ ] DB-4. 설정 영속화
- [ ] PKG-1. yt-dlp/ffmpeg/ffprobe 배포 전략 적용
- [ ] PKG-2. Windows 빌드 및 WebView2 배포 정책 적용
- [ ] PKG-3. 최종 Windows 패키징 검증
