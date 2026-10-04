# DEVELOPMENT_PLAN — v0.1
## 프로젝트 목표

사용자가 치지직 채널을 검색해 해당 채널의 VOD 목록에서 영상을 선택하거나, 치지직 VOD URL을 직접 입력하여 원하는 영상을 yt-dlp로 다운로드할 수 있는 Windows 데스크톱 애플리케이션을 구현한다.

핵심 요구사항:

- 치지직 채널명 검색
- 채널 검색 결과 또는 저장 채널 선택
- 선택한 채널의 동영상 목록 조회
- 치지직 VOD URL 직접 입력 및 단일 VOD 조회
- 선택한 동영상 다운로드
- 다운로드 진행 상태 시각화
- 다운로드 디렉터리 설정
- 해상도 설정
- 출력 포맷 설정

## 화면 및 탐색 구조

탐색 방식은 두 가지로 제공한다.

1. **채널 검색**
   - 채널명 또는 검색어로 치지직 채널 사용자를 조회한다.
   - 검색 결과 또는 저장 채널에서 채널을 선택하면 해당 채널의 VOD 목록을 조회한다.
   - 채널 저장은 선택과 별개의 보조 기능이며 channelId를 기준으로 중복을 방지한다.

2. **URL 직접 입력**
   - 치지직 VOD URL(`https://chzzk.naver.com/video/{videoNo}`)을 직접 입력한다.
   - URL에서 videoNo를 추출하고 VOD 단건 정보를 조회한다.
   - 조회된 VOD는 채널 저장이나 채널 VOD 목록을 거치지 않고 바로 Queue에 추가할 수 있다.

다운로드 작업은 입력 방식과 분리된 별도 **다운로드 탭**에서 관리한다.

- 채널 검색
- URL 직접 입력
- 다운로드

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
- [x] FND-7. Wails/Yarn 개발 과정에서 생성되는 로컬 산출물 Git ignore 정리

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

### 기존 채널 URL 직접 입력 — 요구사항 정정으로 제품 흐름에서 제외

초기 구현에서는 URL 직접 입력을 채널 URL로 해석했으나, 요구사항 정정에 따라 URL 직접 입력은 **VOD URL 전용**으로 변경한다. 아래 CH-5~CH-8은 초기 구현 기록으로 보존하며 현재 제품 UI에서는 사용하지 않는다.

- [x] CH-5. 치지직 채널 URL 파싱, 정규화 및 channelId 추출 구현 — 초기 구현 기록
- [x] CH-6. 잘못된 도메인, 잘못된 경로, channelId 누락 등 URL Validation 구현 — 초기 구현 기록
- [ ] CH-7. 채널 URL 직접 입력 탭 UI 구현 — 요구사항 정정으로 대체
- [x] CH-8. 추출한 channelId로 비공식 채널 정보 API를 조회하여 실제 채널 여부 검증 — 초기 구현 기록

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

- [ ] VOD-5. 비공식 API 응답 변경 및 UI 오류 상태/재시도 처리 — 구현 완료, 전체 프론트엔드 검증 대기
- [ ] VOD-2C-1. 채널 전환 시 VOD 상태 초기화 — 구현 완료, 전체 프론트엔드 검증 대기
- [ ] VOD-2C-2. 페이지 병합 시 videoNo 기준 중복 제거 — 구현 완료, 전체 프론트엔드 검증 대기
- [ ] VOD-2C-3. Phase 3 다운로드에 전달할 선택 VOD 상태 확정 — 구현 완료, 전체 프론트엔드 검증 대기

#### Phase 2-C 구현 및 검증 현황

구현 완료:

- 초기 VOD 조회 실패와 추가 페이지 조회 실패 모두 오류 Alert에 `다시 시도` 액션 제공
- 실패한 요청의 page/append 상태를 보존해 동일 요청 단위로 재시도
- 요청 sequence를 사용해 채널 전환 또는 재요청 이후 늦게 도착한 이전 응답 무시
- 채널 변경 시 선택 VOD 상태 초기화
- 선택 채널 삭제 시 선택 채널 및 선택 VOD 상태 동시 초기화
- 페이지 병합 시 `videoNo` 기준 Map 병합으로 중복 제거
- VOD 카드에 명시적인 `다운로드 대상으로 선택` 액션과 선택 상태 표시
- 선택 VOD 상태를 `App` 레벨로 승격해 Phase 3 다운로드 로직에서 직접 재사용할 수 있도록 구성

검증 완료:

- 시스템 TypeScript 5.8.3을 사용한 격리 프론트엔드 검사
- 실제 React `useState<T>()` 형태를 반영한 최소 React/AntD 타입 스텁 환경에서 재검증
- 변경된 `App.tsx`, `ChannelVideoList.tsx`, Video/Backend 타입 간 시그니처 일관성 확인
- 채널 전환, 재시도, 중복 제거, 선택 VOD 상태 경로의 TypeScript 컴파일 확인

현재 실행 환경 제약으로 검증 대기:

- 실제 프로젝트 의존성을 사용한 `yarn typecheck`
- `yarn build`
- `wails build`
- 실제 치지직 VOD API와 Wails UI 통합 동작

위 전체 프론트엔드 검증이 완료되면 Phase 2-B의 VOD-3/VOD-4와 Phase 2-C 항목을 함께 완료 처리한다.

### Phase 2-D — VOD URL 직접 입력 흐름 정정

- [x] VOD-URL-1. `https://chzzk.naver.com/video/{videoNo}` URL 파싱 및 정규화
- [x] VOD-URL-2. videoNo 기반 `/service/v3/videos/{videoNo}` 단일 VOD 조회
- [x] VOD-URL-3. URL 직접 입력 탭에서 단일 VOD 메타데이터 표시
- [x] VOD-URL-4. 조회된 VOD를 기존 Download Queue에 직접 추가
- [x] VOD-URL-5. URL 직접 입력 탭에서 저장 채널/채널 VOD 목록 흐름 제거
- [x] VOD-URL-6. 채널 검색 결과의 채널 선택과 채널 저장 액션 분리

#### Phase 2-D 동작 기준

- 채널 검색은 채널 사용자를 찾는 기능이며, 검색 결과의 채널은 저장 여부와 관계없이 선택할 수 있다.
- 선택한 채널의 VOD 목록은 채널 검색 Workspace에서 표시한다.
- URL 직접 입력은 채널 URL을 허용하지 않고 치지직 VOD URL만 허용한다.
- VOD URL에서 videoNo를 추출한 뒤 단일 VOD 상세 API로 메타데이터를 확인한다.
- URL로 조회한 VOD는 별도의 채널 저장 없이 기존 `StartDownload` Queue 흐름을 그대로 사용한다.
- 동일 videoNo가 queued/running 상태라면 기존 Queue 중복 방지 정책을 그대로 따른다.

## Phase 3 — 단일 VOD 다운로드

### Phase 3-A — yt-dlp / ffmpeg 실행 기반

- [x] DL-1. yt-dlp 실행 경로 및 프로세스 래퍼 구현
- [x] DL-3. ffmpeg/ffprobe 연동 기반 구성
- [x] DL-3A-1. yt-dlp / ffmpeg / ffprobe 탐색 및 버전 확인 구현
- [x] DL-3A-2. CHZZK VOD URL과 기본 옵션을 yt-dlp 인자로 변환하는 Command Builder 구현
- [x] DL-3A-3. stdout/stderr 라인 스트리밍 및 exit code 기반 ProcessError 구현
- [x] DL-3A-4. Wails App에서 다운로드 Toolchain 상태 조회 기반 연결

#### Phase 3-A 실행 기준

도구 탐색 우선순위:

1. `CHZZK_DOWNLOADER_TOOLS_DIR` 환경 변수
2. 애플리케이션 실행 파일 옆 `tools` 디렉터리
3. 애플리케이션 실행 파일과 같은 디렉터리
4. 시스템 `PATH`

기본 yt-dlp 명령 정책:

- 사용자 전역 yt-dlp 설정의 영향을 받지 않도록 `--ignore-config` 사용
- 단일 VOD만 대상으로 `--no-playlist` 사용
- Windows 호환 파일명을 위해 `--windows-filenames` 사용
- 기존 파일을 덮어쓰지 않도록 `--no-overwrites` 사용
- 중단된 조각 다운로드 재개를 위해 `--continue` 사용
- 출력 라인 단위 처리를 위해 `--newline`, `--color never` 사용
- 기본 포맷 선택은 `bv*+ba/b`
- 기본 파일명은 `%(title)s [%(id)s].%(ext)s`
- ffmpeg / ffprobe가 같은 디렉터리에 있으면 `--ffmpeg-location`으로 해당 디렉터리를 전달
- ffmpeg / ffprobe가 서로 다른 위치에 있다면 둘 모두 PATH에서 탐색된 경우만 허용

#### Phase 3-A 검증 현황

완료:

- Phase 3-A와 동일한 `internal/downloader` 소스를 사용하는 격리 Go 모듈에서 `gofmt` 수행
- 동일 격리 모듈에서 `go test ./internal/downloader` 성공
- 도구 탐색 우선순위 및 실행 가능 상태 판정 테스트
- CHZZK VOD URL 검증/정규화와 기본 yt-dlp 인자 생성 테스트
- ffmpeg / ffprobe 배치 조건 검증 테스트
- stdout/stderr 라인 스트리밍 테스트
- 비정상 프로세스 종료 시 exit code 및 stderr tail 보존 테스트

현재 실행 환경 제약으로 검증 대기:

- 실제 Repository 전체 체크아웃 기반 `go test ./...`
- 실제 설치된 yt-dlp / ffmpeg / ffprobe를 이용한 통합 실행
- `wails build`

실제 VOD 다운로드 시작, 진행률 파싱, 완료 파일 경로 확보는 Phase 3-B에서 구현한다.

### Phase 3-B — 단일 다운로드 및 진행률

- [ ] DL-2. 선택한 치지직 VOD 실제 다운로드 구현 — 구현 완료, 실 도구 통합 검증 대기
- [ ] DL-4. yt-dlp progress template 기반 진행률/속도/ETA 파싱 — 구현 완료, 실 도구 통합 검증 대기
- [ ] DL-5. 다운로드 오류 및 프로세스 종료 처리 완성 — 구현 완료, 실 도구 통합 검증 대기
- [ ] DL-3B-1. context 기반 다운로드 취소 처리 — 구현 완료, 실 도구 통합 검증 대기
- [ ] DL-3B-2. 다운로드 완료 후 최종 파일 경로 확보 — 구현 완료, 실 도구 통합 검증 대기

#### Phase 3-B 구현 기준

- `Manager.Download`이 Prepare → yt-dlp 실행 → 진행률 파싱 → 최종 파일 경로 반환 흐름을 담당한다.
- `--progress-template`에 앱 전용 마커를 붙여 status, downloaded bytes, total bytes, estimated total, speed, ETA, percent를 안정적으로 구분한다.
- `--print after_move:filepath`에 별도 마커를 붙여 ffmpeg 병합/후처리 후 실제 최종 파일 경로를 확보한다.
- `--print`의 quiet 동작과 무관하게 진행률을 유지하도록 `--progress`를 명시하고, 실제 다운로드 보장을 위해 `--no-simulate`를 명시한다.
- 전체 크기가 없고 estimated total만 있으면 이를 `TotalBytes`로 사용하고 `TotalBytesEstimated=true`로 구분한다.
- 성공 종료인데 최종 파일 경로를 얻지 못한 경우 완료로 간주하지 않고 오류를 반환한다.
- context 취소/timeout은 `errors.Is`로 식별 가능한 상태를 유지한다.
- 프로세스 오류 메시지에서 앱 내부 progress/file marker는 제거하고 실제 stderr 오류만 보존한다.

#### Phase 3-B 검증 현황

완료:

- 실제 Phase 3-A Resolver/Runner 소스와 Phase 3-B 코드를 합친 격리 Go 모듈에서 `gofmt` 수행
- 동일 격리 모듈에서 `go test ./internal/downloader` 성공
- 동일 격리 모듈에서 `go vet ./internal/downloader` 성공
- progress template 인자 및 after_move filepath 인자 생성 테스트
- 실제/추정 전체 크기 fallback 진행률 파싱 테스트
- stdout 기반 진행률 + 최종 파일 경로 수집 테스트
- context timeout 기반 다운로드 취소 테스트
- 프로세스 실패 시 progress marker 제거 및 실제 stderr 보존 테스트
- 최종 파일 경로가 없는 성공 종료를 오류로 처리하는 테스트

현재 실행 환경 제약으로 검증 대기:

- 실제 설치된 yt-dlp를 이용한 CHZZK VOD 다운로드
- 실제 ffmpeg 영상/음성 병합 후 final filepath 확인
- Windows에서 장시간 다운로드 context 취소 동작
- 실제 Repository 전체 `go test ./...`
- `wails build`

위 실제 도구 통합 검증까지 완료되면 Phase 3-B 항목을 완료 처리한다.

### Phase 3-C — Wails / 프론트엔드 연결

- [ ] DL-3C-1. StartDownload / CancelDownload Wails API 정의 — 구현 완료, Wails 통합 검증 대기
- [ ] DL-3C-2. Go → React 다운로드 상태 이벤트 전달 — 구현 완료, Wails 통합 검증 대기
- [ ] DL-3C-3. Phase 2 선택 VOD를 다운로드 시작 동작에 연결 — 구현 완료, Wails 통합 검증 대기
- [ ] DL-3C-4. Phase 4 Download Manager에서 재사용할 다운로드 상태 모델 확정 — 구현 완료, Wails 통합 검증 대기

#### Phase 3-C 구현 기준

- Phase 3에서는 동시에 하나의 VOD만 다운로드하며 Queue/동시 다운로드 정책은 Phase 4에서 확장한다.
- Wails `StartDownload`은 즉시 Task 상태를 반환하고 실제 yt-dlp 다운로드는 goroutine에서 실행한다.
- Wails `CancelDownload(taskId)`는 활성 작업의 context를 취소한다.
- Go → React 상태 전달은 `download:state` 이벤트 하나로 통일한다.
- 이벤트 payload는 `DownloadTask` 전체 상태를 전달해 별도의 progress/completed/error 이벤트 모델을 만들지 않는다.
- `DownloadTask`에는 taskId, videoNo, 제목, 채널명, 썸네일, URL, 출력 경로, 상태, 진행률, 최종 파일 경로, 오류, 시작/종료 시각을 포함한다.
- 앱 종료 시 `OnShutdown`에서 활성 다운로드 context를 취소하고 종료 중에는 프론트 이벤트를 추가 전송하지 않는다.
- Settings가 구현되기 전 기본 저장 위치는 사용자 홈의 `Downloads/CHZZK Video Downloader`를 사용한다.
- 다운로드 시작 전 yt-dlp / ffmpeg / ffprobe 사용 가능 여부를 확인한다.
- Phase 3에서는 활성 다운로드가 있으면 두 번째 다운로드 시작을 거부하며 Phase 4에서 Queue로 교체한다.

#### Phase 3-C 검증 현황

완료:

- Wails runtime v2.15 문서 기준 `runtime.EventsEmit` / `window.runtime.EventsOn` API 확인
- Wails `OnShutdown func(context.Context)` lifecycle signature 확인
- Wails runtime을 최소 stub으로 대체한 격리 Go 모듈에서 App 다운로드 제어 코드 `go test ./...` 성공
- 동일 격리 Go 모듈에서 `go vet ./...` 성공
- StartDownload → progress → completed 상태 이벤트 단위 테스트
- 활성 작업 중 두 번째 다운로드 시작 거부 테스트
- CancelDownload → context 취소 경로 단위 테스트
- cancelled 최종 상태 이벤트 단위 테스트
- StartDownloadRequest validation / 기본 다운로드 경로 테스트
- Phase 3-C 프론트 연결 구조를 재현한 TypeScript 5.8.3 격리 fixture에서 `tsc --noEmit` 성공
- StartDownload / CancelDownload / Toolchain / DownloadTask TypeScript 시그니처 확인
- Wails EventsOn 구독과 DownloadTask 상태 갱신 타입 확인

현재 실행 환경 제약으로 검증 대기:

- 실제 Wails generated binding을 사용하는 `yarn typecheck`
- `yarn build`
- `wails build`
- 실제 Wails 창에서 Go EventsEmit → React EventsOn 전달
- 실제 yt-dlp/ffmpeg 다운로드 시작·진행·취소 UI 통합 동작

위 Wails/실 도구 통합 검증까지 완료되면 Phase 3-B/C의 검증 대기 항목을 완료 처리한다.

### Phase 3-D — Windows Download Tool Bundle

- [ ] DL-3D-1. Windows amd64용 yt-dlp / ffmpeg / ffprobe build-time 준비 스크립트 구현 — 구현 완료, Windows 실행 검증 대기
- [ ] DL-3D-2. 다운로드 바이너리 SHA-256 검증 및 bundle manifest 생성 — 구현 완료, Windows 실행 검증 대기
- [ ] DL-3D-3. Windows 빌드 시 다운로드 도구를 Go executable에 embed — 구현 완료, 실제 Wails build 검증 대기
- [x] DL-3D-4. 앱 최초 실행 시 embedded 도구를 사용자 LocalAppData 관리 디렉터리로 자동 추출
- [x] DL-3D-5. Resolver에 managed bundled tools 경로를 추가해 PATH 없이 실행 가능하게 구성
- [ ] DL-3D-6. Windows dev/build wrapper 및 Portable build helper를 bundle 준비 단계와 연결 — 구현 완료, Windows Wails/Portable 실행 검증 대기 (Phase 7에서 NSIS 배포 제외)
- [x] DL-3D-7. 기존 환경변수 / app-tools / PATH fallback 호환성 유지

#### Phase 3-D 배포 기준

- Windows amd64 기본 배포물은 사용자의 별도 yt-dlp / ffmpeg 설치에 의존하지 않는다.
- Windows build 전 공식 yt-dlp standalone exe와 FFmpeg Windows static build를 준비하고 SHA-256 checksum을 검증한다.
- 바이너리는 Git Repository에 commit하지 않고 build-time 생성물로 유지한다.
- 준비된 바이너리는 Go `embed`를 통해 앱 executable 내부에 포함한다.
- 앱 실행 시 `%LOCALAPPDATA%/CHZZK Video Downloader/tools`에 필요한 도구를 자동 materialize한다.
- Program Files 등 설치 디렉터리에 런타임 쓰기를 시도하지 않는다.
- Resolver 탐색 우선순위는 환경변수 override → managed bundle → 앱 옆 tools → 앱 디렉터리 → PATH 순으로 유지한다.
- Windows 개발/빌드는 `scripts/dev.ps1`, `scripts/build-windows.ps1`을 단일 진입점으로 사용하고 `$PSScriptRoot` 기반 절대경로로 tool 준비 스크립트를 호출한다.
- Phase 7 배포 방식은 단일 Portable EXE로 확정했다. 도구는 앱 executable 안에 포함하며 별도 외부 tool 파일 설치 규칙 없이 추출한다.
- Windows arm64 bundle은 현재 범위에 포함하지 않고 amd64 패키징을 기준으로 한다.

#### Phase 3-D 검증 현황

완료:

- bundle manifest SHA-256 검증 / 손상 bundle 거부 단위 테스트
- 최초 materialize / 동일 bundle 재사용 / bundle 변경 시 교체 단위 테스트
- managed bundle Resolver 우선 탐색 테스트 추가
- 기존 환경변수 / app-tools / PATH fallback 구조 유지
- Linux Go 1.23 격리 fixture에서 `gofmt`, `go test`, `go test -race`, `go vet` 성공
- placeholder-only bundle 상태에서 `GOOS=windows GOARCH=amd64 go test -c` cross-compile 성공
- Wails preBuild hook의 상대경로 작업 디렉터리 문제를 제거하고 dev/build wrapper에서 tool 준비를 선행하도록 수정
- 다운로드 도구 오류 UI가 ToolStatus.error 세부 원인을 표시하도록 보강

Windows 검증 대기:

- `scripts/prepare-windows-tools.ps1` 실제 실행 및 SHA-256 검증
- `scripts/dev.ps1` 실행 후 managed bundle Toolchain Ready 확인
- `scripts/build-windows.ps1` 실행 후 executable 내부 bundle materialize 확인
- Portable EXE에서 PATH에 yt-dlp/ffmpeg가 없는 환경의 Toolchain Ready 확인 (Phase 7에서 NSIS 배포 제외)

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

### Phase 4-A — Download Manager / Queue 백엔드

- [x] DM-2. 다운로드 Queue 구현
- [x] DM-7. queued/running 상태의 videoNo 기준 중복 다운로드 방지
- [x] DM-4A-1. DownloadTask Registry 및 등록 순서 기반 전체 작업 조회 구현
- [x] DM-4A-2. queued → running FIFO 자동 스케줄링 구현
- [x] DM-4A-3. 완료/실패/취소 Task를 메모리 Registry에 유지
- [x] DM-4A-4. 현재 동시 실행 수 1개 고정 및 Phase 5 확장 가능한 Executor/Queue 분리
- [x] DM-4A-5. 기존 StartDownload API를 Queue 등록 API로 호환 확장
- [x] DM-4A-6. GetDownloadTasks Wails API 및 프론트엔드 타입/wrapper 추가

#### Phase 4-A 동작 기준

- 첫 등록 작업은 즉시 `running`으로 전환하고 실제 실행은 goroutine에서 처리한다.
- 실행 중 작업이 있으면 이후 등록 작업은 FIFO `queued` 상태로 유지한다.
- 실행 작업이 완료/실패/취소되면 다음 queued 작업을 자동으로 시작한다.
- queued/running 상태인 동일 `videoNo`는 중복 등록을 거부한다.
- completed/failed/cancelled 상태 작업은 Registry에 남지만 동일 VOD의 재다운로드를 차단하지 않는다.
- Task Registry는 현재 메모리 기반이며 SQLite 영속화는 Phase 6에서 구현한다.
- Phase 4-A에서는 queued 작업 취소 UI/정책을 확장하지 않는다. 해당 범위는 Phase 4-B에서 처리한다.
- 동시 실행 수는 1개로 유지하며 설정 기반 동시 실행 수 확장은 Phase 5에서 처리한다.

#### Phase 4-A 검증 현황

완료:

- Phase 4-A Queue 소스를 재현한 격리 Go 모듈에서 `gofmt` 수행
- 동일 격리 Go 모듈에서 `go test ./internal/downloader` 성공
- 동일 격리 Go 모듈에서 `go vet ./internal/downloader` 성공
- 첫 작업 running / 두 번째 작업 queued 상태 검증
- FIFO 순서 및 이전 작업 종료 후 다음 작업 자동 시작 검증
- queued/running 동일 videoNo 중복 등록 차단 검증
- completed 이후 동일 VOD 재등록 허용 검증
- 실행 중 작업 취소 후 다음 queued 작업 자동 시작 회귀 검증
- Queue Stop 이후 신규 등록 거부 검증
- Wails runtime을 최소 stub으로 대체한 App 통합 fixture에서 `go test ./...` 성공
- 동일 App 통합 fixture에서 `go vet ./...` 성공
- App `StartDownload` → Queue 등록, `GetDownloadTasks`, 중복 차단, 다음 작업 자동 시작 경로 검증
- TypeScript 5.8.3 격리 fixture에서 queued 상태, `GetDownloadTasks()` 반환 타입, DownloadPanel status map 타입 확인

현재 실행 환경 제약으로 검증 대기:

- 실제 Repository 전체 `go test ./...`
- 실제 Wails generated binding 기반 `yarn typecheck`
- `yarn build`
- `wails build`
- 실제 yt-dlp 다운로드 여러 건을 등록한 FIFO 통합 동작

### Phase 4-B — Queue 제어 및 취소 안정화

- [x] DM-6. queued/running 작업 취소 처리
- [x] DM-4B-1. 실행 중 작업 실패/취소 후 다음 Queue 지속 실행 검증
- [x] DM-4B-2. 대기 작업 취소 시 Queue에서 제거하고 cancelled 상태 유지
- [x] DM-4B-3. 종료/취소 경합 시 Task 상태 일관성 보장

#### Phase 4-B 동작 기준

- `CancelDownload(taskId)`는 queued와 running 작업 모두 처리한다.
- queued 작업은 실제 executor 실행 전에 pending Queue에서 제거하고 즉시 `cancelled` 상태와 `finishedAt`을 기록한다.
- running 작업은 `cancelRequested`를 먼저 기록한 뒤 context를 취소한다.
- 취소 요청 이후 executor가 거의 동시에 성공 반환하더라도 `cancelRequested`가 있으면 최종 상태는 `cancelled`를 우선한다.
- 취소 요청 이후 도착한 progress callback은 Task 상태에 반영하지 않는다.
- failed/cancelled 작업 종료 후 Scheduler는 남은 queued 작업을 계속 실행한다.
- terminal 상태(completed/failed/cancelled)에 대한 재취소는 false를 반환하고 상태를 변경하지 않는다.
- Queue Stop 시 queued 작업은 실행하지 않고 cancelled로 확정하며, running 작업에는 취소 요청을 기록한 뒤 context를 취소한다.
- 기존 단일 다운로드 패널에서도 queued/running 상태 모두 취소 요청을 전달할 수 있도록 최소 호환 처리한다.

#### Phase 4-B 검증 현황

완료:

- Phase 4-B Queue 상태 머신을 재현한 Go 1.23.2 격리 모듈에서 `gofmt` 성공
- 동일 격리 모듈에서 `go test ./...` 성공
- 동일 격리 모듈에서 `go vet ./...` 성공
- queued 작업 취소 후 pending Queue에서 제거되고 실제 executor가 시작되지 않는지 확인
- running 작업 취소 후 다음 queued 작업 자동 시작 확인
- 실행 작업 실패 후 다음 queued 작업 자동 시작 확인
- context 취소를 무시하고 성공 반환하는 executor에서도 취소 요청이 최종 `cancelled` 상태를 우선하는 경합 테스트
- Queue Stop 시 running/queued 작업이 모두 cancelled로 수렴하는지 확인
- terminal 작업 재취소가 상태를 변경하지 않는지 확인
- Wails `CancelDownload` App 경로에 queued 취소 회귀 테스트 추가
- 기존 DownloadPanel에서 queued 작업에 `대기 취소` 액션을 노출하도록 최소 호환 처리

현재 실행 환경 제약으로 검증 대기:

- 실제 Repository 전체 `go test ./...`
- 실제 Wails generated binding 기반 `yarn typecheck`
- `yarn build`
- `wails build`
- 실제 yt-dlp 프로세스 종료 시점과 취소 요청이 겹치는 Windows 통합 동작

### Phase 4-C — Download Manager UI

- [ ] DM-1. 다운로드 탭을 다중 Task Manager 화면으로 확장 — 구현 완료, 전체 프론트엔드 검증 대기
- [ ] DM-4. 진행 중 다운로드 Progress UI를 Task 목록 단위로 확장 — 구현 완료, 전체 프론트엔드 검증 대기
- [ ] DM-5. 대기/진행/완료/실패/취소 상태 시각화 — 구현 완료, 전체 프론트엔드 검증 대기
- [ ] DM-8. 다운로드 완료 후 결과 파일 경로를 Task별 표시 — 구현 완료, 전체 프론트엔드 검증 대기
- [ ] DM-4C-1. VOD 화면의 다운로드 시작 동작을 Queue 추가 흐름으로 변경 — 구현 완료, 전체 프론트엔드 검증 대기
- [ ] DM-4C-2. GetDownloadTasks 초기 조회 + download:state 증분 이벤트 병합 — 구현 완료, 전체 프론트엔드 검증 대기

#### Phase 4-C 구현 기준

- 다운로드 탭은 단일 currentTask가 아니라 전체 `DownloadTask[]` Registry 상태를 표시한다.
- 앱 진입 시 `GetDownloadTasks()`로 초기 작업 스냅샷을 조회한다.
- 초기 조회 전에 `download:state` 이벤트를 먼저 구독해 초기화 중 발생하는 상태 이벤트를 놓치지 않는다.
- 초기 스냅샷과 실시간 이벤트가 경합하면 상태 단계와 진행률을 비교해 더 최신 Task 상태를 유지한다.
- queued → running → terminal(completed/failed/cancelled) 순으로 이전 상태가 최신 상태를 덮어쓰지 않도록 병합한다.
- 동일 running 상태끼리는 downloadedBytes와 percent가 더 큰 상태를 우선한다.
- VOD 카드의 기존 `다운로드 대상으로 선택` 단계를 제거하고 `Queue에 추가` 버튼에서 바로 `StartDownload`을 호출한다.
- queued/running 상태인 VOD 카드는 현재 상태를 표시하고 Queue 중복 추가 버튼을 비활성화한다.
- completed/failed/cancelled VOD는 다시 Queue에 추가할 수 있다.
- Download Manager 상단에는 전체/대기/진행/완료/실패 개수 요약을 표시한다.
- 각 Task 카드에는 썸네일, 채널명, 제목, 상태, Queue 순서, 진행률, 다운로드 크기/전체 크기, 속도, ETA, 저장 위치 또는 최종 파일 경로를 표시한다.
- queued/running Task는 각 카드에서 개별 취소할 수 있다.
- failed Task는 오류 메시지, cancelled Task는 취소 상태를 카드 내부에 표시한다.

#### Phase 4-C 검증 현황

완료:

- 시스템 TypeScript 5.8.3을 사용한 Phase 4-C 격리 프론트엔드 fixture에서 `tsc --noEmit` 성공
- React hook/JSX key를 실제 타입 형태에 맞춘 최소 React/AntD stub 환경에서 App, ChannelVideoList, DownloadPanel, downloadTasks 병합 유틸 타입 확인
- `GetDownloadTasks()` 초기 스냅샷과 `download:state` 이벤트 상태 병합 타입 확인
- downloadTasks 병합 유틸을 CommonJS로 컴파일해 running 진행률 역행 방지, terminal 상태 우선, 신규 Task append, snapshot/event merge 실행 테스트 성공
- VOD 카드 queued/running 상태 표시 및 Queue 중복 버튼 비활성화 타입 확인
- Task별 취소 버튼과 Progress 상태 매핑 타입 확인

현재 실행 환경 제약으로 검증 대기:

- 실제 프로젝트 의존성을 사용한 `yarn typecheck`
- `yarn build`
- `wails build`
- 실제 Wails 창에서 GetDownloadTasks 초기 조회와 download:state 이벤트 동시 수신
- 실제 여러 VOD Queue 등록 시 다운로드 탭의 실시간 Task 목록 갱신

위 실제 프론트엔드/Wails 통합 검증까지 완료되면 Phase 4-C 항목을 완료 처리한다.

### Phase 4-D — 통합 안정화 및 Phase 5 준비

- [x] DM-3. yt-dlp 출력 기반 진행률/속도/ETA 파싱의 다중 Task 통합 검증
- [x] DM-4D-1. 빠른 연속 Queue 등록 순서 검증
- [x] DM-4D-2. 완료/실패/취소 직후 다음 작업 시작 검증
- [x] DM-4D-3. Go/React 이벤트 순서 및 Task Registry 최종 일관성 검증
- [x] DM-4D-4. Phase 5 maxConcurrentDownloads 확장을 위한 Scheduler 구조 확정

#### Phase 4-D 안정화 기준

- 빠르게 여러 VOD를 등록해도 `order`와 FIFO `pending` 순서를 유지한다.
- 각 Task의 yt-dlp progress 파싱 결과는 해당 Task에만 반영되며 다른 Task의 진행률과 섞이지 않는다.
- Queue가 emit하는 상태는 Task 단위로 `queued → running → terminal` 순서를 유지하고 이전 상태로 역행하지 않는다.
- React의 snapshot/event 병합은 Go 이벤트 순서와 별개로 terminal 상태 및 더 높은 running 진행률을 우선한다.
- VOD Queue 등록 실패는 사용자 메시지만 표시하고 reject를 다시 던지지 않아 unhandled promise rejection을 만들지 않는다.
- Download Manager KPI에는 전체/대기/진행/완료/실패/취소 상태를 모두 포함한다.
- Queue는 런타임 `SetMaxConcurrent(maxConcurrent)` / `MaxConcurrent()`를 제공한다.
- 동시 실행 수 증가 시 빈 슬롯만큼 queued 작업을 즉시 시작한다.
- 동시 실행 수 감소 시 현재 실행 중인 작업은 강제 취소하지 않고 이후 Scheduler부터 새 제한을 적용한다.
- Phase 5에서는 Queue를 재생성하지 않고 설정값을 `SetMaxConcurrent`에 연결한다.

#### Phase 4-D 검증 현황

완료:

- Phase 4-D Scheduler 로직을 재현한 Go 1.23 격리 모듈에서 `gofmt` 성공
- 동일 격리 모듈에서 `go test ./...` 성공
- 동일 격리 모듈에서 `go test -race ./...` 성공
- 동일 격리 모듈에서 `go vet ./...` 성공
- 12개 Task 빠른 연속 등록 후 실행 순서가 등록 FIFO와 동일한지 확인
- maxConcurrent 1 → 2 증가 시 추가 queued Task가 즉시 실행되는지 확인
- maxConcurrent 2 → 1 감소 시 기존 active Task는 유지하고 새 Task는 제한이 충족될 때까지 대기하는지 확인
- 0 이하 동시성 값 및 Stop 이후 동시성 변경 거부 확인
- 실제 `parseProgressLine`을 거친 두 Task의 downloadedBytes / totalBytes / speed / ETA / percent가 서로 독립적으로 전달되는지 확인
- Queue event의 Task 상태가 queued → running → terminal 방향으로만 진행하는지 확인
- TypeScript 5.8.3으로 `downloadTasks` 병합 유틸 컴파일 성공
- 컴파일된 병합 유틸 실행 테스트에서 running progress 역행 방지, terminal 상태 우선, snapshot에 없는 이벤트 Task 보존 및 등록 순서 병합 확인
- Queue 등록 실패 재throw 제거로 unhandled promise rejection 경로 제거
- Download Manager KPI에 cancelled 개수 추가

현재 실행 환경 제약으로 검증 대기:

- 실제 Repository 전체 `go test ./...` / `go test -race ./...`
- 실제 프로젝트 의존성을 사용한 `yarn typecheck`
- `yarn build`
- `wails build`
- 실제 Wails 이벤트 전송 계층에서 다중 Task snapshot/event 경합 검증
- 실제 yt-dlp 여러 건 다운로드에서 progress/속도/ETA 분리 확인

Phase 4 내부 구현 및 격리 안정화 검증은 완료했으며, 위 항목은 실제 Wails/외부 도구 환경 통합 검증으로 유지한다.

## Phase 5 — Settings

### Phase 5-A — Settings 모델 / 백엔드 API

- [x] SET-5A-1. AppSettings 모델 및 기본값 정의
- [x] SET-5A-2. 다운로드 경로 / 해상도 / 출력 포맷 / 동시 다운로드 수 Validation 구현
- [x] SET-5A-3. 메모리 기반 Settings Store 구현
- [x] SET-5A-4. GetSettings / UpdateSettings Wails API 구현
- [x] SET-5A-5. 프론트엔드 AppSettings 타입 및 backend wrapper 추가
- [x] SET-5A-6. 다운로드 가속 설정 모델 / 기본값 / Validation 및 저장 호환성 구현
- [ ] SET-5A-7. 다운로드 속도 제한(MB/s) 모델 / Validation / 저장 호환성 구현 — 구현 완료, 실제 검증 대기

#### Phase 5-A 설정 기준

기본값:

- 다운로드 경로: 사용자 홈의 `Downloads/CHZZK Video Downloader`
- 해상도: `best`
- 출력 포맷: `mp4`
- 다운로드 가속: `standard` (fragment 동시 다운로드 수 `2`)
- 다운로드 속도 제한: `0 MB/s` (제한 없음)
- 동시 다운로드 수: `3`

지원 해상도:

- `best`
- `2160p`
- `1440p`
- `1080p`
- `720p`

지원 출력 포맷:

- `mp4`
- `mkv`
- `webm`

지원 다운로드 가속:

- `stable` → fragment 동시 다운로드 수 `1`
- `standard` → fragment 동시 다운로드 수 `2`
- `fast` → fragment 동시 다운로드 수 `4`
- `ultra` → fragment 동시 다운로드 수 `8`

Validation:

- 다운로드 경로는 비어 있을 수 없고 NUL 문자를 허용하지 않는다.
- 해상도와 출력 포맷은 위 허용 목록만 저장한다.
- 다운로드 가속은 `stable / standard / fast / ultra`만 저장하고 빈 값은 하위 호환을 위해 `standard`로 정규화한다.
- 다운로드 속도 제한은 유한한 `0` 이상의 MB/s 값만 저장하며 `0`은 제한 없음으로 처리한다.
- 동시 다운로드 수는 `1~8` 범위로 제한한다.
- UpdateSettings는 전체 설정을 검증한 후 한 번에 교체한다.
- 잘못된 업데이트는 기존 설정을 변경하지 않는다.
- 문자열 enum 값은 trim/lower-case 정규화 후 저장한다.

저장 정책:

- Phase 5에서는 앱 실행 중 메모리에만 설정을 보관한다.
- 앱 재시작 후 설정 영속화는 Phase 6의 SQLite `DB-4`에서 구현한다.
- Phase 5-A에서는 설정값을 다운로드 엔진/Queue에 적용하지 않는다. 실제 적용은 Phase 5-B에서 수행한다.

#### Phase 5-A 검증 현황

완료:

- Go 1.23 격리 Settings 모듈에서 `gofmt` 성공
- `go test ./...` 성공
- `go test -race ./...` 성공
- `go vet ./...` 성공
- 기본 다운로드 경로 입력 기반 기본 설정 생성 검증
- 해상도/출력 포맷 대소문자 및 공백 정규화 검증
- 지원하지 않는 해상도/포맷/다운로드 가속/동시 다운로드 수 거부 검증
- 다운로드 가속 기본값 `standard` 및 기존 설정 저장 버전 v1/v2의 기본 가속값 복원 검증
- 잘못된 Update가 기존 Store 값을 변경하지 않는지 검증
- Settings Store 동시 Get/Update race 검증
- Wails runtime 및 기존 App 의존성을 stub으로 대체한 App 통합 fixture에서 `go test ./...` / `go test -race ./...` / `go vet ./...` 성공
- App GetSettings / UpdateSettings 조회·정규화·실패 시 기존 값 보존 검증
- TypeScript 5.8.3 격리 fixture에서 AppSettings 및 GetSettings / UpdateSettings backend 계약 `tsc --noEmit` 성공

현재 실행 환경 제약으로 검증 대기:

- 실제 Repository 전체 `go test ./...` / `go test -race ./...`
- 실제 Wails generated binding 생성 결과 확인
- 실제 프로젝트 의존성 기반 `yarn typecheck`
- `yarn build`
- `wails build`

### Phase 5-B — 다운로드 엔진 설정 적용

- [x] SET-1. 다운로드 디렉터리 설정을 신규 Queue 작업에 적용
- [x] SET-2. 해상도 설정을 yt-dlp format selector로 변환
- [x] SET-3. 출력 포맷 설정을 yt-dlp/ffmpeg 옵션에 적용
- [x] SET-4. 동시 다운로드 수를 Queue SetMaxConcurrent에 연결
- [x] SET-5B-1. Queue 등록 시 설정 Snapshot을 DownloadRequest에 고정
- [x] SET-5B-2. 설정 변경 후 새 작업부터 변경값 적용
- [x] SET-5B-3. 다운로드 가속 설정을 yt-dlp fragment 동시 다운로드 옵션에 적용
- [ ] SET-5B-4. 다운로드 속도 제한을 Queue Snapshot 및 yt-dlp `--limit-rate`에 적용 — 구현 완료, 실제 검증 대기

#### Phase 5-B 적용 기준

- `StartDownload`은 Queue 등록 직전에 현재 `AppSettings` 전체를 한 번 읽어 다운로드 요청에 Snapshot으로 적용한다.
- 호출자가 넘긴 `OutputDir`, `FormatSelector`, `OutputFormat`보다 AppSettings를 우선한다.
- queued 작업은 등록 시점의 다운로드 경로/해상도/출력 포맷/다운로드 가속/다운로드 속도 제한 Snapshot을 유지한다.
- 설정 변경 후 이미 queued/running인 작업의 다운로드 옵션은 변경하지 않는다.
- 설정 변경 후 새로 Queue에 등록하는 작업부터 새 설정을 사용한다.
- 동시 다운로드 수는 개별 작업 Snapshot이 아니라 Scheduler 전역 정책으로 취급하며 `UpdateSettings` 즉시 `Queue.SetMaxConcurrent`에 반영한다.
- Queue가 아직 생성되지 않았다면 최초 생성 시 현재 Settings의 `maxConcurrentDownloads`를 사용한다.
- Queue 동시성 적용에 실패하면 Settings Store를 이전 값으로 복구한다.

해상도 → yt-dlp format selector:

- `best` → `bv*+ba/b`
- `2160p` → `bv*[height<=2160]+ba/b[height<=2160]`
- `1440p` → `bv*[height<=1440]+ba/b[height<=1440]`
- `1080p` → `bv*[height<=1080]+ba/b[height<=1080]`
- `720p` → `bv*[height<=720]+ba/b[height<=720]`

다운로드 가속 → yt-dlp concurrent fragments:

- `stable` → `--concurrent-fragments 1`
- `standard` → `--concurrent-fragments 2`
- `fast` → `--concurrent-fragments 4`
- `ultra` → `--concurrent-fragments 8`
- 신규 설치 및 기존 설정 마이그레이션의 기본값은 `standard`(`2`)로 한다.

다운로드 속도 제한:

- `0 MB/s` → `--limit-rate`를 생략해 제한 없이 다운로드한다.
- `0`보다 큰 값 → MB/s × 1,000,000을 bytes/s 정수로 변환해 `--limit-rate`에 전달한다.
- 속도 제한은 개별 다운로드 작업 Snapshot이며, 여러 작업을 동시에 실행하면 앱 전체 합산 대역폭은 설정값보다 커질 수 있다.

출력 포맷:

- `mp4 / mkv / webm`만 허용한다.
- 병합 컨테이너 지정에는 `--merge-output-format`을 사용한다.
- 이미 단일 컨테이너로 제공되는 영상도 최종 확장자를 설정값에 맞추기 위해 `--remux-video`를 함께 사용한다.
- 재인코딩은 수행하지 않고 ffmpeg remux 범위로 처리한다.

#### Phase 5-B 검증 현황

완료:

- Phase 5-B downloader/settings 실제 소스 조합을 재현한 Go 1.23 격리 모듈에서 `gofmt` 성공
- 동일 격리 모듈에서 `go test ./...` 성공
- 동일 격리 모듈에서 `go test -race ./...` 성공
- 동일 격리 모듈에서 `go vet ./...` 성공
- 모든 지원 해상도의 format selector 변환 검증
- Settings 적용 시 호출자 다운로드 옵션이 Snapshot 값으로 교체되는지 검증
- Command Builder에 `--format`, `--merge-output-format`, `--remux-video`, `--concurrent-fragments`가 함께 적용되는지 검증
- 다운로드 가속 4단계가 각각 fragment 동시 다운로드 수 `1 / 2 / 4 / 8`로 변환되는 격리 Go 테스트 성공
- 지원하지 않는 출력 포맷 거부 검증
- App/Wails 의존성을 stub으로 대체한 통합 fixture에서 `go test ./...` / `go test -race ./...` / `go vet ./...` 성공
- queued 작업이 설정 변경 후에도 등록 당시 경로/1080p/MKV Snapshot을 유지하는지 검증
- 설정 변경 후 신규 작업이 새 경로/720p/WebM 설정을 사용하는지 검증
- maxConcurrent 1 → 2 변경 시 기존 Queue 재생성 없이 대기 작업이 즉시 시작되는지 검증
- 최초 Queue 생성 시 현재 Settings의 maxConcurrent를 사용하는지 검증

현재 실행 환경 제약으로 검증 대기:

- 실제 Repository 전체 `go test ./...` / `go test -race ./...`
- 실제 yt-dlp + ffmpeg에서 각 해상도/출력 포맷 조합 다운로드
- 실제 Wails generated binding 기반 통합 실행
- `yarn typecheck`
- `yarn build`
- `wails build`

### Phase 5-C — Settings UI

- [ ] SET-5. 설정 UI 구현 — 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] SET-5C-1. 설정 버튼 및 우측 Drawer 구현 — 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] SET-5C-2. 다운로드 경로 표시/선택 UI 구현 — 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] SET-5C-3. 해상도 / 출력 포맷 / 동시 다운로드 수 입력 UI 구현 — 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] SET-5C-4. 저장 / Validation / 성공·실패 피드백 구현 — 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] SET-5C-5. 다운로드 가속 `안정 / 기본 / 고속 / 초고속` 선택 UI 구현 — 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] SET-5C-6. 다운로드 속도 제한 MB/s 입력 UI 구현 — 구현 완료, 실제 프론트엔드/Wails 검증 대기

#### Phase 5-C UI 기준

- 앱 헤더 우측에 `설정` 버튼을 표시한다.
- 설정은 우측 Drawer로 열고 Ant Design `destroyOnHidden`을 사용한다.
- Drawer가 열릴 때마다 `GetSettings()`를 다시 호출해 현재 백엔드 설정값으로 Form을 초기화한다.
- 다운로드 폴더는 직접 입력 대신 읽기 전용 경로 + `폴더 선택` 버튼을 제공한다.
- 폴더 선택은 Wails Go Runtime의 `OpenDirectoryDialog`를 사용한다.
- 폴더 선택 취소는 현재 Form 값을 변경하지 않는다.
- 해상도는 `최고 화질 / 2160p 이하 / 1440p 이하 / 1080p 이하 / 720p 이하`를 제공한다.
- 출력 포맷은 `MP4 / MKV / WebM`을 제공한다.
- 다운로드 가속은 `안정 (1) / 기본 (2) / 고속 (4) / 초고속 (8)`을 제공한다.
- 다운로드 속도 제한은 MB/s 단위 숫자로 입력하며 소수점 둘째 자리까지 허용하고 `0`은 제한 없음으로 안내한다.
- 동시 다운로드 수는 정수 `1~8`만 허용하며 기본값은 `3`이다.
- 저장 전 프론트 Form Validation을 수행하고, 최종 Validation은 기존 `UpdateSettings` 백엔드가 다시 수행한다.
- 저장 성공 시 성공 메시지를 표시하고 Drawer를 닫는다.
- 백엔드 오류는 Drawer를 유지한 채 사용자 메시지로 표시한다.
- 경로·해상도·포맷·다운로드 가속의 Queue Snapshot 정책과 동시 다운로드 수의 즉시 Scheduler 적용 정책을 Drawer 내부에 안내한다.
- Phase 6-P 완료에 따라 설정이 SQLite에 저장되고 앱 재시작 후 복원된다는 안내를 표시한다.

#### Phase 5-C 검증 현황

완료:

- Wails v2.15 공식 Runtime 문서에서 Go `OpenDirectoryDialog(ctx, OpenDialogOptions)` API와 취소 시 빈 문자열 반환 동작 확인
- 네이티브 폴더 선택 메서드를 재현한 Go 1.23.2 격리 fixture에서 `gofmt` 성공
- 동일 fixture에서 `go test ./...` 성공
- 동일 fixture에서 `go test -race ./...` 성공
- 동일 fixture에서 `go vet ./...` 성공
- 현재 Form 경로가 native dialog의 DefaultDirectory로 전달되는지 검증
- 현재 경로가 비어 있으면 Settings의 downloadDir을 native dialog 기본 경로로 사용하는지 검증
- 폴더 선택 취소 시 빈 문자열을 정상 처리하는지 검증
- native dialog 오류를 Wails API 오류로 래핑하는지 검증
- TypeScript 5.8.3 격리 fixture에서 SettingsDrawer `tsc --noEmit` 성공
- AppSettings Form, 다운로드 경로 선택, Select/InputNumber 값 타입, 저장 호출 타입 확인
- Header 설정 버튼 / 우측 Drawer 연결 구조 확인
- backend wrapper의 SelectDownloadDirectory 계약 추가

현재 실행 환경 제약으로 검증 대기:

- Node.js 24 + 실제 Yarn 의존성을 사용한 `yarn typecheck`
- `yarn build`
- 실제 Wails generated binding에서 SelectDownloadDirectory 생성 결과
- Windows 네이티브 OpenDirectoryDialog 실제 동작
- `wails build`
- 실제 앱에서 설정 저장 후 maxConcurrent / 신규 Queue 다운로드 옵션 반영 UI 통합

위 실제 프론트엔드/Wails 검증 완료 후 Phase 5-C 체크 항목을 완료 처리한다.

### Phase 5-D — 설정 통합 안정화

- [x] SET-5D-1. 설정 변경 후 신규 Queue 작업 적용 검증
- [x] SET-5D-2. 실행 중 작업의 설정 Snapshot 불변성 검증
- [x] SET-5D-3. maxConcurrent 증가/감소 Wails 통합 검증
- [x] SET-5D-4. 다운로드 경로 및 포맷/해상도 조합 검증
- [x] SET-5D-5. Phase 6 SQLite 설정 영속화용 Settings 구조 확정
- [ ] SET-5D-6. 다운로드 속도 제한 저장/복원 및 Queue Snapshot 회귀 검증 — 구현 완료, 실제 검증 대기

#### Phase 5-D 안정화 기준

- 실행 중 작업과 queued 작업의 다운로드 경로/해상도/출력 포맷/다운로드 가속/다운로드 속도 제한은 Queue 등록 시점 Snapshot을 끝까지 유지한다.
- Settings 변경 후 신규 Queue 작업부터 새 다운로드 옵션을 적용한다.
- `maxConcurrentDownloads` 증가 시 빈 Scheduler 슬롯만큼 queued 작업을 즉시 실행한다.
- `maxConcurrentDownloads` 감소 시 현재 실행 중인 작업을 강제 종료하지 않으며 active 수가 새 제한 미만이 될 때까지 신규 실행을 보류한다.
- 지원하는 해상도 5종 × 출력 포맷 3종, 총 15개 조합이 동일한 다운로드 경로와 올바른 yt-dlp 인자로 변환되어야 한다.
- Phase 6 설정 영속화 경계는 API용 `AppSettings`와 분리된 `StorageRecord`를 사용한다.
- `StorageRecord` v4는 `schemaVersion / downloadDir / resolution / outputFormat / downloadAcceleration / downloadRateLimitMBps / maxConcurrentDownloads / theme`을 저장한다.
- 기존 v1/v2/v3 레코드는 누락된 테마·다운로드 가속·다운로드 속도 제한을 각 버전의 기본값으로 호환 복원한다.
- 저장 레코드를 복원할 때도 Normalize/Validate를 다시 수행하고 지원하지 않는 schemaVersion은 거부한다.
- SQLite settings는 migration 7에서 `download_rate_limit_mbps` 컬럼을 추가하고 이후 구조 변경은 schemaVersion 기반 마이그레이션으로 처리한다.

#### Phase 5-D 검증 현황

완료:

- Settings StorageRecord v2를 재현한 Go 1.23 격리 모듈에서 `gofmt` 성공
- 동일 모듈에서 `go test ./internal/settings` 성공
- 동일 모듈에서 `go test -race ./internal/settings` 성공
- 동일 모듈에서 `go vet ./internal/settings` 성공
- StorageRecord JSON encode/decode → AppSettings round-trip 검증
- 지원하지 않는 StorageRecord schemaVersion 거부 검증
- 잘못된 저장 설정값 복원 거부 검증
- downloader/settings/command 조합을 재현한 Go 1.23 격리 모듈에서 `go test`, `go test -race`, `go vet` 성공
- 5개 해상도 × 3개 출력 포맷 = 15개 조합의 `--format / --paths / --merge-output-format / --remux-video` 검증
- App/Queue/Settings 통합 fixture에서 `go test`, `go test -race`, `go vet` 성공
- maxConcurrent 2 → 1 감소 시 기존 active 작업 유지 및 세 번째 작업 대기 검증
- 실행 중 작업의 Settings Snapshot 불변성 검증
- queued 작업의 Settings Snapshot 불변성 재검증
- Settings 변경 이후 신규 작업에 새 경로/해상도/포맷 적용 검증
- Phase 5-C 테스트에서 누락된 `errors` import를 발견해 수정

현재 실행 환경 제약으로 검증 대기:

- 실제 Repository 전체 `go test ./...` / `go test -race ./...`
- 실제 Node.js 24 + Yarn 의존성 기반 `yarn typecheck` / `yarn build`
- `wails build`
- 실제 Windows Wails 앱에서 Settings Drawer 저장 → Scheduler 증가/감소 통합 동작
- 실제 yt-dlp + ffmpeg를 사용한 15개 조합 다운로드 결과 검증

Phase 5 내부 구현과 격리 통합 안정화는 완료했으며, 실제 Windows/Wails/외부 도구 검증은 배포 환경 통합 검증으로 유지한다.

## Phase 5-E — 인증 세션 다운로드

- [x] SET-5E-1. `cookies.txt` 토큰 인증 모델을 일반 다운로드 설정과 분리하고 기본값/Validation 구현
- [x] SET-5E-2. 상단 헤더에 별도 인증 버튼과 전용 Drawer 구현
- [x] SET-5E-3. `cookies.txt` 파일 선택 기반 토큰 인증 UI 구현
- [x] SET-5E-4. 인증 설정 SQLite 영속화 및 migration 8 추가
- [x] SET-5E-5. Queue 등록 시 `cookies.txt` 경로를 DownloadRequest Snapshot으로 고정
- [x] SET-5E-6. 일반 다운로드와 HLS probe/구간 분할/ffmpeg fallback의 모든 yt-dlp 호출에 동일 `--cookies` 인자 적용
- [x] SET-5E-7. 실패 로그의 `--cookies` 경로 마스킹
- [ ] SET-5E-8. 실제 Windows에서 `cookies.txt` 토큰 인증 다운로드 검증
- [x] SET-5E-9. 헤더 상태를 비활성 시 `인증 미사용`, 활성 시 `토큰 인증`으로 표시
- [x] SET-5E-10. 토큰 인증 Drawer의 기존 `cookies.txt` 생성 방법을 유지하되 파일 첫 줄 확인 항목만 제거하고 세션 만료/보안 안내를 하나의 주의사항 블록으로 구성
- [x] SET-5E-11. 사용자 제공 화면을 포함한 Get cookies.txt LOCALLY 생성 도움말 Drawer 및 3단계 안내 구현
- [x] SET-5E-12. 생성 도움말 진입 링크를 칩형 버튼으로 변경해 가시성 개선
- [x] SET-5E-13. 주의사항 두 문장을 bullet 목록으로 표시해 항목 구분 강화
- [x] SET-5E-14. 파일 선택 영역 라벨을 `cookies.txt 경로 설정`으로 명확화
- [x] SET-5E-15. 생성 도움말 칩 버튼을 하늘색 배경/파란색 윤곽선으로 강조
- [x] SET-5E-16. 인증 안내 info/exclamation 아이콘 크기를 축소
- [x] SET-5E-17. 인증 안내 아이콘의 제목 정렬을 보정하고 도움말 칩 색상을 다크/라이트 테마 변수로 분리
- [x] SET-5E-18. 인증 Drawer 문구를 `인증 설정`/`인증파일 사용`/`인증기능 사용` 기준으로 정리하고 Alert 아이콘 정렬 재보정
- [x] SET-5E-19. `cookies.txt 생성 방법` info 및 `주의사항` exclamation 아이콘 상단 마진을 6px로 통일
- [ ] SET-5E-20. cookies.txt 도움말 이미지 경로를 `frontend/src/assets/cookies-txt-guide.png`로 변경 — PNG 파일 직접 추가 대기
- [x] SET-5E-21. 인증 사용 시 `SearchChannels`, `GetChannelVideos`, `GetVideo` CHZZK API 요청에 `cookies.txt` 쿠키 전달
- [x] SET-5E-22. 인증 설정의 사용 여부와 `cookies.txt` 경로가 SQLite 저장 후 앱 재실행에서도 복원되는 회귀 테스트 추가
- [x] SET-5E-23. `인증기능 사용` 스위치를 변경 즉시 저장하고 헤더 인증 상태에도 즉시 반영
- [x] SET-5E-24. `cookies.txt` 파일 선택 즉시 경로를 저장·반영하고 인증 Drawer의 별도 저장/취소 버튼 제거
- [x] SET-5E-25. 저장된 인증 설정이 `enabled=true`/빈 경로인 경우 시작 시 인증을 비활성화해 SQLite를 자동 복구하고 Persistence 전체 초기화 실패 방지
- [x] SET-5E-26. 선택 채널 VOD 목록이 표시된 상태에서 인증 사용 여부가 변경되면 VOD 첫 페이지를 즉시 재조회

#### Phase 5-E 인증 기준

- 인증 기능은 일반 설정 Drawer에 포함하지 않고 앱 헤더 우측의 별도 인증 버튼에서 관리한다.
- 인증 Drawer 제목은 `인증 설정`, 섹션 제목은 `인증파일 사용`, 스위치 라벨은 `인증기능 사용`으로 표시한다.
- 인증 방식은 `cookies.txt`를 사용하는 토큰 인증만 제공하며 브라우저 로그인 세션 직접 추출 기능은 제공하지 않는다.
- 토큰 인증 Drawer의 `cookies.txt 생성 방법` 안내는 기존 문구를 유지하고 `파일 첫 줄이 # Netscape HTTP Cookie File 또는 # HTTP Cookie File인지 확인` 단계만 제외한다. 생성 도움말 진입 링크는 해당 안내 블록 하단에 칩형 버튼으로 배치한다.
- 생성 도움말은 사용자 제공 Get cookies.txt LOCALLY 화면을 표시하고 CHZZK 로그인 후 임의의 라이브 혹은 영상 페이지 열기/확장 프로그램 실행 → `Netscape` 선택 → `Export` 및 앱 파일 선택의 3단계로 안내한다.
- 별도의 파일 첫 줄 검사 단계는 사용자 안내에서 제외한다.
- 토큰 인증 Drawer의 경고 블록에는 세션 만료 시 `cookies.txt` 재발급 안내와 로그인 세션 정보 공유 금지를 각각 bullet 항목으로 표시한다.
- 도움말 Drawer 하단에는 `주의사항`으로 cookies.txt 공유 금지와 현재 CHZZK 페이지의 `Export` 사용 기준을 명시한다.
- 인증이 비활성화된 경우 헤더 버튼은 `인증 미사용`, 활성화된 경우 `토큰 인증`으로 표시한다.
- `cookies.txt` 파일 자체를 데이터베이스에 복사하지 않고 선택한 파일 경로만 저장하며 yt-dlp `--cookies`에 전달한다.
- 인증 설정의 `enabled`와 `cookies_file_path`는 `authentication_settings`에 저장하며 앱 재실행 시 동일 값을 복원한다.
- 저장된 인증 설정이 `enabled=true`인데 `cookies_file_path`가 비어 있거나 `.`인 경우 앱 시작 시 `enabled=false`, 빈 경로로 자동 복구해 인증 설정 오류가 전체 Persistence 초기화를 막지 않도록 한다.
- `인증기능 사용` 스위치 변경 시 별도 저장 버튼 없이 즉시 `enabled` 값을 SQLite에 저장하고 App의 인증 상태를 갱신한다.
- `cookies.txt` 파일 선택 직후 `cookies_file_path`를 SQLite에 저장하고 App의 인증 상태를 갱신한다. 인증 Drawer에는 별도 저장/취소 버튼을 두지 않는다.
- 최초 인증 활성화를 위해 인증이 꺼진 상태에서도 `cookies.txt` 경로는 선택할 수 있으며, 경로가 없는 상태에서 활성화를 시도하면 활성화하지 않고 파일 선택을 안내한다.
- CHZZK API 조회는 인증 활성 시 `SearchChannels`, `GetChannelVideos`, `GetVideo` 요청에 동일 `cookies.txt`를 적용하고, 인증 비활성 시 기존 비인증 요청을 유지한다.
- 선택 채널의 VOD 목록이 표시 중인 경우 인증 사용 여부(`enabled`)가 바뀌면 현재 목록을 초기화하고 `GetChannelVideos` 첫 페이지를 다시 요청해 변경된 인증 상태를 즉시 반영한다.
- API 요청용 `cookies.txt`는 Netscape 형식의 domain/path/secure/expiry 조건을 적용해 요청 URL에 일치하는 쿠키만 `Cookie` 헤더로 전달한다.
- 썸네일 및 채널 이미지 로딩 방식은 이번 범위에서 변경하지 않는다.
- 인증 토큰/쿠키 원문은 앱 설정이나 다운로드 이력에 별도로 저장하지 않는다.
- Queue에 등록된 작업은 등록 시점의 `cookies.txt` 경로 Snapshot을 유지하고, 이후 인증 설정 변경은 새 작업부터 적용한다.
- HLS fallback의 format probe, split format probe, 구간 다운로드, ffmpeg downloader 재시도에도 최초 작업과 동일한 인증 Snapshot을 사용한다.
- 다운로드 오류 로그에서 `--cookies`의 파일 경로와 URL의 민감 query/fragment를 마스킹한다.

#### Phase 5-E 검증 현황

구현/정적 확인 완료:

- 인증 데이터가 AppSettings와 분리된 별도 모델/Store/API/SQLite 테이블을 사용하는 구조 확인
- 일반 Settings 저장이 인증 값을 덮어쓰지 않는 독립 저장 경계 확인
- 활성화된 토큰 인증은 yt-dlp `--cookies path`로 변환되고 비활성화 시 인증 인자를 전달하지 않는 구조 확인
- 일반 다운로드와 세 가지 HLS 대체 실행 경로에 공통 인증 옵션 전달 확인
- 진단 로그의 command line과 verbose process output에서 `--cookies` 파일 경로 마스킹 경로 확인
- 브라우저 선택/프로필/`--cookies-from-browser` 관련 설정 및 실행 경로 제거
- 토큰 인증 Drawer의 기존 `cookies.txt 생성 방법` 문구 유지, 파일 첫 줄 확인 항목 제거, 생성 도움말 링크 및 단일 경고 블록 구조 확인
- 사용자 제공 이미지 asset과 Get cookies.txt LOCALLY 3단계 도움말 Drawer 연결 확인
- 세 CHZZK API 경로에 인증용 `WithCookiesFile` 호출이 연결되고 비활성 시 빈 경로로 기존 요청을 유지하는 구조 정적 확인
- `SearchChannels`, `GetChannelVideos`, `GetVideo` 각각에 cookies.txt 쿠키 전달 테스트 추가
- 인증 설정 저장 후 앱 저장 → DB 종료 → 새 App 인스턴스에서 재오픈하여 사용 여부와 cookies.txt 경로가 복원되는 테스트 추가
- 인증 Drawer 스위치의 `onChange`가 즉시 `UpdateAuthenticationSettings`를 호출하고 성공 시 `onAuthenticationUpdated`로 헤더 상태를 갱신하는 구조 정적 확인
- 인증 파일 선택 성공 직후 `UpdateAuthenticationSettings`를 호출해 경로를 저장하고 `onAuthenticationUpdated`로 즉시 반영하는 구조 정적 확인
- 잘못된 기존 인증 설정(`enabled=1`, 빈 경로)을 Load 시 자동 비활성화하고 SQLite에도 수정된 값을 다시 저장하는 회귀 테스트 추가
- 동일 잘못된 DB 상태에서 `initializePersistence()`가 실패하지 않고 인증 비활성 상태로 시작하는 회귀 테스트 추가
- `ChannelVideoList`가 `authenticationEnabled` 변경을 첫 페이지 조회 effect의 dependency로 사용해 인증 상태 변경 시 VOD 목록을 재조회하는 구조 정적 확인

실 환경 검증 대기:

- 실제 Windows Wails binding 생성 및 앱 실행
- 실제 Netscape 형식 `cookies.txt` 선택 및 인증 필요 VOD 다운로드
- Node.js 24 + Yarn 기반 frontend typecheck/build
- Repository 전체 Go test/race/vet 및 Wails build

## Phase 6 — Persistence 및 UX Refinement

### Phase 6-P — Persistence

- [ ] DB-1. SQLite 초기화 및 마이그레이션 구조 구성 — 구현 완료, 실제 Go SQLite driver 통합 검증 대기
- [ ] DB-2. 저장 채널 영속화 — 구현 완료, 실제 Go SQLite driver 통합 검증 대기
- [ ] DB-3. 다운로드 이력 영속화 — 구현 완료, 실제 Go SQLite driver 통합 검증 대기
- [ ] DB-4. 설정 영속화 — 구현 완료, 실제 Go SQLite driver 통합 검증 대기

#### Phase 6-P 저장 기준

- SQLite는 CGO 없이 Windows 빌드가 가능한 `modernc.org/sqlite` driver를 사용한다.
- 데이터베이스 기본 위치는 OS 사용자 설정 디렉터리 아래 `CHZZK Video Downloader/data.sqlite3`로 한다.
- SQLite 연결은 앱 단일 로컬 DB 사용 패턴에 맞춰 최대 connection 수를 1로 제한한다.
- `foreign_keys=ON`, `busy_timeout=5000`, `journal_mode=WAL`을 적용한다.
- `schema_migrations` 테이블과 순차 migration version으로 schema 변경을 관리한다.
- migration v1은 `saved_channels`, `download_tasks`, `app_settings`를 생성하고, migration v2는 `app_settings.theme`, migration v3는 다운로드 `error_code`, migration v4는 실패 진단 `log_path`, migration v5는 `app_settings.download_acceleration`을 추가하고 설정 저장 버전을 v3으로 올린다. migration v6는 기존에 임시 데이터 충돌로 잘못 분류된 HLS initialization fragment 오류 이력을 `hls_initialization_fragment_order`로 교정한다.
- 저장 채널은 앱 시작 시 SQLite에서 읽어 기존 `chzzk.Store`에 복원한다.
- 채널 추가/삭제는 메모리 Store와 SQLite를 함께 갱신하며 SQLite 저장 실패 시 메모리 변경을 rollback한다.
- 모든 DownloadTask 상태 이벤트(queued/running/progress/terminal)를 taskId 기준 upsert한다.
- 앱 재시작 시 이전 queued/running 이력은 실제 프로세스가 존재하지 않으므로 cancelled로 복구하고 finishedAt과 중단 오류를 기록한다.
- 완료/실패 다운로드 이력은 앱 재시작 후에도 다운로드 탭에서 조회할 수 있다. 사용자가 취소한 작업은 즉시 목록에서 제거하고 SQLite 취소 이력도 삭제하며, 비정상 종료로 복구된 cancelled 이력은 UI에서 숨긴다.
- 재시작 간 download taskId 충돌을 방지하기 위해 Task ID에 UTC UnixNano와 process-local counter를 함께 사용한다.
- Settings는 `StorageRecord v2`를 SQLite 단일 row(id=1)에 저장하며 theme을 함께 영속화한다.
- 앱 시작 시 저장된 Settings를 복원하고 해당 `maxConcurrentDownloads`로 Queue를 생성한다.
- Settings 저장 실패 시 메모리 Settings와 Queue 동시성 값을 이전 상태로 rollback한다.
- Persistence 초기화 실패 시 앱은 기존 메모리 기반 동작으로 fallback하고 Wails error log를 남긴다.
- Phase 6-P에서는 Windows 패키징, yt-dlp/ffmpeg 번들링, WebView2 배포 정책을 변경하지 않는다.

#### Phase 6-P 검증 현황

구현/정적 확인 완료:

- `internal/persistence`에 SQLite open / migration / channel / download history / settings repository 구현
- migration v1/v2 SQL을 SQLite 호환 문법 기준으로 확인
- 저장 채널 add/delete/restore 경로 구현
- DownloadTask 전체 필드 및 Progress 필드 저장/복원 경로 구현
- queued/running 중단 이력 cancelled 복구 경로 구현
- Settings StorageRecord v2 저장/복원 및 v1 Dark 호환 복원 경로 구현
- 앱 startup에서 DB → 채널/Settings 복원 → Queue 생성 순서 연결
- Settings 변경 시 SQLite 저장과 Scheduler rollback 경로 연결
- 다운로드 이벤트 → SQLite upsert → Wails event 순서 연결
- 다운로드 이력과 현재 Queue Task merge 경로 구현
- Settings Drawer의 재시작 초기화 안내를 SQLite 영속화 안내로 갱신
- 저장 채널 복원용 `Store.ReplaceAll` 구현 및 단위 테스트 추가
- Persistence repository 및 App 재시작 시나리오 테스트 코드 추가

현재 실행 환경 제약으로 검증 대기:

- `modernc.org/sqlite` module 다운로드가 DNS/network 차단으로 불가능하여 실제 Go SQLite driver 기반 `go test ./...` 미실행
- 실제 Repository `go mod tidy` 및 go.sum 생성 확인
- 실제 Go SQLite driver로 migration idempotency / CRUD / interrupted recovery 테스트 실행
- 실제 Wails 앱을 재시작해 저장 채널 / 다운로드 이력 / Settings 복원 확인
- Node.js 24 + Yarn 기반 `yarn typecheck` / `yarn build`
- `wails build`

위 실제 SQLite driver 통합 검증이 완료되면 DB-1~DB-4를 완료 처리한다.

### Phase 6-UX — UX Refinement

#### Phase 6-UX-A — App Shell / Navigation

- [ ] UX-A1. 헤더와 탭의 시각적 위계 정리 — 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] UX-A2. 개발 Phase 표기 등 사용자에게 불필요한 내부 정보 제거 — 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] UX-A3. 주요 화면의 최대 너비, 여백, 카드 밀도와 반응형 레이아웃 통일 — 1차 Shell 구현 완료, 후속 화면 UX 단계에서 계속 보강
- [ ] UX-A4. 로딩 / Empty / Error 상태의 기본 표현 규칙 통일

#### Phase 6-UX-B — Channel / VOD 탐색

- [ ] UX-B1. 채널 검색과 URL 직접 입력의 입력/결과 레이아웃 정리 — 채널 검색 Workspace 및 URL 단건 상세 레이아웃 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] UX-B2. 저장 채널 목록의 선택/삭제 동작과 현재 선택 상태 가독성 개선 — 좌측 검색/북마크 탭, 전체 카드 선택, 검색/북마크 목록의 별 북마크 토글까지 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] UX-B3. VOD 카드의 정보 우선순위와 다운로드 추가 CTA 정리 — 태그 chip, 하단 정렬, 폰트/CTA 간격/문구, 썸네일 실패 상태 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] UX-B4. 채널 전환과 VOD 더보기 흐름의 상태 피드백 개선
- [x] UX-B5. 검색/북마크 내부 탭 전환 시 ChannelSearchTab을 unmount하지 않고 검색어·검색 결과·더보기 누적 상태 유지

#### Phase 6-UX-C — Download Manager

- [ ] UX-C1. 다운로드 탭의 활성 작업과 완료 이력 시각적 구분 — 섹션형 Workspace 구현 완료, 실제 Wails 검증 대기
- [ ] UX-C2. Task 상태/진행률/속도/ETA/저장 위치 정보 밀도 개선 — compact row UI 구현 완료, 실제 Wails 검증 대기
- [ ] UX-C3. Queue 대기 순서와 취소 액션의 가시성 개선 — 취소 즉시 제거 및 상태 전이 안정화 구현 완료, 실제 다운로드 검증 대기
- [ ] UX-C4. Toolchain 경고와 다운로드 오류 메시지의 우선순위 정리 — 기능 중심 상태 문구, 401 인증 필요/부분 데이터 충돌 오류 분류 구현 완료, 실제 Wails 검증 대기
- [ ] UX-C5. 완료 다운로드의 폴더 열기 / 목록 삭제 및 실패 다운로드의 목록 삭제 액션 — 구현 완료, Windows Explorer 및 Persistence 통합 검증 대기
- [ ] UX-C6. VOD별 임시 다운로드 경로 격리 및 임시 데이터 충돌 복구 액션 — 구현 완료, 실제 Windows 다운로드 검증 대기
- [ ] UX-C7. 다운로드 실패 진단 로그 저장 및 실패 항목 로그 파일 열기 — 구현 완료, 실제 반복 실패 VOD 검증 대기
- [ ] UX-C8. HLS initialization fragment 순서 오류 감지, discontinuity 분할 다운로드 + ffmpeg fallback 및 실행 상태 관측 — 구현 완료, VOD 15461111 실제 검증 대기

#### Phase 6-UX-D — Settings / Feedback / Accessibility

- [ ] UX-D0. Light / Dark 테마 선택, 즉시 UI 적용 및 SQLite 영속화 — 구현 완료, 실제 프론트엔드/Wails 검증 대기
- [ ] UX-D1. Settings Drawer의 섹션 구조와 설명 문구 간결화 — 화면/다운로드 섹션 1차 정리 완료, 후속 UX 점검 대기
- [ ] UX-D2. 저장/취소/폴더 선택 액션의 상태 피드백 통일 — 공통 Message를 헤더 아래에서 표시하도록 위치 보정 완료, 실제 Wails 시각 검증 대기
- [ ] UX-D3. 키보드 포커스, 버튼 상태, 텍스트 대비 등 기본 접근성 점검
- [ ] UX-D4. 전체 UI에서 버튼/Tag/Alert/Empty/Skeleton 표현 일관성 점검

#### Phase 6-UX 이번 구현 기준

- 헤더는 `[앱 이름] [채널 검색 / URL 직접 입력 / 다운로드] [설정 아이콘]`의 단일 행 구조로 구성한다.
- 기존 본문 Tabs는 제거하고 헤더 중앙 pill navigation으로 이동한다.
- 사용자에게 불필요한 Phase / Wails 개발 표시는 제품 UI에서 제거한다.
- 헤더 설정 아이콘은 34×34px rounded 사각형 outlined 버튼으로 표시해 탐색 탭과 시각적으로 분리한다.
- 설정 아이콘은 Ant Design의 `.ant-btn-icon` 래퍼와 SVG까지 flex/block으로 정렬해 버튼 내부에서 시각적으로 정중앙에 배치한다.
- 공통 Ant Design Message는 헤더 높이 68px과 본문 상단 여백 16px을 반영해 `top: 84px`에서 시작하고, 헤더 중앙 탭을 가리지 않는다.
- 채널 검색 화면은 좌측 채널 탐색 패널 + 우측 VOD 패널의 2열 Workspace로 구성한다.
- 좌측 채널 탐색 패널은 `검색 / 북마크` 내부 탭으로 전환한다.
- 검색/북마크 내부 탭은 두 View를 모두 mount한 상태로 유지하고 비활성 View만 숨겨, 탭 전환 시 검색어·검색 결과·더보기 누적 결과가 초기화되지 않도록 한다.
- 우측 패널은 선택한 채널의 VOD 목록을 항상 표시하는 전용 영역으로 사용한다.
- VOD 패널 헤더의 `VOD / 채널명` 왼쪽에 선택 채널 프로필 이미지를 표시한다.
- 검색 결과 채널 카드는 별도 선택 버튼 없이 카드 전체 클릭/Enter/Space로 선택한다.
- 검색 결과 카드 우상단에는 별 아이콘을 표시하고 클릭 시 북마크 추가/해제를 토글한다.
- 북마크 해제는 현재 채널 선택 상태를 해제하지 않는다.
- 검색 결과 카드 hover에서는 위치 이동 transform을 사용하지 않고 Border/Shadow만 변경해 스크롤 컨테이너 경계에서 Border가 잘리지 않도록 한다.
- 채널 검색의 설명 문구는 제거하고 검색 입력과 결과에 바로 접근하도록 한다.
- 헤더 아래 남은 높이를 Workspace가 채우며 app-content 자체는 스크롤하지 않는다.
- 좌측 검색/북마크 목록과 우측 VOD 목록은 각각 `min-height: 0` + `overflow-y: auto`로 독립 스크롤한다.
- 다운로드 탭은 별도의 `app-scroll-view`에서 화면 단위 스크롤을 유지한다.
- URL 직접 입력 탭은 채널 검색과 동일하게 헤더 아래 남은 높이를 채우는 단일 Workspace panel로 구성한다.
- URL 입력 영역은 상단에 고정하고 단건 VOD 결과 영역만 `min-height:0 + overflow-y:auto`로 스크롤한다.
- URL 단건 결과는 좌측 16:9 썸네일, 우측 영상 제목/채널/게시일/재생시간/조회/태그/다운로드 CTA의 2열 상세 레이아웃으로 표시한다.
- URL 단건 썸네일도 이미지 없음/로드 실패 시 동일한 fallback 안내를 사용한다.
- URL 결과가 없는 초기 상태와 조회 실패/빈 결과 상태는 결과 영역 중앙 Empty 상태로 표시한다.
- 900px 이하에서는 URL 단건 결과를 1열로 전환해 썸네일 위 / 영상 정보 아래로 배치한다.
- 기본 gap은 Workspace 16px, 내부 목록 8~12px로 통일해 불필요한 세로 공백을 줄인다.
- VOD의 사용자 태그는 `#태그` chip으로 표시하며 chip 간 gap은 6px로 유지한다.
- VOD CTA는 `Queue에 추가` 대신 `다운로드 추가` 용어를 사용하고 태그 영역과 버튼 사이에 16px 간격을 둔다.
- VOD 카드의 `재생시간/조회 → 태그 → 다운로드 버튼`은 footer 블록으로 묶고 `margin-top:auto`로 카드 하단에 정렬해 제목 길이가 달라도 하단 정보의 시작 위치를 맞춘다.
- VOD 제목은 14px, 재생시간/조회 메타는 11px로 한 단계씩 축소한다.
- 태그 chip 영역은 24px 한 줄 높이를 확보해 태그 유무에 따른 CTA 세로 위치 차이를 줄인다.
- 썸네일 URL이 없거나 이미지 로드에 실패하면 `썸네일이 없거나 불러오지 못했습니다` 안내를 동일한 16:9 영역에 표시한다.
- 검색 결과 목록과 `더 보기` 액션 사이에는 12px 간격을 유지해 마지막 카드와 버튼이 붙어 보이지 않도록 한다.
- 저장 채널 사용자 용어는 화면에서 `북마크 채널`로 통일한다.
- 북마크 목록에서는 텍스트 `삭제` 버튼 대신 채워진 별 아이콘을 표시하고, 별 클릭 시 북마크를 해제해 해당 항목을 목록에서 즉시 제거한다.
- 북마크 목록의 별 아이콘은 검색 결과용 공용 즐겨찾기 버튼 위치 규칙과 분리해 카드 우측 세로 중앙에 고정한다.
- Settings Drawer의 Light / Dark 선택은 값을 클릭하는 즉시 Ant Design theme과 앱 surface token에 미리보기로 적용한다. 저장 시 영속화하고 취소/X 닫기 시 기존 저장 테마로 복원한다.
- 기본 테마는 Dark이며 기존 DB의 v1 Settings는 migration v2에서 Dark로 승격한다.
- `app_settings.theme`을 SQLite에 저장해 재실행 후에도 테마를 유지한다.
- Settings Drawer 최하단의 `설정 적용`, `설정은 자동으로 보관됩니다.` 안내 블록은 제거하고 실제 설정 Form과 저장/취소 액션만 유지한다.
- 과거 빌드의 migration version 충돌로 `schema_migrations`에는 최신 버전이 기록됐지만 `app_settings.download_acceleration` 컬럼이 없는 DB를 시작 시 감지해 컬럼과 설정 schema_version을 자동 복구한다.
- 다운로드 탭은 헤더 아래 남은 높이를 채우는 단일 Workspace로 구성하고 `진행 중 / 완료 / 실패` 섹션을 내부 스크롤 영역에서 분리한다.
- 취소 요청이 승인되면 running Task도 즉시 cancelled 상태 이벤트를 발생시키고 UI 목록에서 제거한다. 실제 프로세스 종료 전까지 Scheduler active slot은 유지해 동시성 제한을 보존한다.
- Windows 실행 취소는 직접 프로세스만 종료하지 않고 `taskkill /T /F`로 다운로드/후처리 하위 프로세스 트리를 함께 종료한다.
- 다운로드 중간 파일은 최종 저장 경로와 분리해 `.chzzk-temp/{videoNo}`에 격리하고, `--continue`로 정상적인 부분 다운로드 재개를 허용한다.
- 실제 임시 데이터 충돌 실패는 `errorCode=partial_data_conflict` 복구 경로를 유지하며, 실패 항목의 `임시 파일 정리 후 재시도` 액션은 해당 VOD 임시 디렉터리만 삭제한 뒤 새 Queue 작업을 생성한다. 최종 영상 파일은 삭제하지 않는다.
- `initialization fragment found after media fragments`는 임시 파일 충돌이 아니라 `hls_initialization_fragment_order`로 분류한다. 최초 native HLS 다운로드가 이 오류로 실패하면 native 임시 파일을 삭제하지 않고 HLS discontinuity 분할 다운로드를 먼저 시도하고, 분할 처리까지 실패한 경우에만 ffmpeg external downloader를 최종 fallback으로 사용한다.
- discontinuity 분할 fallback은 원래 format selector로 선택되는 base format ID를 probe한 뒤, `--hls-split-discontinuity`가 실제로 노출하는 `-0`, `-1`, `-2` 등 구간 format ID를 다시 조회한다. 각 구간은 하나의 yt-dlp 실행에 모두 묶지 않고 exact format ID로 독립 다운로드해 특정 구간의 `The downloaded file is empty` 오류가 이미 성공한 구간까지 무효화하지 않도록 한다.
- split 구간 중 zero-byte/`The downloaded file is empty` 결과는 정상 미디어 구간의 앞/뒤 boundary에 있을 때만 제외한다. 정상 구간 사이의 index가 비어 있으면 영상 누락 위험이 있으므로 split fallback을 실패 처리하고 ffmpeg 최종 fallback으로 넘긴다. 정상 구간은 index 순서대로 ffmpeg concat demuxer로 합치고, 영상/오디오가 분리된 선택이면 각각 concat 후 최종 mux한다.
- split fallback의 UI 진행률은 전체 split candidate 수를 기준으로 구간별 percent를 합산하고, 각 구간의 개별 total/ETA를 전체 값처럼 노출하지 않는다.
- format probe의 `--print` JSON 템플릿은 pretty-print `#j`가 아닌 compact `j`를 사용해 각 probe 결과를 반드시 한 줄로 출력하고, line-based Runner에서 `json.Unmarshal`이 전체 payload를 받을 수 있도록 한다.
- native 다운로드는 `.chzzk-temp/{videoNo}/native`를 사용하고, 분할 fallback/ffmpeg fallback은 각각 새로운 `fallback-split-{runId}` / `fallback-{runId}` 경로를 사용해 Windows에서 native `.part` 파일이 잠겨 있어도 대체 다운로드 시작을 막지 않는다.
- ffmpeg fallback 명령에는 다운로드 가속 설정의 `--concurrent-fragments` 값을 유지하고, 번들 ffmpeg 디렉터리를 PATH 선두에 명시하며 `--downloader-args "ffmpeg:-nostdin"`을 적용한다. fallback에서는 `--paths`의 home과 temp를 모두 현재 `fallback-{runId}`로 지정해 외부 downloader가 어느 출력 단계에 쓰더라도 동일 경로에서 파일 증가를 관측한다.
- `--print`가 quiet를 암시하는 yt-dlp 동작을 고려해 ffmpeg fallback에만 `--verbose --no-quiet`를 추가한다. verbose 로그의 URL query/fragment뿐 아니라 CHZZK CDN의 `hdntl`/`hdnts`/`hmac` 서명 path segment도 저장 전에 `<redacted>`로 마스킹한다.
- Windows 시스템 코드페이지와 무관하게 yt-dlp가 출력하는 최종 파일 경로/probe JSON을 UTF-8로 읽을 수 있도록 모든 yt-dlp command에 `--encoding utf-8`과 `PYTHONIOENCODING=utf-8`을 적용한다. split 완료 경로 문자열로 파일을 찾지 못하면 `fallback-split-*` 디렉터리에서 해당 exact format ID를 포함한 실제 파일을 다시 탐색해 한글 파일명 mojibake 때문에 성공 다운로드를 폐기하지 않는다.
- fallback 진입 직후에는 `fallback_preparing` 상태를 유지한다. 일반 디버그 출력만으로 다운로드 중 상태로 전환하지 않고 `Invoking ffmpeg downloader` / ffmpeg command line 또는 실제 fallback 파일 증가가 확인될 때만 `fallback_downloading`으로 전환한다.
- fallback 준비 단계와 다운로드 단계를 별도 timeout으로 관리한다. ffmpeg downloader가 120초 내 시작되지 않으면 preparation timeout으로 중단하고, downloader 시작 후에는 fallback 파일 크기가 60초 동안 증가하지 않을 때만 stalled로 판단해 프로세스 트리를 취소한다.
- ffmpeg fallback의 파일 크기 baseline은 프로세스를 시작하기 전에 측정해, 프로세스 시작 직후 빠르게 기록된 바이트가 진행량에서 누락되지 않도록 한다.
- Runner는 `\n`뿐 아니라 ffmpeg가 진행 상태에 사용하는 `\r`도 라인 구분자로 처리한다.
- discontinuity 분할과 ffmpeg HLS fallback까지 모두 실패하면 사용자에게 대체 방식까지 실패했음을 표시하고, 실패 로그의 recent process output에 native → discontinuity split → ffmpeg 시도 순서와 각 실패 원인을 함께 남긴다.
- fallback 성공 파일은 검증된 `fallback-{runId}` 내부 경로에서 실제 다운로드 폴더로 이동한 뒤 완료 처리한다. 이동 시 동일 파일이 이미 있으면 덮어쓰지 않으며 Windows 파일 핸들 해제 지연을 고려해 짧게 재시도한다.
- 다운로드 실패 시 다운로드 폴더의 `.chzzk-logs`에 VOD별 진단 로그를 남긴다. 로그에는 UTC 시각, VOD URL, 출력 설정, 사용자 오류/원인 체인, 마지막 진행 상태, 민감 인자를 마스킹한 실행 인자, 최근 400줄의 stdout/stderr를 기록한다.
- 실패 Task의 `logPath`를 SQLite에 영속화하고 다운로드 탭에서 `로그 파일 열기`를 제공한다. 목록 삭제/임시 파일 정리 후 재시도 시에도 기존 로그 파일 자체는 보존한다.
- HTTP 401/Unauthorized 및 로그인 필요 신호는 `authentication_required`로 분류하고 사용자에게 `로그인이 필요한 콘텐츠입니다. 연령 제한 또는 접근 권한이 필요한 영상일 수 있습니다.`를 표시한다.
- 기존 `initialization fragment found after media fragments`의 `partial_data_conflict` 분류는 제거하고 HLS 구조 오류 자동 fallback으로 대체한다.
- cancelled 상태는 다운로드 이력 UI에서 표시하지 않으며 정상 취소 시 SQLite 이력을 삭제한다.
- 완료 Task에는 `폴더 열기`와 `목록에서 삭제`를 제공하고, 실패 Task에는 `목록에서 삭제`를 제공한다. 삭제는 파일이 아니라 앱의 다운로드 이력만 제거한다.
- 다운로드 도구 상태 UI에는 외부 프로그램명을 직접 노출하지 않고 `영상 다운로드`, `파일 저장/영상 처리`처럼 사용자가 이해할 기능 수준으로만 표현한다.
- B2C 데스크톱 서비스 기준으로 불필요한 장식보다 명확한 위계, 충분한 여백, 낮은 대비의 surface, 일관된 radius를 우선한다.

#### Phase 6-UX 검증 현황

완료:

- Go 1.23 설정 격리 fixture에서 `gofmt`, `go test`, `go test -race`, `go vet` 성공
- Theme 기본값 Dark / Light 저장 / 잘못된 Theme 거부 / StorageRecord v1→Dark 호환 복원 검증
- Python 실제 SQLite 엔진에서 migration v1 → v2 적용 및 기존 settings row의 `theme=dark`, `schema_version=2` 승격 확인
- 헤더 내부 Phase/Wails 표시 제거 및 헤더 navigation 구조 정적 확인
- 설정 버튼의 `shape="circle"` 제거 및 34×34px / `border-radius: 10px` rounded 사각형 스타일 정적 확인
- 설정 버튼 `.ant-btn-icon`의 flex 중앙 정렬, `line-height: 0`, SVG `display: block` 적용 확인
- `AntdApp message={{ top: 84 }}` 설정과 기존 `AntdApp.useApp()` Message 호출 경로 연결 정적 확인
- 채널 검색 좌측 검색/북마크 탭 + 우측 VOD 레이아웃 구조 확인
- 검색/북마크 탭 전환 시 검색 View가 unmount되지 않는 구조와 비활성 View의 `hidden` 처리, 100% 높이 유지 스타일 정적 확인
- 북마크 목록의 텍스트 삭제 버튼 제거, 채워진 별 아이콘 및 `onRemove` 해제 경로 연결 정적 확인
- 북마크 별 위치 selector 우선순위 확인: 공용 `top: 9px`보다 북마크 전용 `top: 50% + translateY(-50%)`가 우선 적용되도록 확인
- Scroll chain 정적 확인: `app-shell overflow:hidden → app-content min-height:0/overflow:hidden → workspace height:100%/min-height:0 → sidebar/VOD scroll min-height:0/overflow-y:auto`
- Settings Drawer Light/Dark Form 계약과 AppSettings theme 타입 연결 확인
- 테마 선택 즉시 preview 적용, 저장 시 유지, 취소/X 닫기 시 저장 테마 복원 경로 추가
- Settings Drawer 최하단의 설정 안내 Alert 2개 제거
- legacy SQLite의 누락된 `download_acceleration` 컬럼 자동 복구 및 기본값 `standard` 복원 테스트 추가
- Download Manager의 cancelled 숨김 / 완료 액션 / 기능 중심 tool 상태 문구 구조 정적 확인
- Queue 취소/삭제 격리 fixture에서 `gofmt`, `go test`, `go test -race`, `go vet` 성공
- running 취소 즉시 cancelled event 발생 및 executor 종료 전 Scheduler slot 유지 검증
- terminal Task만 Registry에서 삭제되고 running Task 삭제는 거부되는지 검증
- Runner 취소 격리 fixture에서 `go test`, `go test -race`, `go vet` 성공
- Runner가 context 취소 시 프로세스를 즉시 종료하는지 검증
- Download Manager TSX를 TypeScript 5.8.3 parser로 구문 검증
- VOD별 temp 경로가 `.chzzk-temp/{videoNo}`로 분리되고 최종 저장 경로와 별도 `--paths temp:` 인자로 전달되는 구조 정적 확인
- `partial_data_conflict` 오류 코드의 Queue → SQLite → Frontend 타입 연결 및 복구 API/버튼 경로 정적 확인
- 실패 진단 로그의 최근 프로세스 출력 캡처, 민감 실행 인자 마스킹, `logPath` Queue → SQLite → Frontend 연결 및 로그 파일 열기 경로 정적 확인
- HLS initialization fragment 오류 분류 → native temp 보존 → 원래 선택 format probe → `--hls-split-discontinuity` 구간 다운로드/누락 검증/concat → 실패 시 독립 `fallback-{runId}`의 `m3u8:ffmpeg` 최종 fallback → 3단계 진단 로그 병합 경로 정적 확인
- 최신 main의 다운로드 가속 `--concurrent-fragments` 정책과 HLS fallback command를 함께 유지하도록 충돌 병합 확인
- discontinuity split format probe의 compact JSON(`j`, pretty-print `#j` 미사용), 실제 split format ID 재조회, exact format별 독립 다운로드, leading/trailing empty boundary 허용 / 중간 empty 거부, 모든 yt-dlp command의 UTF-8 출력 강제, 한글 최종 경로 보존 및 mojibake 경로의 실제 split 파일 재탐색, split 성공 시 ffmpeg fallback 미실행, ffmpeg PATH 주입 / `--verbose --no-quiet`, staged home+temp 경로, 프로세스 시작 전 file-size baseline, downloader start marker 감지, 120초 preparation timeout / 60초 file-growth stall 분리, `\r` 출력 분리, query/fragment 및 CDN path 서명 마스킹, 성공 파일 promote 경로 테스트 추가
- Download Manager의 `대체 방식 재시도` 상태 Tag, 준비/연결/다운로드 중 문구 및 전체 크기를 알 수 없는 fallback metrics 표시 경로 정적 확인

현재 실행 환경 제약으로 검증 대기:

- Node.js 24 + 실제 Yarn 의존성 기반 `yarn typecheck`
- `yarn build`
- 실제 Wails 앱에서 Light/Dark 전환 및 재실행 복원
- 960×640 / 일반 데스크톱 창 크기에서 실제 레이아웃 시각 검증
- 실제 Windows에서 실행 중 다운로드 취소 시 `taskkill /T /F`로 하위 프로세스까지 종료되고 다음 Queue 작업이 정상 시작되는지 검증
- 완료 다운로드의 `폴더 열기`가 Windows Explorer에서 실제 경로를 여는지 검증

#### Phase 6-UX 원칙

- 기존 API, Queue, Persistence 동작을 변경하지 않고 화면 정보 구조와 상호작용을 우선 개선한다.
- 기능 추가가 필요한 UX 개선은 현재 항목에 임의로 포함하지 않고 별도 Task로 기록한다.
- 현재의 React 19 + Ant Design v5 + Tailwind v4 조합을 유지하고 새로운 UI 프레임워크를 추가하지 않는다.
- 기존 컴포넌트를 우선 재사용하며 요청과 관계없는 대규모 리팩터링은 하지 않는다.
- Desktop Windows 앱을 1차 기준으로 하되 최소 창 크기 960×640에서도 주요 액션이 가려지지 않도록 한다.
- 사용자에게 불필요한 Phase/개발 상태 문구는 제품 UI에서 제거하고 개발 진행 상태는 DEVELOPMENT_PLAN에서만 관리한다.

## Phase 7 — Windows 패키징

### Phase 7-A — Windows 배포 메타데이터

- [x] PKG-7A-1. 개인 취미 프로젝트 배포자명을 `MoonKim`으로 적용
- [x] PKG-7A-2. Windows 제품 메타데이터에 `companyName`, 저작권, 사용자용 설명 적용
- [ ] PKG-7A-3. Windows Portable EXE의 파일 속성 실제 검증 — 빌드 스크립트에 메타데이터 검사 구현, Windows 실 빌드 검증 대기

#### Phase 7-A 메타데이터 기준

- 법인/회사명을 의미하는 별도 브랜드를 사용하지 않고 개인 배포자명은 `MoonKim`으로 통일한다.
- Wails의 `companyName` 필드는 Windows 리소스 메타데이터 필드명이며, 값은 개인 배포자명 `MoonKim`을 사용한다.
- 제품명은 `CHZZK Video Downloader`를 유지한다.
- 현재 저장된 개발 버전은 `0.1.0`이며, Phase 7-C의 기본 패치/명시적 마이너·메이저/버전 유지 정책에 따라 빌드한다.
- 저작권 표기는 `Copyright © 2026 MoonKim`을 사용한다.
- 사용자용 제품 설명에서는 내부 구현 도구명을 직접 노출하지 않고 `CHZZK VOD downloader`로 표시한다.
- 실제 Portable EXE의 Windows 파일 속성 반영 여부는 Phase 7 패키징 검증에서 확인한다.

- [ ] PKG-1. yt-dlp/ffmpeg/ffprobe 배포 전략 적용 — Phase 3-D에서 executable bundle 기반 선행 구현, 최종 Portable EXE 실 실행 검증은 Phase 7에서 수행
- [ ] PKG-2. Windows 빌드 및 WebView2 배포 정책 적용 — Portable/embedded-bootstrapper 구현 완료, 실제 Windows 환경 검증 대기
- [ ] PKG-3. 최종 Windows 패키징 검증


### Phase 7-B — 단일 Portable EXE 패키징

- [x] PKG-7B-1. 기존 PR #17의 `MoonKim` 메타데이터와 버전 `0.1.0` 유지
- [x] PKG-7B-2. `build-windows.ps1`을 단일 Portable EXE 빌드로 변경하고 Installer 옵션 제거
- [x] PKG-7B-3. 도구 bundle 파일 존재/중복/플랫폼/SHA-256 검사 및 누락·손상 시 빌드 중단
- [x] PKG-7B-4. 버전별 EXE, SHA-256, release metadata, third-party 안내를 `build/portable/v{version}`에 생성하는 스크립트 구현·계약 검증
- [x] PKG-7B-5. 배포 파일 단일 amd64 PE EXE/체크섬/안내문 검증 스크립트 및 정상·변조·누락·옵션 거부 계약 테스트
- [x] PKG-7B-6. 동일 bundle의 managed 도구 재사용, 직접 갱신본 유지, 누락 파일 복원, 공백 경로 단위 검증
- [ ] PKG-7B-7. `-webview2 embed` 및 버전과 무관한 WebView2 사용자 데이터 경로 적용 — 코드 구현 완료, 실제 Windows 실행 검증 대기
- [x] PKG-7B-8. Portable 실행/빌드/검증/도구 갱신/사용자 데이터 정책을 README에 반영

#### Phase 7 배포 기준

- 공식 기본 배포물은 `CHZZK-Video-Downloader-v{version}-portable.exe`이며 앱 설치 프로그램과 ZIP을 생성하지 않는다.
- 현재 저장 버전은 `0.1.0`이며 기본 빌드 성공 시 `0.1.1`로 증가한다. 개인 배포자명 `MoonKim`과 저작권을 유지하고 코드 서명을 적용하지 않는다.
- 프로젝트는 Wails 기반이다. 기존 Go embed 및 LocalAppData 도구 관리 방식을 재사용하며 Electron portable/asar 구조로 전환하지 않는다.
- yt-dlp/ffmpeg/ffprobe는 EXE에 내장하며 런타임에는 `%LOCALAPPDATA%/CHZZK Video Downloader/tools`에서 실행한다.
- 동일 bundle에서 managed 도구는 재사용한다. 파일 누락 또는 bundle 변경 시 내장본을 추출한다. 새 bundle은 직접 갱신한 도구도 내장본으로 교체한다. 별도 도구 자동 업데이트 기능은 이번 범위에 추가하지 않는다.
- 앱 설정·북마크·다운로드 기록은 기존 `%APPDATA%/CHZZK Video Downloader/data.sqlite3`에 유지한다.
- WebView2 데이터 경로는 `%LOCALAPPDATA%/CHZZK Video Downloader/webview2`로 고정한다. 기존 WebView2 캐시는 이관하지 않는다.
- `-webview2 embed`로 공식 bootstrapper를 포함한다. Runtime이 없는 PC에는 별도 Runtime 설치/인터넷 연결이 필요하므로 완전 오프라인 실행을 보장하지 않는다.
- SHA-256 및 release metadata와 third-party 안내는 EXE와 함께 게시하되 실행 의존 파일로 사용하지 않는다.

#### Phase 7 검증 현황

완료:

- PowerShell 7.4.6에서 빌드/배포 검증 스크립트 구문 확인
- 실제 filesystem/hash + mock Wails/Windows VersionInfo를 사용한 패키징 계약 검증: 정상 생성, 변조, 잘못된 PE 아키텍처, 외부 EXE 혼입, 안내문/도구 누락, bundle 손상, Installer 옵션 거부, 비-Windows 빌드 거부
- 실제 Wails binding 생성 후 Yarn 4.9.2 `yarn build` 성공 (타입 검사 포함, 기존 큰 chunk 경고 있음)
- `go build ./internal/...`, bundle/Resolver `go test -race` 및 `go vet` 성공
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build` 및 Wails 2.15.0 `wails build -platform windows/amd64 -s -skipbindings -webview2 embed` 교차 빌드 성공. 실제 frontend를 포함하되 도구 bundle은 README placeholder 상태이므로 배포 가능한 다운로드 도구 내장본으로 간주하지 않는다.
- 교차 빌드 EXE의 Windows resource에서 제품명/버전/MoonKim/저작권 문자열 확인. 실제 Windows 파일 속성 검사는 별도로 대기한다.
- 기존 App 설정/SQLite persistence 관련 테스트만 선별 실행하여 통과
- Go 번들 및 Resolver 관련 테스트: 최초 추출, 동일 bundle 재사용, 변경 bundle 교체, checksum 오류, managed 도구 직접 갱신본 보존, 누락 복원, 공백 경로

Windows 실 검증 대기:

- 실제 `build-windows.ps1`에서 도구 준비/실행 및 Wails Portable EXE 생성, `MoonKim` 파일 속성 확인
- EXE만 별도 한글/공백·읽기 전용 경로로 이동 후 PATH 없이 다운로드/병합/취소/재시작 확인
- 새 EXE 교체 후 기존 SQLite 설정·북마크·다운로드 기록 유지 확인
- WebView2 설치/미설치/설치 취소 환경 확인, Windows 10/11 실행 및 SmartScreen 표시 확인

관련 검증 차단 사항:

- 기존 `go test ./internal/...`에서 `download_test.go`의 두 callback에 타입/이름이 혼용된 파라미터가 있고, `queue_test.go`에서 미정의 `immediateErrorExecutor`를 참조하여 downloader 전체 테스트가 컴파일되지 않는다. Portable 변경 이전 코드의 오류이며 관련 bundle/Resolver 테스트와 production compile은 별도로 수행한다. 전체 테스트 통과로 기록하지 않는다.
- `go test app.go app_download_test.go app_persistence_test.go app_settings_test.go`는 기존 3개 Queue 테스트가 기본 동시 다운로드 1을 기대하지만 현재 기본값은 3이어서 실패했다. 관련 설정/저장 테스트는 선별 통과했으며 이 요청에서는 다운로드 테스트를 임의 수정하지 않는다.


### Phase 7-C — Portable 빌드 버전 관리

- [x] PKG-7C-1. 기본 실행 시 patch +1, `-Minor`/`-Major` 선택 시 하위 버전 0 초기화
- [x] PKG-7C-2. `-NoVersionBump`로 동일 버전 재빌드 및 모든 옵션에 `-RefreshTools` 조합 지원
- [x] PKG-7C-3. 상충하는 버전 옵션, JSON 버전 불일치, Windows 버전 범위 오류를 빌드 전에 거부
- [x] PKG-7C-4. Wails/Frontend 버전·EXE 파일명·release metadata 동기화, 기존 JSON 필드/서식 유지
- [x] PKG-7C-5. 성공 시 증가된 버전 유지, 빌드 또는 배포 검증 실패 시 두 JSON을 원래 바이트로 복원
- [x] PKG-7C-6. PowerShell 계약 테스트와 README 사용 예시 업데이트

#### Phase 7-C 검증 및 사용 기준

- PowerShell 7.4.6 fixture에서 기본 patch, Minor + RefreshTools, Major, NoVersionBump, 두 JSON/EXE/metadata 동기화를 검증했다.
- NoVersionBump 실행 및 patch/minor/major 빌드 실패 시 두 JSON이 바이트 단위로 유지/복원되는 것을 검증했다.
- 컴파일 성공 후 배포 검증 실패 시에도 JSON을 복원하는 것을 검증했다.
- 상충하는 버전 옵션 조합과 Wails/Frontend 버전 불일치를 거부하는 것을 검증했다.
- Windows PowerShell의 기본 문자 인코딩에 의존하지 않고 UTF-8 JSON을 읽으며, 버전 변경 외 기존 필드·줄바꿈·BOM을 유지한다.
- Repository의 저장 버전 자체는 이번 스크립트 구현 요청에서 수동 증가시키지 않았다. 실제 Windows 빌드가 성공하면 버전 변경 파일도 이후 commit에 포함한다.
- 기존 Windows 실제 실행 검증 대기 및 전체 테스트 차단 사항은 그대로 유지한다. 이번 추가 수정은 PowerShell/문서 범위이며 기존 Go 테스트 오류를 임의 수정하지 않는다.

### Phase 7-D — Windows 메타데이터 읽기 호환성 수정

- [x] PKG-7D-1. 저장소에서 관리하는 `scripts/windows-info.json`에 문자열 `FileVersion` 및 파일·제품 숫자 버전 추가
- [x] PKG-7D-2. Portable 빌드마다 로컬 `build/windows/info.json`을 공통 템플릿으로 갱신해 기존 파일의 누락/오래된 값 방지
- [x] PKG-7D-3. 파일 버전까지 메타데이터 검사에 포함하고 불일치 시 EXE 경로·필드별 예상값/실제값/빈 값 표시
- [x] PKG-7D-4. 계약 테스트를 실제 입력 리소스 템플릿 기반으로 개선하고 최초 생성·오래된 로컬 파일 교체·메타데이터 실패 시 버전 복원 검증
- [x] PKG-7D-5. README에 템플릿 관리 및 오류 후 동일 버전 재빌드 절차 기록
- [ ] PKG-7D-6. Windows PowerShell 5.1에서 실제 Wails EXE의 `FileVersion`/제품명/배포자/제품 버전/저작권 조회 및 Portable 빌드 성공 확인

#### Phase 7-D 검증 및 관리 기준

- Windows PowerShell/.NET Framework는 문자열 `FileVersion`이 없으면 다른 언어 테이블을 조회하는 과정에서 제품 메타데이터를 빈 값으로 덮어쓸 수 있다. 기존 Wails 기본 템플릿의 `fixed.file_version`만으로는 이 문자열을 대신할 수 없다.
- 공통 템플릿은 모든 제품 메타데이터를 현재 `wails.json`의 `info`에서 읽도록 구성한다. `build/windows/info.json`은 빌드 시 덮어쓰는 로컬 산출물로 계속 Git에서 제외한다.
- PowerShell 7.4.6에서 Portable 계약 테스트 통과: 첫 빌드 시 템플릿 생성, 문자열/숫자 버전 동기화, 오래된 로컬 리소스 교체, 다섯 필드의 빈 값 및 잘못된 배포자 진단, 메타데이터 실패 시 두 버전 파일의 바이트 단위 복원, 버전 유지 재빌드.
- PowerShell 스크립트 구문 검사 통과. 별도 임시 fixture에서 문자열 `FileVersion`만 제거했을 때 메타데이터 검사로 실패하는 것을 확인해 기존 누락을 테스트가 감지하도록 검증했다.
- 계약 테스트는 Wails 템플릿 입력을 읽는 mock builder 및 mock VersionInfo를 사용한다. .NET Framework의 빈 메타데이터 동작은 모의하며 실제 Windows API 검증을 완료한 것으로 간주하지 않는다.
- 기본 patch/명시적 minor·major/버전 유지 정책을 유지하고 저장소 버전은 이번 오류 수정으로 증가시키지 않는다.

### Phase 7-E — Windows 백그라운드 콘솔 창 억제

- [x] PKG-7E-1. Windows 도구 프로세스에 `HideWindow` 및 `CREATE_NO_WINDOW` 공통 적용
- [x] PKG-7E-2. Runner의 다운로드/영상 처리, Resolver의 버전 확인, 취소용 `taskkill.exe`에 콘솔 숨김 적용
- [x] PKG-7E-3. 기존 stdout/stderr 진행률·오류 및 취소 처리를 유지하고 비-Windows에서는 기존 실행 방식 유지
- [x] PKG-7E-4. 실제 자식 프로세스의 콘솔 핸들을 확인하는 Windows 통합 테스트와 버전 조회/실패/취소 회귀 테스트 추가
- [ ] PKG-7E-5. 실제 Windows Portable EXE에서 시작·도구 상태 확인·다운로드·병합·취소 시 콘솔 창이 뜨지 않는지 확인

#### Phase 7-E 검증 기준

- Windows 전용 프로세스 설정은 명령 실행 전에 적용한다. stdout/stderr 파이프를 유지하며 사용자에게 필요한 진행률/오류를 계속 앱으로 전달한다.
- Windows 통합 테스트는 Runner 및 버전 조회가 실행한 자식의 `GetConsoleWindow()` 값을 확인한다. Linux에서의 교차 컴파일 성공을 Windows 실제 실행 성공으로 기록하지 않는다.
- Linux에서 downloader production 소스 전체와 Runner/Resolver/bundle 테스트를 선별해 `go test -race -count=1` 및 `go vet` 통과. Windows amd64 downloader production 빌드와 콘솔 핸들 통합 테스트 교차 컴파일 통과.
- 전체 downloader 테스트는 기존 `download_test.go` callback 타입 오류 및 `queue_test.go`의 미정의 `immediateErrorExecutor` 때문에 컴파일되지 않는다. 이번 수정과 관련된 Runner/Resolver/bundle 테스트를 별도로 검증하며 해당 기존 오류를 임의 변경하지 않는다.

### Phase 7-F — 앱 아이콘 교체

- [x] PKG-7F-1. 사용자 제공 녹색 `C` 아이콘을 Windows 배포용 원본 자산으로 추가
- [x] PKG-7F-2. Portable 빌드마다 원본 자산을 `build/appicon.png`으로 갱신하고 기존 `build/windows/icon.ico`를 제거해 Wails가 새 아이콘을 재생성하도록 적용
- [x] PKG-7F-3. 아이콘 자산의 정사각형 규격, 투명 모서리 및 빌드 스크립트의 갱신 경로 확인
- [ ] PKG-7F-4. 실제 Windows Portable EXE의 탐색기/작업표시줄 아이콘 표시 확인
- [x] PKG-7F-5. 개발 실행 전 `assets/appicon.png`을 `build/appicon.png`으로 갱신하고 기존 `build/windows/icon.ico`를 제거해 손상된 ICO 재사용 방지

#### Phase 7-F 검증 기준

- 저장소 원본은 `assets/appicon.png`으로 관리하고 `build/appicon.png`, `build/windows/icon.ico`는 기존과 같이 빌드 산출물로 유지한다.
- 아이콘 원본은 256×256 RGBA PNG이며 네 모서리가 투명한 상태임을 확인한다.
- `build-windows.ps1`은 Wails 실행 직전에 원본 PNG를 복사하고 기존 생성 ICO를 제거하므로 이전 로컬 아이콘이 남아 새 아이콘 생성을 막지 않는다.
- `scripts/dev.ps1`도 Wails 실행 전에 PNG 원본을 갱신하고 기존 `icon.ico`를 제거하므로, 이전에 생성되었거나 손상된 ICO의 `unexpected EOF`를 재사용하지 않는다.
- 실제 Windows Shell의 아이콘 캐시까지 포함한 최종 표시는 Windows Portable EXE 실 빌드 후 확인한다.

### Phase 7-G — 라이선스 및 제3자 고지 정리

- [x] PKG-7G-1. Repository 작성 소스에 MIT `LICENSE` 추가 및 저작권자 `MoonKim` 명시
- [x] PKG-7G-2. `THIRD_PARTY_NOTICES.md`에 yt-dlp, FFmpeg/ffprobe, WebView2 및 주요 런타임 의존성의 라이선스·출처·적용 범위 정리
- [x] PKG-7G-3. Portable 릴리스에 `LICENSE`와 `THIRD_PARTY_NOTICES.md`를 함께 복사하고 release metadata에 project license 필드 기록
- [x] PKG-7G-4. Portable 검증에서 프로젝트 라이선스·제3자 고지·project license metadata 누락을 실패 처리
- [x] PKG-7G-5. README에 프로젝트 라이선스와 제3자 라이선스 분리 원칙 및 배포 파일 구성을 문서화
- [ ] PKG-7G-6. Windows/PowerShell 환경에서 `scripts/test-portable.ps1` 계약 테스트 실제 실행

#### Phase 7-G 라이선스 기준

- Repository에서 직접 작성한 프로젝트 소스 코드는 MIT License를 적용하고 `Copyright (c) 2026 MoonKim`으로 표기한다.
- Repository의 MIT License는 제3자 구성요소를 재라이선스하지 않는다.
- 공식 Windows `yt-dlp.exe`는 upstream 안내에 따라 PyInstaller 번들에 GPLv3+ 코드가 포함된 배포물로 고지한다. yt-dlp 원본을 수정하지 않고 별도 프로세스로 실행한다.
- FFmpeg/ffprobe는 현재 빌드 스크립트가 선택하는 `ffmpeg-master-latest-win64-lgpl.zip` 기준으로 LGPL-2.1-or-later 구성요소로 고지한다. artifact/provider 변경 시 라이선스를 다시 검토한다.
- Wails `-webview2 embed`가 포함하는 Microsoft Edge WebView2 Evergreen Runtime bootstrapper는 Microsoft 배포 조건의 적용을 받으며 프로젝트 MIT 대상에 포함하지 않는다.
- Wails/React/Ant Design/modernc.org/sqlite 등 애플리케이션 의존성은 각 upstream 라이선스를 유지하고 정확한 버전은 `go.mod`/`go.sum`/`frontend/yarn.lock`을 기준으로 한다.
- Portable 릴리스에는 실행 파일과 별개로 `LICENSE`와 `THIRD_PARTY_NOTICES.md`를 함께 게시하는 것을 배포 기준으로 한다.

#### Phase 7-G 검증 현황

완료:

- 현재 빌드 스크립트가 공식 `yt-dlp.exe`와 BtbN `win64-lgpl` FFmpeg archive를 선택하는 경로를 확인했다.
- yt-dlp upstream의 현재 Licensing 안내에서 PyInstaller Windows 실행 파일이 GPLv3+ 결합 저작물로 배포됨을 확인했다.
- FFmpeg 공식 License 안내에서 기본 LGPL-2.1-or-later와 GPL 옵션 활성화 시 라이선스 변경 조건을 확인했고, 현재 artifact명이 LGPL build임을 대조했다.
- Microsoft 공식 WebView2 배포 문서에서 Evergreen bootstrapper를 애플리케이션과 함께 패키징할 수 있음을 확인했다.
- Wails v2.15.0은 MIT, modernc.org/sqlite는 BSD-3-Clause, React/Ant Design 계열 주요 런타임 패키지는 MIT임을 각 package metadata에서 확인했다.
- `build-windows.ps1`, `verify-portable.ps1`, `test-portable.ps1`의 license 파일/metadata 계약을 동일하게 갱신했다.

검증 대기:

- 현재 작업 환경에는 PowerShell이 없어 `scripts/test-portable.ps1`을 실제 실행하지 못했다. Windows/PowerShell 환경에서 계약 테스트를 실행한 뒤 PKG-7G-6을 완료 처리한다.

### Phase 7-H — Wails 개발 실행 embed 사전 준비

- [x] PKG-7H-1. `scripts/dev.ps1`에서 `frontend/dist`가 없거나 비어 있는지 Wails 실행 전에 확인
- [x] PKG-7H-2. 비어 있는 경우 `frontend`의 `yarn build`를 선행하고 실패 시 `wails dev`를 실행하지 않도록 처리
- [x] PKG-7H-3. 프론트엔드 빌드 후에도 embed 가능한 파일이 없으면 명시적 오류로 중단
- [x] PKG-7H-4. README의 개발 실행 절차에 자동 사전 빌드와 직접 `wails dev` 실행 조건을 반영
- [ ] PKG-7H-5. 실제 Windows의 빈 `frontend/dist` 상태에서 `scripts/dev.ps1` 실행 후 Wails 개발 앱 기동 확인

#### Phase 7-H 검증 기준

- `main.go`의 `//go:embed all:frontend/dist`는 유지하고 개발 wrapper가 컴파일 전에 필요한 프론트엔드 산출물을 보장한다.
- 기존 `frontend/dist`에 파일이 있으면 불필요한 프로덕션 프론트엔드 빌드를 반복하지 않는다.
- 프론트엔드 빌드 실패 또는 산출물 미생성 상태에서는 원래의 Go embed 오류까지 진행하지 않고 원인을 앞 단계에서 표시한다.
- 실제 Windows/Wails 실행 검증은 Windows 환경에서 완료한 뒤 PKG-7H-5를 완료 처리한다.
