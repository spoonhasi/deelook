# deelook

CNC의 HMI 화면을 웹 브라우저에 다시 그리고, FANUC 서보·주축 고속 샘플링(FOCAS2 `cnc_sdt*`)을 직접 해 보는 프로그램입니다.
[deemesh](https://github.com/spoonhasi/deemesh)를 쓰는 예제이기도 합니다. 소스: <https://github.com/spoonhasi/deelook> (MIT)

> **For agents (English)** — deelook serves two pages: `/hmi`, Fanuc CNC HMI screens redrawn from values read
> through deemesh-hub (read-only: nothing it serves writes to a machine), and `/sampling`, FANUC high-speed servo/spindle
> sampling with FOCAS2 `cnc_sdt*` over deelook's own connection, stored as one SQLite file per session. This manual is at
> `/` and `/help`: HTML for browsers, Markdown for any other `Accept` (and always at `/help.md`). Its first section is the live
> state of this deelook; the HTTP API is at the end. Machine addresses (IP, port) never appear in pages or API answers.

## 이 deelook의 지금 상태

_실행 중인 deelook의 `/help`에서는 여기에 허브 연결, FOCAS2 라이브러리, 기계, 수집, 디스크 상태가 나옵니다._

## 페이지

| 주소 | 내용 |
|---|---|
| `/`, `/help` | 이 도움말 (브라우저에는 HTML, 그 밖에는 Markdown. `/help.md`는 늘 Markdown) |
| `/hmi` | HMI 화면. 아래쪽 메뉴나 `/hmi?machine=2`, `/hmi?machine=bench-31i`로 기계를 고릅니다(없으면 설정의 `machine`, 기본은 번호가 가장 작은 FANUC 기계) |
| `/sampling` | 시계열 수집 |

## HMI 화면 (`/hmi`)

- deemesh-hub에서 값을 읽어 FANUC 0i-F 화면(NC Guide 기준 640×480)과 조작반을 그립니다. **읽기 전용**이라 기계에 값을 쓰거나
  키를 보내는 경로가 없습니다.
- 기계는 화면 아래 메뉴에서 고릅니다(주소의 `?machine=`도 따라 바뀜). 주소에 없으면 설정의 `machine`이고, 기본값 `""`(빈 값)이면
  허브 목록에서 번호가 가장 작은 FANUC 기계입니다.
- 화면은 웹의 MDI 키(POS, PROG, OFS/SET, MESSAGE, GRAPH, SYSTEM, PAGE↑↓)와 화면 아래 소프트키로 고릅니다. 고른 화면은 그
  브라우저에만 적용되고 기계 화면은 바뀌지 않습니다.
- 되는 화면: POS(ABSOLUTE, RELATIVE, ALL), PROG(PROGRAM), OFS/SET(OFFSET, WORK), MESSAGE(ALARM, MESSAGE), 상태줄
  (`--EMG--`, `-RESET-`, `ALM`). 알람이 뜨면 제어기처럼 ALARM 화면으로 넘어갑니다. SYSTEM, GRAPH, SETTING 등은 틀만 있습니다.
- 보는 브라우저가 몇 개든 (기계, 화면)마다 허브를 한 번만 읽고, 아무도 보지 않으면 5초 뒤 읽기를 멈춥니다.
- 조작반 램프: 설정의 `[lamps]`에 적은 기계는 PMC Y 신호(NC Guide 표준 조작반 래더)로, 나머지는 deemesh의 공통 상태값
  (운전 모드, 싱글 블록 등)으로 켭니다.

## 시계열 수집 (`/sampling`)

- **대상**: deemesh-hub의 `nc_focas2_fanuc` 기계 전부와, 기계 옆 "추가"에서 IP로 직접 추가한 기계("변경"에서 이름·주소 바꾸기, 지우기). 화면 위쪽 "목록"에서
  deemesh-hub와 직접 추가 중 하나를 고르면 그 목록의 기계만 나옵니다. 허브에 연결이 안 되면 마지막으로 받은 목록으로
  수집합니다(수집은 deelook이 CNC에 직접 연결). 두 목록 모두 데이터 폴더의 `deelook.db`에 저장됩니다(설정 파일에는 없음).
- **필요한 것**: FANUC의 FOCAS2 라이브러리. deelook에 들어 있지 않습니다. Windows는 `Fwlib64.dll`과 `fwlibe64.dll`,
  Linux는 32비트 `libfwlib32.so`를 실행 파일 옆에 두거나 설정 `fwlib`로 지정합니다. 없으면 HMI 화면만 됩니다.
  라이브러리가 없을 때는 시계열 수집 화면에서 **파일 올리기**로 넣을 수도 있습니다. deelook이 시험한 FANUC 판(SHA-256이 맞는
  파일)만 받고, 다 올리면 다시 시작하지 않아도 바로 불러옵니다(데이터 폴더의 `fwlib`에 저장). 다른 판은 직접 넣으면 됩니다.
- **방식**
  - 기본(`cnc_sdtsetchnl`…): 서보 데이터 0~46번·주축 0~21번, 종료할 때까지 또는 정한 점 수, 즉시 또는 블록 N이 실행될 때부터.
  - 확장(`cnc_sdtsetchnl2`…, Windows만): 위에 더해 PMC 신호, O·N 번호. 늘 즉시 시작하고, 블록 N이 실행되거나 PMC 신호가
    바뀐(0→1, 1→0) 순간에 CNC가 **책갈피**(FANUC 이름: 트리거)를 남깁니다. 책갈피는 수집을 시작·종료시키지 않습니다.
  - 현재값(`cnc_sdtread1shot`): 수집 없이 지금 값을 한 번 또는 1초마다.
- **쓰는 순서**: 기계 → 방식·주기·길이 → 채널(종류, 축, 데이터) → 수집 시작 → 수집 종료. 수집 중에는 최근 3초 차트와 읽기
  통계가 나오고, 끝난 세션은 목록에서 **보기**(전체 차트), **CSV**, **DB**(세션 파일 그대로 받기), **삭제**를 합니다.
- 화면의 **cnc_sdt\* 안내** 패널에 데이터 종류별 단위, 설정 규칙, 오류 코드가 정리되어 있습니다.
- **저장**: 수집 한 건 = SQLite 파일 하나. 넘겨줄 때는 그 파일만 주면 되고, 그 파일에는 CNC의 IP·포트가 없습니다.
- **디스크 보호**: 남은 공간이 `minfree`보다 적으면 수집을 시작하지 않고, 수집 중이면 멈춥니다. 저장이 실패하면 바로 멈춥니다.

## 설정 (`deelook.toml`)

실행 파일 옆의 `deelook.toml`입니다(`deelook -config /etc/deelook/deelook.toml`처럼 다른 파일을 줄 수 있음). 없으면 처음 실행할 때
지금 쓰이는 기본값으로 만들어집니다. 안의 상대 경로(`data`, `fwlib`)는 그 파일이 있는 폴더 기준입니다. 이름은 명령줄 옵션
(`deelook -h`)과 같고, 명령줄로 준 값이 우선합니다. 바꾼 뒤 deelook을 다시 시작해야 적용됩니다(`hub`는 화면에서 바꾸면 바로).

| 이름 | 기본값 | 뜻 |
|---|---|---|
| `hub` | `"http://127.0.0.1:8080"` | deemesh-hub 주소. `""`이면 deemesh 없이(HMI 화면 없음, 시계열 수집은 직접 추가한 기계로). 페이지 오른쪽 위에서 바꾸거나 지우면 바로 적용되고 이 줄도 바뀜 |
| `listen` | `":8090"` | 웹 서버 주소. `"127.0.0.1:8090"`이면 이 PC에서만 |
| `machine` | `""` | `/hmi` 주소에 기계가 없을 때 보여 줄 기계: 번호(`1`), 이름(`"bench-31i"`), `""`(번호가 가장 작은 FANUC 기계) |
| `interval`, `timeout` | `"1s"`, `"5s"` | HMI 화면을 허브에서 읽는 간격, 한 번 읽을 때 허브 쪽 제한 시간 |
| `data` | `"deelook-data"` | 데이터 폴더 (상대 경로는 실행 파일 옆) |
| `minfree` | `"1GB"` | 이보다 디스크 공간이 적으면 수집 중단 (`"0"`: 보지 않음) |
| `fwlib` | `""` | FOCAS2 라이브러리 파일 (`""`: 실행 파일 옆) |
| `open` | `"auto"` | 시작하면 브라우저 열기 (`"auto"`: 더블클릭으로 실행했을 때만) |
| `[lamps]` | (없음) | `1 = "ncguide"`: 그 기계의 조작반 램프를 PMC로 |

## 데이터 폴더 (`deelook-data`)

| 이름 | 내용 |
|---|---|
| `sessions\` | 수집 한 건마다 파일 하나, `20261014-234235_bench-31i_s9.db` (현지 시각, 기계 이름, 세션 번호). 넘겨줄 것 |
| `deelook.db` | deelook 내부용: 세션 목록, 세션 번호, 기계 목록과 IP, 수집 화면 설정(기계마다). 넘겨주지 않음. 지워도 세션 파일로 목록이 다시 만들어짐 |

세션 파일 안: `session`(설정과 결과 한 줄), `chunks`(채널별 샘플 묶음: 16비트 원시값, little endian), `events`(책갈피 = 트리거).
샘플 i는 첫 샘플부터 i×주기 뒤의 값입니다. 파일 안의 시각은 UTC입니다.

## HTTP API

모든 응답은 JSON입니다(스트림, CSV, DB 파일 제외). 상태를 바꾸는 요청(POST, DELETE 중 본문이 있는 것)은
`Content-Type: application/json`이어야 합니다. 기계의 IP·포트는 어느 응답에도 나오지 않습니다. 인증이 없으므로 deelook에
접속할 수 있는 사람은 누구나 수집을 시작·종료하고 세션을 지울 수 있습니다.

**HMI**

| 요청 | 내용 |
|---|---|
| `GET /api/machines` | 허브의 기계 목록(`machine_id`, `machine_name`, `protocol`, 그 기계를 그리는 화면 `screen`)과 설정의 `machine`(`default`) |
| `GET /api/screens/fanuc/stream?machine=1&view=pos` | 화면 값의 Server-Sent Events. `view`: `pos`, `prog`, `offset`(`&page=N`), `work`, `alarm`, `message` |

**시계열 수집**

| 요청 | 내용 |
|---|---|
| `GET /api/hub`, `PUT /api/hub` | deemesh-hub 주소와 연결 상태 / 바꾸기 `{"url"}` (`""`: deemesh 안 씀; `deelook.toml`에도 씀) |
| `GET /api/sampling/types` | 데이터 종류 표(서보, 주축, CNC 정보)와 PMC 주소 종류 |
| `GET /api/sampling/machines` | 수집할 수 있는 기계와 축 이름, 허브 연결 상태, FOCAS2 라이브러리 |
| `POST /api/sampling/machines` | 기계를 IP로 추가: `{"name", "host", "port"}` (CNC에 연결해 축 이름을 읽음) |
| `POST /api/sampling/library` | FOCAS2 라이브러리가 없을 때 파일 하나를 그대로(`Content-Type: application/octet-stream`). 시험한 FANUC 판만 받고, 다 모이면 바로 불러옴 |
| `PATCH /api/sampling/machines/{machine}` | 직접 추가한 기계 바꾸기: `{"name", "host", "port"}` (빈 값·0은 그대로, 새 주소면 연결해서 축 이름을 다시 읽음) |
| `DELETE /api/sampling/machines/{machine}` | 직접 추가한 기계 지우기 |
| `POST /api/sampling/machines/{machine}/names` | 축 이름을 CNC에서 다시 읽기 |
| `GET /api/sampling/status` | 수집한 적이 있는 기계의 상태(읽기 통계, 책갈피, FOCAS2 호출 기록) |
| `POST /api/sampling/{machine}/start` | 수집 시작. 본문은 아래 설정 |
| `POST /api/sampling/{machine}/stop` | 수집 종료 |
| `POST /api/sampling/{machine}/oneshot` | 현재값: `{"servoPeriodUs", "channels"}` |
| `GET /api/sampling/{machine}/live?window=3s&buckets=600` | 최근 값(구간마다 최솟값·최댓값) |
| `GET /api/sampling/sessions?machine=&since=&until=&status=&before=&limit=50` | 기록된 세션, 최근 것부터. 시각은 RFC 3339(시작한 때 기준), `status`는 쉼표로(`stopped,completed`), `before`는 그 번호 아래(다음 묶음), `limit` 최대 5000 |
| `GET /api/sampling/sessions/days?tz=540&…` | 같은 거르기로 날짜별 건수·크기, 최근 날부터(`tz`: UTC보다 몇 분 앞선 곳의 날짜) |
| `GET /api/sampling/sessions/{id}/info` | 세션 하나 |
| `GET /api/sampling/sessions/{id}/view?from=&to=&buckets=` | 세션 전체 또는 구간(µs)의 차트용 값 |
| `GET /api/sampling/sessions/{id}/csv` | 세션을 CSV로 |
| `GET /api/sampling/sessions/{id}/db` | 끝난 세션의 SQLite 파일 |
| `DELETE /api/sampling/sessions/{id}` | 끝난 세션(파일) 지우기 |
| `POST /api/sampling/sessions/delete` | 끝난 세션 여러 개 지우기: `{"ids": [...]}` 또는 `{"machine", "until"}`(그때보다 앞서 시작한 것, `until` 필수). `"dryRun": true`면 지우지 않고 몇 건·몇 바이트인지만 |
| `GET /api/sampling/sessions/zip?ids=1,2,3` | 끝난 세션 파일들을 zip 하나로 (2000건까지) |
| `GET /api/sampling/storage` | 남은 디스크 공간, 세션 파일 합계, `minfree` |
| `GET /api/ui/{key}`, `PUT /api/ui/{key}` | 수집 화면이 맞춰 둔 설정(JSON): `sampling.pick`(고른 목록·기계), `sampling.form.{machine}`(그 기계의 수집 설정). 없으면 `null`, `null`을 넣으면 지움 |

수집 설정(`start` 본문):

```json
{
  "mode": "basic",
  "servoPeriodUs": 1000,
  "points": 2000,
  "path": 1,
  "startSeq": 0,
  "trigger": {"seq": 0, "signal": "", "unit": 1, "up": false, "down": false},
  "channels": [
    {"kind": "servo", "axis": 1, "type": 3, "shift": 0, "label": "X TCMD"}
  ]
}
```

- `mode`: `basic` 또는 `extended`. `servoPeriodUs`: 1000 또는 250. `points`: 0이면 종료할 때까지, 8 이상이면 그만큼 모으고 끝.
- `startSeq`(basic): 0이면 즉시, N이면 블록 N이 실행될 때 시작. `trigger`(extended): 블록 번호 `seq`, PMC 비트 `signal`
  (예: `"Y0.4"`)의 상승 `up`·하강 `down`.
- `channels`(최대 8): `kind`는 `servo`, `spindle`(축 번호 1부터), extended에서는 `pmcbit`(`signals`: 비트 주소 16개까지),
  `pmcbyte`, `pmcword`(`signals`: 주소 하나), `cnc`(`axis`: 계통, `type`: 1 O 번호, 2 N 번호). `type`은 `/api/sampling/types`의 번호,
  `shift`는 오른쪽 시프트 0~15.

예:

```bash
curl -s http://localhost:8090/api/sampling/sessions?limit=5
curl -s -X POST -H 'Content-Type: application/json' -d '{"mode":"basic","servoPeriodUs":1000,"points":2000,"path":1,"channels":[{"kind":"servo","axis":1,"type":3,"shift":0,"label":"X TCMD"}]}' http://localhost:8090/api/sampling/1/start
curl -s -OJ http://localhost:8090/api/sampling/sessions/9/db
```
