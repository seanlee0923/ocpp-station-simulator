# ocpp-station-simulator

`github.com/seanlee0923/ocpp` 기반 웹 EV 충전소 시뮬레이터. 실제 충전기 없이 CSMS를 개발/테스트할 수 있도록, 브라우저에서 가상 충전소를 만들고 OCPP 1.6 / 2.0.1 / 2.1 메시지(Boot, Authorize, Start/Stop Transaction, MeterValues, StatusNotification)를 직접 트리거하고, CSMS가 보내는 RemoteStart/RemoteStop/Reset 같은 원격 명령에도 응답한다. 관리자가 발급한 계정으로만 로그인해서 쓸 수 있고, 모든 조작은 그 계정 이름으로 이력에 남는다.

## 실행 방법 1: 바이너리

별도 인프라 없이 그냥 빌드해서 실행하면 된다 (기본값: SQLite, `./data/ocpp-simulator.db`). `ADMIN_USER`/`ADMIN_PASS`를 지정하지 않으면 임시 관리자 계정과 비밀번호가 생성되어 로그에 한 번 출력된다.

```sh
./build.sh
./ocpp-station-simulator
# http://localhost:8080 접속, 로그에 출력된 admin 계정으로 로그인
```

## 실행 방법 2: Docker

```sh
docker build -t ocpp-station-simulator .
docker run -d -p 8080:8080 \
  -e ADMIN_USER=admin -e ADMIN_PASS=change-me \
  -v ocpp-data:/data \
  ocpp-station-simulator
```

또는 `docker compose`:

```sh
ADMIN_USER=admin ADMIN_PASS=change-me docker compose up -d --build
```

`docker-compose.yml`은 기본적으로 SQLite + 명명된 볼륨(`ocpp-data`)으로 동작한다. 사내 MySQL을 쓰려면 `DB_DRIVER=mysql DB_DSN=...` 환경변수를 같이 넘기면 된다 (아래 설정 표 참고).

## 설정

환경변수 또는 플래그로 조정한다 (바이너리/Docker 공통).

| 변수 | 기본값 | 설명 |
|---|---|---|
| `PORT` / `-port` | `8080` | HTTP 포트 |
| `DB_DRIVER` / `-db-driver` | `sqlite` | `sqlite` 또는 `mysql` |
| `DB_DSN` / `-db-dsn` | `./data/ocpp-simulator.db` (Docker는 `/data/...`) | sqlite 파일 경로, 또는 mysql DSN |
| `ADMIN_USER` / `ADMIN_PASS` | (없음) | 지정하면 기동마다 이 자격으로 관리자 계정을 갱신. 없으면 최초 기동 시 임시 관리자를 생성하고 비밀번호를 로그에 한 번 출력 |

MySQL 예시:

```sh
DB_DRIVER=mysql DB_DSN="user:pass@tcp(host:3306)/ocpp_simulator?parseTime=true" ./ocpp-station-simulator
```

## 사용자/권한

- 로그인 없이는 아무 API도 쓸 수 없다 (세션 쿠키 기반, 서버 재시작 시 전부 로그아웃됨).
- 관리자만 "사용자 관리" 화면에서 새 계정을 만들 수 있다. 자기가입은 없다.
- 충전소 생성/조작 이력(`StationEvent.Actor`)은 실제 로그인한 계정 이름으로 남는다 — 예전의 "이름만 입력" 방식과 달리 스푸핑 불가능.

## 개발 모드

```sh
# 터미널 1: 백엔드
cd backend && go run ./cmd/server

# 터미널 2: 프론트엔드 (vite dev server, :8080으로 API 프록시)
cd frontend && npm install && npm run dev
```

## 구조

- `backend/` — Go + Gin. REST/WebSocket API, station.Station 기반 OCPP 클라이언트 어댑터(v16/v201/v21), GORM(SQLite/MySQL) 영속화, 세션 기반 인증(`internal/auth`).
- `frontend/` — React + TypeScript (Vite). `build.sh`가 이 빌드 결과물을 `backend/internal/webui/dist`로 복사해 Go 바이너리에 임베드한다.
- `Dockerfile` / `docker-compose.yml` — 프론트 빌드 → Go 바이너리 빌드(CGO 없음) → 최종 alpine 런타임 이미지, 전부 멀티스테이지 한 번에.

세부 설계는 최초 구현 시점의 계획 문서를 참고: 단일 프로세스 구조, MeterValues는 실제 계량 없이 입력값을 그대로 전송, TLS 검증 스킵은 `wss://` 테스트 CSMS 전용 옵션, 세션은 서버 상태 없는 서명 쿠키(재시작 시 재로그인 필요).
