# DEVELOPMENT_PLAN — v0.3

## Version 0.3 — CHZZK/YouTube 통합

### v0.3 제품 목표

v0.3부터 제품 표시 이름은 **CHZZK/YouTube Video Downloader**를 사용한다.

기존 CHZZK 검색·VOD 조회·다운로드 기능은 유지하면서 YouTube를 두 번째 Media Provider로 추가한다. 검색 UX는 Provider별로 분리하고, URL 직접 입력과 Download Queue는 Provider 공통 흐름으로 통합한다.

v0.3 완료 기준:

- CHZZK 채널 검색 → VOD 목록 → 영상 다운로드가 v0.1과 동일하게 동작한다.
- YouTube 전용 검색 화면에서 영상을 검색하고 다운로드 Queue에 추가할 수 있다.
- 하나의 URL 직접 입력 화면에서 CHZZK와 YouTube 단일 영상 URL을 자동 판별하고 메타데이터를 조회할 수 있다.
- CHZZK/YouTube 영상 다운로드가 동일한 Download Queue, 진행 상태, 취소, 완료/실패 이력 흐름을 사용한다.
- YouTube 영상은 영상 다운로드 외에 MP3 다운로드를 선택할 수 있다.
- 기존 v0.1의 Settings, SQLite 데이터, 북마크 및 다운로드 이력을 손실 없이 유지한다.
- Windows amd64 Portable 배포에서 변경된 제품명과 v0.3 버전 정책을 검증한다.

v0.3 기본 범위에서 YouTube 재생목록 전체 다운로드, YouTube 채널 저장/구독 관리, CHZZK MP3 다운로드는 포함하지 않는다. YouTube URL은 단일 영상 중심으로 처리하고 playlist 파라미터가 포함되어도 기존 `--no-playlist` 정책을 유지한다.


### Phase 9 — Multi Provider 기반 및 제품명 전환

- [ ] V03-9-1. Provider 타입을 `chzzk | youtube`로 정의하고 공통 Media 식별 규칙 확정
- [ ] V03-9-2. CHZZK Video 모델과 YouTube 메타데이터를 수용할 공통 Media 모델 정의
- [ ] V03-9-3. DownloadTask에 Provider, mediaId, sourceURL, downloadType(video/audio) 필드 반영
- [ ] V03-9-4. 기존 CHZZK 검색/VOD/URL 흐름을 공통 Media → Queue 경로에 연결하되 기존 동작 유지
- [ ] V03-9-5. Queue 중복 판단 키를 Provider + mediaId + downloadType 기준으로 확장
- [ ] V03-9-6. SQLite download task schema를 Provider/downloadType에 대응하고 기존 row migration 구현
- [ ] V03-9-7. 앱 헤더·창 제목·Windows ProductName 등 표시 이름을 `CHZZK/YouTube Video Downloader`로 변경
- [ ] V03-9-8. 기존 `CHZZK Video Downloader` AppData/LocalAppData 데이터 경로의 호환 또는 마이그레이션 정책 구현
- [ ] V03-9-9. 기존 v0.1 데이터로 실행했을 때 설정·북마크·다운로드 이력이 그대로 복원되는지 검증

#### Phase 9 구현 원칙

- Provider 분기 로직을 UI와 Queue 곳곳에 직접 추가하지 않고 URL/검색 메타데이터 계층에서 공통 Media 모델로 정규화한다.
- 기존 CHZZK 비공식 API 계층은 유지하며 YouTube 추가를 이유로 CHZZK API 구현을 불필요하게 교체하지 않는다.
- 동일 YouTube 영상의 video와 MP3는 서로 다른 다운로드 작업으로 허용할 수 있도록 `downloadType`을 중복 키에 포함한다.
- 앱 표시 이름 변경 때문에 기존 사용자 데이터 경로가 새 폴더로 단절되지 않도록 데이터 호환을 기능 변경보다 먼저 처리한다.


### Phase 10 — CHZZK/YouTube 통합 URL 직접 입력

- [ ] V03-10-1. URL Provider 자동 판별기 구현
- [ ] V03-10-2. 기존 `https://chzzk.naver.com/video/{videoNo}` 파싱을 Provider 공통 URL 조회 흐름에 연결
- [ ] V03-10-3. YouTube `youtube.com/watch` 및 `youtu.be` 단일 영상 URL 파싱/정규화
- [ ] V03-10-4. YouTube Shorts URL의 단일 영상 정규화 지원
- [ ] V03-10-5. yt-dlp 기반 YouTube 단일 영상 메타데이터 조회 계층 구현
- [ ] V03-10-6. CHZZK/YouTube 조회 결과를 동일한 URL 결과 레이아웃으로 표시
- [ ] V03-10-7. Provider별 아이콘/라벨과 영상 메타데이터를 표시하되 다운로드 CTA 구조는 공통화
- [ ] V03-10-8. URL 결과에서 영상 다운로드를 기존 Download Queue에 추가
- [ ] V03-10-9. 지원하지 않는 도메인, 잘못된 URL, 삭제/비공개/접근 제한 영상 오류 메시지 정리
- [ ] V03-10-10. playlist가 포함된 YouTube URL에서도 단일 영상만 Queue에 추가되는지 검증

#### Phase 10 URL 기준

- URL 직접 입력 화면은 Provider를 사용자가 먼저 선택하지 않아도 된다.
- CHZZK는 기존 단일 VOD API를 사용한다.
- YouTube는 별도의 API Key를 사용자에게 요구하지 않는 경로를 우선한다.
- 메타데이터 조회 실패와 실제 다운로드 실패를 구분해 표시한다.
- URL 조회 결과를 Queue에 추가한 이후에는 검색/URL 진입 경로와 관계없이 동일한 작업 모델을 사용한다.


### Phase 11 — YouTube 검색

- [ ] V03-11-1. 상단 Navigation에 CHZZK 검색과 분리된 `YouTube 검색` 화면 추가
- [ ] V03-11-2. API Key 입력 없이 사용할 수 있는 yt-dlp 검색 경로를 우선 구현하고 검색 결과 모델 정의
- [ ] V03-11-3. 검색어 입력, 로딩, Empty, 오류 및 재시도 상태 구현
- [ ] V03-11-4. 썸네일, 제목, 채널, 재생시간, 게시 정보 중심의 YouTube 검색 결과 카드 구현
- [ ] V03-11-5. 검색 결과의 다음 결과 조회 또는 더 보기 정책 구현
- [ ] V03-11-6. YouTube 검색 결과에서 영상 다운로드 Queue 등록 연결
- [ ] V03-11-7. YouTube 검색 결과에서 MP3 다운로드 진입점 연결
- [ ] V03-11-8. 검색 요청 경합 시 오래된 응답이 현재 결과를 덮어쓰지 않도록 상태 처리
- [ ] V03-11-9. CHZZK 검색 상태와 YouTube 검색 상태가 서로 영향을 주지 않는지 검증

#### Phase 11 검색 기준

- CHZZK의 채널 검색/북마크 UX를 YouTube에 억지로 재사용하지 않는다.
- v0.3의 YouTube 검색은 **영상 검색**을 기본 단위로 한다.
- YouTube 채널 저장, 구독 목록, 재생목록 탐색은 v0.3 기본 범위에 포함하지 않는다.


### Phase 12 — YouTube MP3 다운로드

- [ ] V03-12-1. 다운로드 유형 `video | audio` 모델 및 Wails/Frontend 타입 연결
- [ ] V03-12-2. YouTube audio 다운로드 Command Builder 구현
- [ ] V03-12-3. yt-dlp audio extraction + ffmpeg MP3 변환 경로 구현
- [ ] V03-12-4. YouTube URL 결과와 검색 결과에 `MP3 다운로드` 액션 추가
- [ ] V03-12-5. MP3 작업의 진행률, 속도, ETA, 취소를 기존 Download Manager에 연결
- [ ] V03-12-6. MP3 완료 파일 경로와 확장자를 Queue/SQLite에 정상 저장
- [ ] V03-12-7. 영상 다운로드와 MP3 다운로드를 Download Manager에서 명확히 구분
- [ ] V03-12-8. 동일 YouTube 영상을 video와 audio로 각각 Queue에 등록할 수 있는지 검증
- [ ] V03-12-9. MP3 변환 실패, ffmpeg 실패, 원본 audio stream 부재 오류 처리
- [ ] V03-12-10. CHZZK 결과에는 MP3 다운로드 액션을 노출하지 않는지 검증

#### Phase 12 MP3 기준

- MP3 다운로드는 YouTube Provider 전용으로 시작한다.
- 기존 영상 해상도와 영상 컨테이너 설정은 MP3 작업에 적용하지 않는다.
- 초기 v0.3에서는 안정적인 기본 MP3 품질을 우선하고, 세부 bitrate 선택 UI는 별도 요구가 생길 때 확장한다.
- 취소/실패/재시도/목록 삭제 정책은 가능한 범위에서 기존 Download Queue 정책을 재사용한다.


### Phase 13 — 설정·Queue·Persistence 통합 안정화

- [ ] V03-13-1. 기존 해상도/출력 포맷 설정을 CHZZK/YouTube video 작업에 동일하게 적용
- [ ] V03-13-2. 동시 다운로드 수 기본값과 Scheduler 정책을 Provider와 무관하게 유지
- [ ] V03-13-3. 다운로드 가속 설정을 YouTube video/audio에서 지원 가능한 범위로 연결
- [ ] V03-13-4. Provider/downloadType을 포함한 다운로드 이력 저장·복원 검증
- [ ] V03-13-5. 앱 재시작 시 queued/running 복구 정책이 YouTube 작업에도 동일하게 적용되는지 확인
- [ ] V03-13-6. 완료/실패 항목의 폴더 열기·목록 삭제·로그 열기 동작을 Provider 공통으로 유지
- [ ] V03-13-7. Provider별 오류를 사용자 메시지와 진단 로그에 구분해 기록
- [ ] V03-13-8. 기존 CHZZK HLS fallback 로직이 YouTube 추가로 영향을 받지 않는지 회귀 검증


### Phase 14 — v0.3 브랜딩·배포 및 회귀 검증

- [ ] V03-14-1. `wails.json`, Frontend, Windows resource의 제품명을 `CHZZK/YouTube Video Downloader`로 동기화
- [ ] V03-14-2. Portable artifact 이름을 `CHZZK-YouTube-Video-Downloader-v{version}-portable.exe`로 변경
- [ ] V03-14-3. v0.1.x 기준 `-Minor` 빌드로 v0.3.0 버전 전환 검증
- [ ] V03-14-4. README/THIRD_PARTY_NOTICES/release metadata를 CHZZK + YouTube 지원 기준으로 갱신
- [ ] V03-14-5. CHZZK 검색/URL/다운로드 전체 회귀 테스트
- [ ] V03-14-6. YouTube 일반 URL, 단축 URL, Shorts URL 영상 다운로드 테스트
- [ ] V03-14-7. YouTube 검색 → video 다운로드 및 검색 → MP3 다운로드 통합 테스트
- [ ] V03-14-8. 삭제/비공개/로그인·연령 제한/지역 제한 등 접근 실패 케이스 확인
- [ ] V03-14-9. 동시 다운로드, 취소, 재시도, 완료/실패 이력, 앱 재실행 복원 검증
- [ ] V03-14-10. Windows 10/11 Portable EXE에서 PATH 없이 CHZZK/YouTube video 및 YouTube MP3 실 다운로드 검증
- [ ] V03-14-11. 기존 v0.1 사용자 데이터가 v0.3 실행 후 유실되지 않는지 최종 검증

#### v0.3 완료 조건

- CHZZK 기존 기능의 회귀 없이 YouTube 검색과 YouTube URL 다운로드가 제공된다.
- URL 직접 입력은 CHZZK/YouTube를 자동 판별하고 동일한 결과/Queue 흐름으로 동작한다.
- YouTube 영상은 video와 MP3 중 원하는 다운로드 유형을 선택할 수 있다.
- Download Manager와 SQLite는 Provider 및 downloadType을 구분하면서 기존 취소/재시도/이력 정책을 유지한다.
- 제품 표시 이름과 Windows 배포 메타데이터는 `CHZZK/YouTube Video Downloader`로 일치한다.
- Portable 파일명은 Windows 파일명 제약에 맞춰 `CHZZK-YouTube-Video-Downloader-v{version}-portable.exe`를 사용한다.
- 기존 v0.1 사용자 데이터 호환성과 Windows 실 다운로드 검증이 완료되어야 v0.3 계획 항목을 완료 처리한다.
