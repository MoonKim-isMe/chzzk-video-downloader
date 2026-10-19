# DEVELOPMENT_PLAN — 현재 작업

기존 버전별 개발 계획과 완료 기록은 `docs/DEVELOPMENT_PLAN-0.1.md`, `docs/DEVELOPMENT_PLAN-0.2.md`, `docs/DEVELOPMENT_PLAN-0.3.md`에 유지합니다.

## Phase 4-C 보완 — 다운로드 탭 상태 안내

- [x] DL-UX-1. 다운로드 탭의 `대체 방식 재시도` 태그에 마우스 오버 안내 툴팁 추가
- [x] DL-UX-2. 대체 다운로드의 진행률·남은 시간 표시가 지연되거나 실제와 다를 수 있음을 안내
- [ ] DL-UX-3. Windows 앱에서 실제 대체 다운로드 중 툴팁 표시와 레이아웃 확인

### 검증

- 소스 정적 확인: 기존 `fallbackActive` 조건에만 툴팁이 표시되며 다운로드 상태와 동작 로직은 변경하지 않음
- 미실행: `yarn typecheck`, `yarn build`, Windows 실제 화면 확인 (실행 가능한 프로젝트 의존성 및 앱 환경 없음)
