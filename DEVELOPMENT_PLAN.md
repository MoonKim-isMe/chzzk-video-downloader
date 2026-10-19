# DEVELOPMENT_PLAN — 현재 작업

기존 버전별 개발 계획은 `docs/DEVELOPMENT_PLAN-0.1.md`, `docs/DEVELOPMENT_PLAN-0.2.md`, `docs/DEVELOPMENT_PLAN-0.3.md`에서 계속 관리합니다. 과거 작업 기록은 변경하지 않습니다.

## Phase 7-M — CHZZK sourceURL MPD 오류의 HLS 대체 다운로드

- [x] PKG-7M-1. 비로그인 HAR의 CHZZK 영상 API 및 NAVER MPD 구조 분석 (`15651133`)
- [x] PKG-7M-2. `KeyError('sourceURL')`를 별도 오류로 분류하고 해당 오류에서만 새 대체 경로 실행
- [x] PKG-7M-3. CHZZK `videoId`/`inKey` 재조회 후 MPD의 인증된 HLS 주소 선택 및 화질 제한 유지
- [x] PKG-7M-4. FFmpeg 직접 실행으로 영상·오디오 스트림을 받아 기존 임시 파일/최종 파일/진행률/취소/오류 로그 흐름에 연결
- [x] PKG-7M-5. MPD 선택·안전한 URL 필터·커맨드·오류 분류 Go 단위 테스트 추가
- [ ] PKG-7M-6. 실제 Windows Portable에서 영상 `15651133`의 전체 다운로드 및 취소/재시도 검증
- [x] PKG-7M-7. 실제 HAR의 36자리 `videoId`가 32자리 고정 검사를 통과하지 못하는 회귀 문제 수정 및 정상/비정상 ID 검증 테스트 추가

### 동작 기준 및 제한

- 정상 yt-dlp 다운로드와 기존 HLS 초기화 조각 오류 대체 다운로드는 변경하지 않습니다.
- `sourceURL` 오류에서만 별도로 CHZZK 재생 API를 조회하며, 서명된 HLS URL은 실행 시점마다 새로 획득합니다.
- 영상과 AAC 오디오가 함께 포함된 `video/mp2t` HLS Representation을 선택하고 기존 최대 해상도 설정을 적용합니다.
- FFmpeg는 스트림 복사로 MP4/MKV를 생성합니다. WebM은 H.264/AAC 스트림 복사와 호환되지 않으므로 실패를 명확히 안내합니다.
- FFmpeg 직접 다운로드에서는 yt-dlp의 동시 조각 가속 및 정확한 MB/s 다운로드 속도 제한 기능이 적용되지 않습니다.
- `inKey`, 서명된 URL, 쿠키 등 재생 권한 정보는 실패 로그나 사용자 오류 텍스트에 노출하지 않습니다.
- 현재 환경에서는 CHZZK CDN 직접 연결 및 Windows Portable 실행을 확인할 수 없으므로 실 다운로드 성공은 검증 전입니다.

### 후속 오류 분석 (2026-10-19)

- Windows 실패 로그 `source_url_missing`의 하위 원인 `영상 식별 정보가 올바르지 않습니다`는 실제 영상의 `videoId`가 36자리인데 32자리만 허용한 검증 오류로 확인했습니다.
- 하위 재생 API 호출 전에 중단되는 문제를 해결하기 위해 안전한 32~64자리 16진수 ID를 허용하고, 실제 HAR의 36자리 ID와 비정상 값 거부 테스트를 추가했습니다.
- 대체 재생 API의 실응답/FFmpeg 다운로드 확인은 아직 완료되지 않았으므로 PKG-7M-6은 미완료 상태입니다.

### Go 문법 오류 보정 (2026-10-19)

- [x] PKG-7M-8. 영상 ID 정규식 수정 반영 과정에서 중복 삽입된 `package`/`import` 선언을 제거하여 Go 문법 오류를 해결
- [x] PKG-7M-9. HLS 재생목록의 상대 `.ts` 세그먼트에 `_lsu_sa_` 서명 전달, 임시 재생목록 생성 및 FFmpeg 파일 입력 연동
- 컴파일 오류: `sourceurl_fallback.go:63`, `:209`, `:211` — 닫히지 않은 정규식 raw 문자열과 중복 소스 삽입에 따른 연쇄 오류
- 복구된 소스를 `gofmt`로 검사하고 격리된 Go 표준 라이브러리 테스트 패키지에서 관련 단위 테스트를 재실행
- 프로젝트 전체 Go 1.25 및 Windows Wails 빌드는 사용자 환경에서 별도 확인 필요

### HLS 첫 세그먼트 서명 누락 보정 (2026-10-19)

- 실제 Windows 로그: FFmpeg의 첫 세그먼트 주소에서 `_lsu_sa_` 쿼리가 사라져 `Error when loading first segment`가 발생했습니다. HAR의 성공한 브라우저 세그먼트 요청에는 같은 서명이 존재합니다.
- MPD로 선택한 원본 HLS 재생목록을 요청하고 상대 세그먼트 URL, 키/초기화 구간 및 중첩 재생목록의 인증 쿼리를 보존한 임시 로컬 m3u8 파일을 생성합니다.
- HTTPS `pstatic.net` 도메인만 허용하고, 중첩 깊이/파일 수/크기를 제한합니다. FFmpeg 명령에는 서명된 원격 URL 대신 로컬 manifest 경로만 노출하며 종료 시 manifest를 삭제합니다.
- 로컬 파일 입력에서는 HTTP 전용 FFmpeg `-user_agent`/`-headers` 옵션이 `Option not found`로 실패하므로 제거하고, `-protocol_whitelist`로 필요한 파일 및 HTTPS 프로토콜을 허용합니다.
- 격리 Go 환경에서 `go test -count=1 ./...`, `go vet ./...` 통과. 서명이 필요한 모의 HTTP 세그먼트를 로컬 HLS manifest로 불러와 FFmpeg가 MP4를 생성하는 테스트 통과.
- CHZZK CDN의 실제 응답, Windows Portable의 전체 VOD 다운로드 및 취소/재시도 검증은 **PKG-7M-6 미완료**로 유지합니다.

### Phase 7-N — 다운로드 중단 시 이력·진단 유지 (2026-10-19)

- [x] PKG-7N-1. 직접 취소와 앱 종료/비정상 실행 컨텍스트 취소를 구분하고, 후자를 `failed / interrupted` 상태로 기록
- [x] PKG-7N-2. 실행 중 비정상 취소에는 기존 오류 로그 형식의 진단 로그 작성 및 작업에 연결
- [x] PKG-7N-3. 앱 시작 시 중단된 SQLite `running/queued` 이력을 `cancelled` 대신 `failed / interrupted`로 복구해 목록에서 유지
- [x] PKG-7N-4. 명시적 사용자 취소는 기존과 같이 목록/이력에서 제거하도록 유지
- [ ] PKG-7N-5. Windows 실제 다운로드 도중 예기치 않은 종료/복귀 상황과 첫 중단 원인 검증

#### 관찰된 현상과 한계
- 영상이 일부 저장된 후 여러 항목이 일괄적으로 사라지고 로그가 없는 현상이 보고됐습니다.
- 기존 구현은 앱 종료와 상위 컨텍스트 취소를 일반 취소로 분류했고, 취소 이력을 SQLite에서 삭제하며, 재시작 시 남은 `running/queued` 상태도 숨겼습니다.
- 이번 변경은 비정상 중단을 이력과 로그에 남기는 진단·상태 보존 수정입니다. FFmpeg 중단의 직접 원인과 실제 앱 종료 여부는 추가 현장 검증이 필요합니다.
- 기존 버전에서 이미 삭제된 다운로드 이력은 복구하지 않습니다. 강제 프로세스 종료 직전에는 로그를 쓰지 못할 수 있으나, DB에 기록된 작업은 재시작 후 실패 이력으로 표시합니다.

### Phase 7-M — 중간 다운로드 정체 및 감시기 오류 분류 보완 (2026-10-19)

- [x] PKG-7M-10. 50.9%에서 발생한 내부 FFmpeg 감시기의 `context canceled`가 `interrupted`로 오분류되는 현상 수정
- [x] PKG-7M-11. sourceURL HLS 대체 다운로드만 180초 정체 감시 허용, 기존 일반 HLS 대체 다운로드의 60초 조건 유지
- [x] PKG-7M-12. FFmpeg HLS segment 일시 오류 재시도(`-seg_max_retry 3`), `total_size`를 다운로드 진행 바이트로 반영
- [x] PKG-7M-13. `hls_download_stalled` 오류 종류와 감시기 중단 사유/시간/최종 진행률 로그 추가, 중요 FFmpeg 출력 위주로 진단 보존
- [ ] PKG-7M-14. Windows 실제 VOD `15651133` 전체 다운로드 재검증 및 정체 원인(네트워크/서명/특정 세그먼트) 확인

실제 사용자 로그에서 약 32분 32초, 50.9%, 2.0 GB까지 FFmpeg 출력이 정상 증가한 뒤 자식 감시기의 취소가 발생했습니다. 이는 `runMonitoredHLSFallback`의 60초 정체 중단 경로와 일치하지만, 정체를 처음 유발한 네트워크/서명 만료 등 원인은 아직 확인되지 않았습니다. 이번 변경은 불필요하게 이른 취소를 줄이고 원인 진단을 정확히 하는 것이며, 모든 형태의 CDN 서명 만료를 자동 해결한다고 보장하지 않습니다.
