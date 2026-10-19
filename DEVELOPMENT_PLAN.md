# DEVELOPMENT_PLAN — 현재 작업

기존 버전별 개발 계획은 `docs/DEVELOPMENT_PLAN-0.1.md`, `docs/DEVELOPMENT_PLAN-0.2.md`, `docs/DEVELOPMENT_PLAN-0.3.md`에서 계속 관리합니다. 과거 작업 기록은 변경하지 않습니다.

## Phase 7-M — CHZZK sourceURL MPD 오류의 HLS 대체 다운로드

- [x] PKG-7M-1. 비로그인 HAR의 CHZZK 영상 API 및 NAVER MPD 구조 분석 (`15651133`)
- [x] PKG-7M-2. `KeyError('sourceURL')`를 별도 오류로 분류하고 해당 오류에서만 새 대체 경로 실행
- [x] PKG-7M-3. CHZZK `videoId`/`inKey` 재조회 후 MPD의 인증된 HLS 주소 선택 및 화질 제한 유지
- [x] PKG-7M-4. FFmpeg 직접 실행으로 영상·오디오 스트림을 받아 기존 임시 파일/최종 파일/진행률/취소/오류 로그 흐름에 연결
- [x] PKG-7M-5. MPD 선택·안전한 URL 필터·커맨드·오류 분류 Go 단위 테스트 추가
- [ ] PKG-7M-6. 실제 Windows Portable에서 영상 `15651133`의 전체 다운로드 및 취소/재시도 검증

### 동작 기준 및 제한

- 정상 yt-dlp 다운로드와 기존 HLS 초기화 조각 오류 대체 다운로드는 변경하지 않습니다.
- `sourceURL` 오류에서만 별도로 CHZZK 재생 API를 조회하며, 서명된 HLS URL은 실행 시점마다 새로 획득합니다.
- 영상과 AAC 오디오가 함께 포함된 `video/mp2t` HLS Representation을 선택하고 기존 최대 해상도 설정을 적용합니다.
- FFmpeg는 스트림 복사로 MP4/MKV를 생성합니다. WebM은 H.264/AAC 스트림 복사와 호환되지 않으므로 실패를 명확히 안내합니다.
- FFmpeg 직접 다운로드에서는 yt-dlp의 동시 조각 가속 및 정확한 MB/s 다운로드 속도 제한 기능이 적용되지 않습니다.
- `inKey`, 서명된 URL, 쿠키 등 재생 권한 정보는 실패 로그나 사용자 오류 텍스트에 노출하지 않습니다.
- 현재 환경에서는 CHZZK CDN 직접 연결 및 Windows Portable 실행을 확인할 수 없으므로 실 다운로드 성공은 검증 전입니다.
