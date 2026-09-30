# DEVELOPMENT_PLAN

## 프로젝트 목표

치지직 채널을 검색하고, 채널의 VOD 목록을 조회한 뒤 선택한 영상을 yt-dlp로 다운로드하는 Windows 데스크톱 애플리케이션을 구현한다.

핵심 요구사항:

- 치지직 사용자 검색
- 검색한 사용자의 동영상 목록 조회
- 선택한 동영상 다운로드
- 검색한 사용자 저장
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

- [ ] FND-1. Wails v2 + Go 기본 애플리케이션 구조 구성
- [ ] FND-2. React 19 + TypeScript + Vite 프론트엔드 구성
- [ ] FND-3. Tailwind CSS v4 + Ant Design v5 기본 UI 환경 구성
- [ ] FND-4. 개발/빌드 명령과 저장소 기본 설정 구성
- [ ] FND-5. 최소 앱 Shell과 Go 애플리케이션 바인딩 기반 구성
- [ ] FND-6. 가능한 범위의 포맷/정적 검증 및 빌드 절차 확인

### Phase 0 완료 조건

- Wails v2 프로젝트 구성이 저장소에 존재한다.
- Go 애플리케이션 엔트리포인트와 Wails App 객체가 존재한다.
- React/TypeScript/Vite 프론트엔드가 구성되어 있다.
- Tailwind CSS v4와 Ant Design v5를 사용할 수 있는 구성이 존재한다.
- Yarn을 기준으로 프론트엔드 명령이 정의되어 있다.
- Windows 로컬 개발 및 빌드 절차가 README에 기록되어 있다.
- 다운로드/API/SQLite 비즈니스 기능은 Phase 0에 포함하지 않는다.

## Phase 1 — 채널 검색 및 저장

- [ ] CH-1. 치지직 채널 검색 API 계층 구현
- [ ] CH-2. 채널 검색 UI 구현
- [ ] CH-3. 저장 채널 추가/삭제 구현
- [ ] CH-4. 저장 채널 목록 UI 구현

## Phase 2 — 채널 VOD 목록

- [ ] VOD-1. 채널별 VOD 목록 조회 구현
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
