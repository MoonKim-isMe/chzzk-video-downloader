# DEVELOPMENT_PLAN

## 프로젝트 목표

사용자가 치지직 채널을 이름으로 검색하거나 채널 URL을 직접 입력하여 채널을 선택하고, 해당 채널의 VOD 목록에서 원하는 영상을 yt-dlp로 다운로드할 수 있는 Windows 데스크톱 애플리케이션을 구현한다.

핵심 요구사항:

- 치지직 채널명 검색
- 치지직 채널 URL 직접 입력 및 검증
- 검색/URL 입력으로 선택한 채널 저장
- 선택한 채널의 동영상 목록 조회
- 선택한 동영상 다운로드
- 다운로드 진행 상태 시각화
- 다운로드 디렉터리 설정
- 해상도 설정
- 출력 포맷 설정

## 화면 및 탐색 구조

채널을 추가하거나 선택하는 방법은 두 가지로 제공한다.

1. **채널 검색**
   - 채널명 또는 검색어로 치지직 채널을 조회한다.
   - 검색 결과에서 채널을 선택하고 저장할 수 있다.

2. **URL 직접 입력**
   - 치지직 채널 URL을 직접 입력한다.
   - URL을 정규화하고 channelId를 추출한 뒤 채널을 선택하고 저장할 수 있다.

다운로드 작업은 입력 방식과 분리된 별도 **다운로드 탭**에서 관리한다.

- 채널 검색
- URL 직접 입력
- 다운로드

저장 채널 데이터는 채널 검색과 URL 직접 입력에서 공통으로 사용하며 channelId를 기준으로 중복을 방지한다.

## API 사용 원칙

치지직 데이터 조회는 다음 우선순위를 따른다.

1. **치지직 웹 서비스에서 사용하는 비공식 JSON API**
2. **치지직 공식 Open API**
3. **HTML 크롤링은 필요한 API가 모두 없거나 사용할 수 없는 경우에만 최후 수단으로 검토**

현재 확인된 비공식 API 예시는 다음과 같다.

```text
GET https://api.chzzk.naver.com/service/v1/search/channels
GET https://api.chzzk.naver.com/service/v1/channels/{channelId}
GET https://api.chzzk.naver.com/service/v1/channels/{channelId}/videos
```

비공식 API는 치지직 웹 서비스 내부 인터페이스이므로 사전 고지 없이 변경될 수 있다. 호출 코드는 UI나 비즈니스 로직에 직접 결합하지 않고 별도 Chzzk API 계층으로 격리한다.

공식 Open API는 비공식 API에 필요한 기능이 없거나 비공식 API가 정상 동작하지 않을 때 보조 또는 대체 경로로 사용한다. 공식 API의 Client Secret을 애플리케이션 바이너리에 하드코딩하지 않는다.

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

## Phase 1 — 채널 탐색 및 저장

채널을 찾는 방식은 **채널 검색**과 **URL 직접 입력** 두 가지를 지원한다. 두 방식 모두 동일한 Channel 모델과 저장 채널 상태를 사용한다.

### 채널 검색

- [x] CH-1. 비공식 채널 검색 API Client 구현
- [x] CH-2. 검색어, offset, size 기반 검색 및 페이지 처리 구현
- [x] CH-3. 채널명, 프로필 이미지, 설명, 팔로워 수 등 검색 결과 모델 정의
- [ ] CH-4. 채널 검색 탭 UI 구현

### URL 직접 입력

지원하는 기본 입력 형식:

```text
https://chzzk.naver.com/{channelId}
```

- [x] CH-5. 치지직 채널 URL 파싱, 정규화 및 channelId 추출 구현
- [x] CH-6. 잘못된 도메인, 잘못된 경로, channelId 누락 등 URL Validation 구현
- [ ] CH-7. URL 직접 입력 탭 UI 구현
- [x] CH-8. 추출한 channelId로 비공식 채널 정보 API를 조회하여 실제 채널 여부 검증

### 저장 채널

- [x] CH-9. 검색/URL 입력 결과를 공통 Channel 모델로 정규화
- [x] CH-10. 저장 채널 추가/삭제 및 channelId 기준 중복 방지 구현
- [ ] CH-11. 저장 채널 목록 및 선택 UI 구현

### Phase 1 검증 현황

완료:

- `go test ./internal/chzzk`
- 채널 URL 정상/비정상 입력 단위 테스트
- 검색 API 응답 → 내부 Channel 모델 변환 단위 테스트
- channelId 기반 채널 조회 단위 테스트
- channelId 중복 저장 방지 단위 테스트

구현되었으나 현재 실행 환경 제약으로 검증 대기:

- CH-4 채널 검색 탭 UI
- CH-7 URL 직접 입력 탭 UI
- CH-11 저장 채널 목록 및 선택 UI
- 다운로드 탭 기본 탐색 Shell

현재 환경은 npm registry에 연결할 수 없어 `yarn install`, `yarn typecheck`, `yarn build`, `wails build`를 실행하지 못했다. 프론트엔드 검증 완료 후 위 UI 항목을 완료 처리한다.

저장 채널 Store는 Phase 1에서는 앱 실행 중 메모리 기반으로 동작하며, 앱 재시작 후 영속화는 Phase 6의 SQLite 작업에서 교체한다.

### Phase 1 API 기준

- 채널 검색은 비공식 `/service/v1/search/channels` API를 우선 사용한다.
- channelId 기반 채널 정보 조회는 비공식 `/service/v1/channels/{channelId}` API를 우선 사용한다.
- 비공식 API가 변경되더라도 프론트엔드 수정 범위를 최소화할 수 있도록 응답을 내부 Channel 모델로 변환한다.
- 공식 Open API는 fallback 또는 비공식 API에서 제공하지 않는 데이터 보완 용도로 사용한다.
- Phase 1에서는 HTML DOM 크롤링을 사용하지 않는다.

## Phase 2 — 채널 VOD 목록

### Phase 2-A — VOD API 및 모델

- [x] VOD-1. 비공식 `/service/v1/channels/{channelId}/videos` 기반 채널별 VOD 목록 조회 구현
- [x] VOD-2. VOD 메타데이터 모델 정의
- [x] VOD-2A-1. page/size/totalCount/totalPages 기반 페이지 메타데이터 처리
- [x] VOD-2A-2. VOD URL 생성 및 Phase 3 전달용 videoNo 정규화
- [x] VOD-2A-3. 잘못된 channelId, API 오류 코드, 비정상 JSON, 예상하지 못한 VOD 응답 형태 처리

#### Phase 2-A 검증 현황

완료:

- `gofmt` 적용
- Phase 2-A VOD 계층을 재현한 격리 Go 테스트 하네스에서 `go test ./internal/chzzk` 성공
- VOD 목록 요청 파라미터 및 응답 모델 변환 단위 테스트
- 페이지/size 보정 및 다음 페이지 계산 단위 테스트
- API 오류 코드 및 비정상 JSON 응답 단위 테스트
- VOD 핵심 필드 누락 응답 감지 단위 테스트

현재 실행 환경 제약으로 실제 Repository 전체 체크아웃 기반의 `go test ./...` 및 `wails build`는 수행하지 못했다.

확인 사항:

- VOD 목록은 `sortType=LATEST`, `pagingType=PAGE`, `page`, `size`를 사용한다.
- 기본 페이지 크기는 24, 최대 페이지 크기는 50으로 제한한다.
- `Video` 모델에 videoNo, 제목, 타입, 게시일, 썸네일, 재생시간, 조회수, 카테고리, 성인 여부, 태그, 채널 정보와 표준 치지직 VOD URL을 포함한다.
- 실제 목록 화면과 연속 조회 UX는 Phase 2-B에서 구현한다.

### Phase 2-B — VOD 목록 UI

- [ ] VOD-3. 선택 채널 VOD 목록 UI 구현 — 구현 완료, 전체 프론트엔드 검증 대기
- [ ] VOD-4. 페이지네이션 또는 연속 조회 UI 구현 — 구현 완료, 전체 프론트엔드 검증 대기

#### Phase 2-B 구현 및 검증 현황

구현 완료:

- 저장 채널 선택 시 첫 VOD 페이지 자동 조회
- VOD 썸네일, 제목, 영상 타입, 게시일, 재생시간, 조회수, 카테고리, 태그 표시
- VOD가 없을 때 Empty 상태 표시
- 초기 목록 조회 중 Skeleton 표시
- API 오류 메시지 표시
- `hasNext` / `nextPage` 기반 `더 보기` 조회 및 기존 목록 뒤에 추가
- 채널 변경 시 ChannelVideoList를 채널 ID 기준으로 재마운트해 이전 요청/화면 상태 분리
- Wails `GetChannelVideos` 호출용 TypeScript 타입 및 backend wrapper 추가

검증 완료:

- 시스템 TypeScript 5.8.3을 사용한 격리 프론트엔드 검사
- React/AntD 외부 타입을 최소 스텁으로 대체하여 JSX 구문, 내부 Video 타입, BackendApp 시그니처, 상태 업데이트 타입 일관성 확인

현재 실행 환경 제약으로 검증 대기:

- 실제 프로젝트 의존성을 사용한 `yarn typecheck`
- `yarn build`
- `wails build`
- 실제 치지직 VOD API와 Wails UI 통합 동작

위 전체 프론트엔드 검증이 완료되면 VOD-3, VOD-4를 완료 처리한다.

### Phase 2-C — 상태 연결 및 안정화

- [ ] VOD-5. 비공식 API 응답 변경 및 UI 오류 상태/재시도 처리
- [ ] VOD-2C-1. 채널 전환 시 VOD 상태 초기화
- [ ] VOD-2C-2. 페이지 병합 시 videoNo 기준 중복 제거
- [ ] VOD-2C-3. Phase 3 다운로드에 전달할 선택 VOD 상태 확정

## Phase 3 — 단일 VOD 다운로드

- [ ] DL-1. yt-dlp 실행 경로 및 프로세스 래퍼 구현
- [ ] DL-2. 선택한 치지직 VOD 다운로드 구현
- [ ] DL-3. ffmpeg/ffprobe 연동 기반 구성
- [ ] DL-4. 다운로드 진행 데이터 수집 인터페이스 구현
- [ ] DL-5. 다운로드 오류 및 프로세스 종료 처리 구현

## Phase 4 — 다운로드 탭 및 Download Manager

다운로드 탭에서는 현재 실행 중인 작업과 대기/완료/실패 상태를 시각적으로 확인할 수 있어야 한다.

각 다운로드 항목에 가능한 범위에서 다음 정보를 표시한다.

- 썸네일
- 채널명
- 영상 제목
- 상태
- 진행률(%)
- 다운로드된 크기 / 전체 크기
- 다운로드 속도
- ETA
- 출력 경로

- [ ] DM-1. 앱 주요 탐색에 다운로드 탭 추가
- [ ] DM-2. 다운로드 Queue 구현
- [ ] DM-3. yt-dlp 출력 기반 진행률/속도/ETA 파싱
- [ ] DM-4. 진행 중 다운로드 Progress UI 구현
- [ ] DM-5. 대기/진행/완료/실패 상태 시각화
- [ ] DM-6. 다운로드 취소 처리
- [ ] DM-7. 중복 다운로드 방지
- [ ] DM-8. 다운로드 완료 후 결과 파일 경로 표시

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
