# deelook

CNC의 HMI 화면을 웹 브라우저에 다시 그리고, FANUC 서보·주축 고속 샘플링(FOCAS2 `cnc_sdt*`)을 직접 해 보는 프로그램입니다.
**[deemesh](https://github.com/spoonhasi/deemesh)를 쓰는 예제**이기도 합니다.

> **English** — deelook redraws Fanuc CNC HMI screens in a web browser from values read through
> [deemesh](https://github.com/spoonhasi/deemesh), a free unified CNC interface (the same address means the same thing on
> FANUC, SIEMENS SINUMERIK and Mitsubishi controls). It also shows how to build what deemesh does not offer: FANUC
> high-speed servo/spindle sampling with FOCAS2 `cnc_sdt*` — every option of those calls, with the pitfalls measured on real
> CNCs. MIT-licensed: copy what you need. The rest of this document is in Korean.

| 부분 | 보여 주는 것 | 어떻게 |
|---|---|---|
| **HMI 화면** (`/hmi`) | CNC 공통 인터페이스로 제어기 화면을 만드는 법 | 값은 전부 **deemesh-hub**에서 읽습니다(읽기 전용, 기계에 쓰는 경로 없음). deemesh는 FANUC·SIEMENS·Mitsubishi 등 여러 제어기를 같은 주소 체계로 다루는 공통 인터페이스입니다(무료). |
| **시계열 수집** (`/sampling`) | deemesh가 다루지 않는 FANUC 고속 샘플링을 **직접 만드는 법** | deelook이 FOCAS2 라이브러리를 직접 불러 CNC에 연결합니다. 서보·주축 데이터, PMC 신호, O·N 번호, 책갈피(트리거)까지 `cnc_sdt*`의 모든 기능을 화면에서 써 보고 SQLite에 저장합니다. |

**무엇을 쓰면 되나**

- 여러 메이커 CNC의 상태·좌표·프로그램·알람 같은 값을 한 가지 방식으로 읽고 쓰고 싶다 → **deemesh**
- FANUC 서보·주축 파형(토크, 속도, 위치 편차 …)을 ms 단위로 모으고 싶다 → 이 저장소의 코드를 보고 직접 만드세요
  (MIT라 그대로 가져다 써도 됩니다). 어디부터 보면 되는지는 아래 "[`cnc_sdt*`를 직접 만들 때](#cnc_sdt를-직접-만들-때)".

시계열 수집에는 FANUC의 FOCAS2 라이브러리(`cnc_sdt*`가 든 것)가 필요해서 deelook은 **Windows 64비트와 Linux 32비트**로
나옵니다(그 함수가 든 라이브러리가 있는 조합). 라이브러리는 FANUC의 것이라 deelook에 들어 있지 않습니다. **없어도 deelook은
시작하고 HMI 화면은 그대로 됩니다.** 시계열 수집 화면만 무엇을 어디에 넣으면 되는지 안내하고, 넣은 뒤 다시 시작하면 수집이 됩니다.

## 지금 되는 것 (Fanuc 0i-F, NC Guide 기준 640×480)

화면은 웹의 MDI 키(POS, PROG, OFS/SET, MESSAGE, GRAPH, SYSTEM, PAGE↑↓)와 화면 아래 소프트키로 고릅니다.
고른 화면은 그 브라우저에만 적용되고 기계 화면은 바뀌지 않습니다.

| 화면 | 상태 |
|---|---|
| POS: ABSOLUTE / RELATIVE / ALL | 됨 (이동 중인 축 `*` 표시 포함) |
| PROG: PROGRAM | 됨 (실행 블록 하늘색, 버퍼에 읽힌 블록 파란색, BC) |
| OFS/SET: OFFSET (PAGE로 넘김), WORK | 됨 |
| MESSAGE: ALARM, MESSAGE | 됨. 알람이 뜨면 제어기처럼 ALARM 화면으로 넘어감 |
| 분할 화면 왼쪽 ABSOLUTE / RELATIVE / ALL | 됨 |
| 상태줄 `--EMG--`(빨강, 깜박임), `-RESET-`, `ALM`(빨강) | 됨 (시뮬레이터에서 확인) |
| PROG의 FOLDER·NEXT·CHECK, OFS/SET의 SETTING, SYSTEM, GRAPH, 알람 HISTRY | 틀만 있음 ("NOT AVAILABLE IN DEELOOK") |

조작반:

- **Main Panel**: 램프 표시. `-lamps 1=ncguide`를 준 기계는 NC Guide 표준 조작반 래더의 PMC 출력(Y0~Y8)을 그대로 비추고, 아닌 기계는 deemesh의 공통 값(모드, 싱글 블록, 드라이 런, 블록 스킵, 머신 록, 사이클 시작/정지)으로 켭니다.
- **Sub Panel**: 비상정지(눌리면 `EMG`), 이송·주축 오버라이드 다이얼 위치.
- **MDI**: 화면을 고르는 키만 동작합니다.

deemesh로 읽을 수 없어 보여 주지 못하는 것: 키인 버퍼 입력, 제어기 확인 메시지(`START FROM MIDDLE OF PROG` 등), 알람 이력.

## 시계열 수집 (FOCAS2 샘플링)

deemesh-hub에 `nc_focas2_fanuc` 프로토콜로 등록된 기계는 모두 대상입니다(따로 지정하지 않음). 목적은 **`cnc_sdt*`로 할 수 있는 것을 전부 직접 써 보는 것**이라, 함수가 주는
선택지를 화면에 그대로 다 내놓았고, 설정마다 "이 설정이 부르는 FOCAS2 함수"와 인수를 보여 줍니다.

기계 목록과 접속 정보(IP·포트)는 허브의 `/machineList`에서 받지만, 수집 자체는 deelook이 CNC에 직접 FOCAS2로 연결하므로
**허브 없이도 됩니다.**

- 허브에서 받은 Fanuc 기계 목록(IP·포트, 축 이름)은 데이터 폴더의 `deelook.db`에 저장합니다. 허브에 연결이 안 되면 화면에
  알림을 띄우고 마지막으로 받은 목록으로 수집합니다. 허브 목록에서 빠진 기계는 다음에 목록을 받을 때 같이 빠집니다.
- 허브에 없는 기계는 기계 옆 **추가**에서 이름·IP·FOCAS2 포트로 직접 추가합니다(번호 `L1`, `L2`…, 이름을 비우면 그 번호).
  추가할 때 연결해서 축·주축 이름을 FOCAS2(`cnc_getpath`, `cnc_setpath`, `cnc_rdaxisname`, `cnc_rdspdlname`)로 읽으므로 기계가
  켜져 있어야 합니다. **변경**에서 이름을 바꾸고, 새 IP·포트로 옮기고(그 주소로 연결해 축 이름을 다시 읽음), 지웁니다. 지금
  IP·포트는 화면에 나오지 않습니다(바꿀 때만 새로 넣음). "축 이름을 CNC에서 읽기"는 허브 기계의 변경 창에도 있습니다(허브가
  꺼져 있고 저장된 이름이 없을 때 등). 이름은 세션 파일 이름에 들어가므로 IP를 넣지 않는 것이 좋습니다.
- 축 이름을 모르면 축은 번호(축1~32, S1~8)로 고릅니다.
- deelook을 디메시와 다른 PC에서 돌리면 deelook PC에서 CNC의 FOCAS2 포트로 직접 접속이 되어야 합니다.

| 방식 | 함수 | 할 수 있는 것 |
|---|---|---|
| 기본 | `cnc_sdtsetchnl` `cnc_sdtstartsmpl` `cnc_sdtreadsmpl` `cnc_sdtendsmpl` `cnc_sdtcancelsmpl` | 서보(0~46번)·주축(0~21번) 데이터. **종료할 때까지**(datanum -1) 또는 **점 수 지정**(다 모으면 CNC가 끝냄 → `cnc_sdtendsmpl`). **즉시** 또는 **블록 N이 실행될 때부터** 시작(그때까지 "시작 블록 대기") |
| 확장 | `cnc_sdtsetchnl2` `cnc_sdtstartsmpl2` `cnc_sdtreadsmpl2` `cnc_sdtendsmpl2` | 위에 더해 **PMC 신호**(비트 최대 16개를 한 채널에, 30i-B 이후는 바이트·워드)와 **CNC 정보**(O·N 번호, 계통별)를 같은 시간축으로. 늘 즉시 시작하고, 블록 N 실행이나 PMC 비트의 0→1·1→0 순간에 CNC가 그 자리에 **책갈피**(FANUC 이름: 트리거)를 남김(수집을 시작·종료시키지 않음). 종료할 때까지 또는 점 수 지정 |
| 현재값 | `cnc_sdtsetchnl` `cnc_sdtread1shot` | 수집 없이 서보·주축 데이터의 지금 값을 한 번(또는 1초마다) 읽기 |

- 주기: 서보 1 ms 또는 0.25 ms, 주축 1 ms, CNC 정보는 4 ms마다 갱신(1 ms 채널이면 같은 값 4번).
  0.25 ms 서보와 주축을 같이 모으면 "종료할 때까지"만 됩니다(CNC 제약).
- 값은 CNC가 주는 16비트 원시값입니다(물리 단위로 바꾸지 않음). 데이터별 환산은 화면의 단위 설명 참고.
  서보·주축은 부호 있는 값, PMC·CNC 정보는 부호 없는 값으로 다룹니다.
- 화면: 수집 중에는 최근 3초를 채널마다 차트로(PMC 비트는 신호별 파형), 책갈피는 점선과 목록으로. 상태에는 읽기 호출 수,
  한 번에 오는 점 수, 호출 시간, 읽는 속도 대 CNC 샘플 속도, 밀릴 때 경고, FOCAS2 호출 기록(결과 코드 포함)이 나옵니다.
  기록된 세션은 **보기**로 전체를 차트로 보고(확대·이동, 책갈피로 이동) **CSV**로 받습니다.
- **cnc_sdt\* 안내** 패널: 레퍼런스를 찾아보지 않아도 되도록 서보 0~46·주축 0~21·CNC 정보의 모든 데이터 종류를
  한국어 이름, 조건별 단위(시프트 0일 때), 권장 시프트, 비고와 함께 표로 보여 주고(찾기, 표에서 바로 채널 추가),
  PMC 주소 종류·경로, 주기·시프트·점 수 규칙, 오류 코드와 상세 번호, 지원 기종, deelook 실측 내용을 정리해 둡니다.
  채널마다 고른 데이터의 단위 전부가 그 밑에 나오고, FOCAS2 오류 메시지에는 한국어 설명이 붙습니다.
- CSV: `time_ms`(첫 샘플부터), 채널마다 한 열(PMC 비트는 신호마다 0/1 한 열), 책갈피가 있으면 `trigger` 열(그 시점의 비트).
- 저장: **수집 한 건 = SQLite 파일 하나**, 데이터 폴더(`data`, 기본 `deelook-data`)의 `sessions\`에
  `20261014-163415_bench-31i_s12.db`처럼(시작 시각, 기계 이름, 세션 번호) 생깁니다. 파일 안에는 그 세션의 설정, 샘플(채널별 묶음),
  트리거만 있고 CNC의 IP·포트는 없습니다(기계 목록은 데이터 폴더의 `deelook.db`에 따로).
  - **`deelook.db`**(데이터 폴더): deelook 내부용입니다. 세션 목록(각 세션 파일 정보의 사본), 지금까지 쓴 세션 번호,
    기계 목록(IP), 수집 화면이 맞춰 둔 설정(고른 목록·기계, 기계마다의 방식·주기·길이·책갈피·채널)이 들어 있고 넘겨주지
    않습니다. 화면의 설정은 바꿀 때마다 저장되어 deelook을 다시 시작하거나 HMI 화면에 갔다 와도, 다른 PC의 브라우저에서
    열어도 그대로 나옵니다. 세션 파일이 원본이라, 탐색기에서 세션 파일을 지우거나 넣으면 목록이
    따라가고(번호가 이미 쓰이는 파일은 목록에서 빠지고 로그에 남음), `deelook.db`를 지워도 세션 파일로 목록이 다시 만들어집니다.
    (기계 목록과 직접 추가한 기계는 그때 다시 받거나 추가해야 합니다.)
  - 이름의 시각은 이 PC의 현지 시각, 파일 안의 시각(시작·끝·받은 시각)은 UTC(`2026-10-14T08:06:01.6065281Z`)입니다.
  - 세션 번호는 수집을 시작할 때마다 기계와 상관없이 1씩 늘고(화면의 #과 같음), 지운 세션의 번호도 다시 쓰지 않습니다.
  - **넘겨주기**: 화면의 세션 목록에서 **DB**를 누르면 그 파일을 그대로 받습니다. 끝난 세션의 파일은 혼자서 완전한
    파일(옆에 `-wal` 없음)이라 복사만 하면 됩니다. 수집 중인 세션은 끝난 뒤에 받습니다.
  - **지우기**: 목록의 **삭제**가 그 파일을 지웁니다. 공간이 바로 생깁니다.
  - **디스크 보호**: 데이터 폴더가 있는 디스크의 남은 공간이 `minfree`(기본 1GB)보다 적으면 새 수집을 시작하지 않고, 수집
    중이면 5초 안에 멈춰 "디스크 부족"으로 남깁니다. 저장(쓰기)이 실패하면 그 즉시 수집을 멈춥니다(데이터를 버리며 계속 읽지 않음).
    세션 목록 위에 남은 공간과 세션 파일 합계가 나옵니다.
  - 예전 판의 단일 DB(`deelook-data/sampling.db`)는 처음 실행할 때 세션별 파일로 나누어 옮기고(세션 번호 유지), 샘플과
    트리거 수가 맞는지 확인한 뒤 지웁니다. 그 안의 기계 목록과 예전 `machines.json`은 `deelook.db`로 옮깁니다.
  - 크기: 실측으로 샘플 하나에 약 2.5바이트(원시값 2바이트 + 저장 구조)라, 1 ms·8채널이면 시간당 약 70 MB(하루 약 1.7 GB),
    0.25 ms 서보 8채널이면 시간당 약 290 MB(하루 약 7 GB)입니다. FAT32 USB에서는 파일 하나가 4GB를 넘을 수 없습니다.
  - CSV는 DB보다 몇 배 크고, 엑셀은 104만 행까지라 1 ms 수집은 약 17분 분량까지만 엑셀로 열립니다.
- CNC 한 대에서 샘플링은 하나만 돌 수 있습니다. Servo Guide를 쓰는 중이면 CNC가 거부합니다.
  deelook을 끄면 진행 중인 샘플링을 CNC에서 멈추고 나갑니다.
- 시작·종료·현재값 API는 `Content-Type: application/json` 요청만 받습니다(다른 사이트에서 폼으로 누르는 것을 막음).
  deelook에 접속할 수 있는 사람은 누구나 허브의 어느 Fanuc 기계에서든 수집을 시작·종료하고, 기계를 추가·삭제할 수 있습니다.

### 실측으로 알게 된 것

2026-10-14, 31i 벤치(30i-B)·NC Guide 0i-F, Fwlib64 6.9/7.3으로 확인했습니다.

레퍼런스에 없는데 deelook이 맞춰 둔 것들입니다.

- 확장 방식도 레퍼런스대로 datanum -1(종료할 때까지)이 됩니다. 예전에 이걸로 `EW_BUFFER`가 났던 것은 아래 PMC 채널 순서
  문제로 한 번에 몇 점씩만 읽혔기 때문이었습니다. 확장 수집을 일찍 끝낼 때도 `cnc_sdtendsmpl2`로 끝납니다.
- **PMC 비트 채널은 맨 뒤 채널이어야 합니다.** 다른 채널 앞에 두면 뒤 채널 데이터가 8워드 묶음마다 2워드씩 밀려
  섞여 돌아오고, 그 값이 `cnc_sdtreadsmpl2`의 트리거 비트로까지 새어 나옵니다(꾸준히 1인 신호가 "SEQ"로 읽힘).
  30i-B와 NC Guide, Fwlib64 6.9와 7.3 모두 같았습니다. deelook은 화면의 채널 순서와 상관없이 PMC 비트 채널을 CNC에는
  마지막으로 보내고 데이터를 원래 순서로 되돌립니다.
- 트리거 비트: SEQ `0x0001`, 상승 `0x0002`, 하강 `0x0004`, 끝(STSM) `0x8000`. 끝난 뒤의 읽기는 `0x8080`을 줍니다.
  트리거가 있는 읽기는 그 순간에서 끊겨 돌아옵니다. 시작할 때 이미 1인 신호는 상승으로 표시되지 않습니다(2026-10-21, 늘 1인
  신호로 31i 벤치·NC Guide에서 각 10번 해 0번. 0.5초마다 바뀌는 PMC 시스템 릴레이 `R9091.6`은 모든 변화가 표시됨). 예전에
  시작 직후 상승을 본 것은 PMC 채널 순서 문제(위)로 데이터가 트리거 비트로 새던 때로 보입니다.
  블록 트리거는 CNC 정보 채널의 N 번호가 바뀌기 몇 ms 전에 찍힙니다(N 번호는 4 ms마다 갱신).
- 점 수를 정한 확장 수집이 끝나면 **PMC·CNC 정보 채널의 마지막 몇 점이 0**으로 옵니다(PMC 4점, N 번호 8점을 봄). 그 0으로는 하강이 표시되지 않습니다.
  deelook은 CNC에 정한 점 수보다 16점 더 요청하고(`datanum`) 끝의 16점과 그 안의 트리거를 버립니다. 늘 1인 PMC 신호로
  31i 벤치·NC Guide에서 각 5번 해 보니 저장된 2000점이 끝까지 1이었습니다(바꾸기 전에는 끝이 0). 프로그램이 도는
  NC Guide에서 O·N 번호 채널도 바꾸기 전에는 3번 모두 마지막 8점이 0이었고, 바꾼 뒤에는 끝까지 그 번호였습니다(2026-10-22).
- **CNC 정보 채널(O·N 번호)은 수집을 시작한 뒤 다음 블록이 시작될 때까지 0**으로 옵니다. 그 전부터 실행 중이던 블록의
  번호는 나오지 않습니다. 2026-10-22 NC Guide에서 1초짜리 `G04` 블록에 N 번호를 붙인 프로그램을 돌리며 6번 수집했더니,
  0이 144~712 ms 이어졌고 0이 끝난 자리는 늘 블록이 바뀐 순간이었습니다(그다음 N은 정확히 1000 ms 뒤에 바뀜). 블록이
  길면(긴 절삭, 휴지) 그만큼 길고, 프로그램이 돌지 않으면 계속 0입니다. PMC 채널은 처음부터 값이 옵니다. 0은 "프로그램 없음"과
  구별되지 않으므로 deelook은 받은 그대로 저장합니다.
- `cnc_sdtreadsmpl2`가 실패한 뒤에는 `cnc_sdtendsmpl2`를 불러야 합니다. 안 부르면 CNC 샘플러가 멈춰 NC Guide는
  새 연결까지 거부했습니다. deelook은 실패·종료 때마다 부르고, 설정 전에도 남은 세션을 정리합니다
  (정리할 것이 없으면 `EW_REJECT`가 정상).
- 한 번의 읽기로 오는 양은 모든 채널 합쳐 약 650~690워드(약 1.3 KB)입니다. 그래서 채널이 많거나 0.25 ms이거나
  네트워크가 느리면 CNC보다 늦게 읽게 됩니다. 31i 벤치(인터넷 너머): 1 ms 3채널은 초당 약 1000점으로 따라가지만,
  0.25 ms 서보 2 + 주축 1은 초당 약 3000~3600점으로 4000에 못 미칩니다. 기본 방식은 그러다 `EW_BUFFER`로 멈추고,
  점 수를 정한 방식은 CNC가 다 모은 순간 끝나서 못 읽은 점이 사라집니다. 화면에 경고가 뜹니다.
- 이 31i 벤치는 PMC 바이트·워드 채널에 `EW_VERSION`을 줍니다(그 기능이 없는 소프트웨어). NC Guide 0i-F는 됩니다
  (2026-10-22): `R9091` 바이트는 6·38·70·102(깜박이는 5·6번 비트), `R9090` 워드는 그 바이트를 위 8비트에 담아
  (little endian: 낮은 주소가 아래 바이트) 왔습니다. 10번 중 첫 번째 한 번만 바이트·워드 채널의 처음 16점이 0이었고,
  같은 설정으로 다시 9번 해서는 나오지 않았습니다.
- NC Guide 0i-F는 서보 0~46번, 주축 0~21번 **모든 데이터 종류를 받고**(`EW_DATA` 없음) 값은 모두 0입니다(2026-10-22,
  종류마다 `cnc_sdtread1shot` 3번). 그래서 어떤 종류가 되는지는 NC Guide로는 알 수 없고 실제 CNC에서 봐야 합니다.
- 31i 벤치(30i-B)도 서보 0~46번(X1)·주축 0~21번(S1) **모든 종류를 받았습니다.** 현재값 3번과, 종류마다 100점짜리 기본 수집
  한 번 모두 `EW_DATA` 없이 됐습니다(2026-10-22, 비상정지 중). 그 기계에 없는 장치의 종류(PDM 슬레이브 전류, 가속도 센서,
  아날로그 신호, 스마트 백래시 등)는 거부되지 않고 0으로 옵니다. 그래서 "된다"는 것은 값이 의미 있게 나오는지로 봐야 합니다.
  비상정지 중에도 값이 나온 것: 서보 위치 피드백 누적(0)·내장 펄스코더 절대 위치(30), R·S상 전류(13, 18, ±8 정도 노이즈),
  무효·유효 전류(23, 24, -13·-39로 일정), 모터 권선·펄스코더 온도(25, 26: 23·28 °C), 속도 피드백(4, ±1). 주축 위치 누적(0),
  속도·주축 속도(1, 18: ±410 = ±0.1 min⁻¹ 노이즈), 부하 토크(6), 전류 진폭(7), 모터 온도(8: 24 °C), 센서 A/D(13~16).
  위치 편차·토크 지령·DC 링크 전압 등은 서보가 꺼져 있어 0이었습니다.
- NC Guide는 실제 서보가 없어 서보 값이 0이고, 초당 샘플 수가 1000이 아니라 약 1300입니다(시뮬레이터 시간).
  PMC 신호, N 번호, 트리거, 블록 N 시작은 NC Guide에서 MDI 프로그램을 돌려 확인했습니다.

FOCAS2 라이브러리는 시작할 때 불러옵니다 (`-fwlib`, 없으면 실행 파일 옆).

| OS | 필요한 파일 (`-fwlib`로 지정, 기본은 실행 파일 옆) | 상태 |
|---|---|---|
| Windows 64비트 | `Fwlib64.dll` 7.3과 `fwlibe64.dll`을 한 폴더에 (이더넷 연결은 이 두 파일이면 됨) | 기본·확장·현재값 모두, 31i 벤치·NC Guide에서 확인 |
| Linux 32비트 | `libfwlib32.so.1.0.2`(x86) / ARM은 `1.0.1` | 기본·현재값 확인. **확장(`cnc_sdt*2`)은 이 라이브러리에 없어** 화면에서 꺼짐 |

라이브러리는 FANUC의 것이라 **저장소에 없습니다.** 구해서 [`fwlib/`](fwlib/README.md)의 플랫폼별 폴더에 넣으면(파일 이름·시험한 판·해시는 그 문서)
`build.sh`가 실행 파일 옆에 복사합니다.

- Windows는 `Fwlib64.dll` 7.3.0.1 + `fwlibe64.dll` 5.3.0.1(포트까지 연결을 가르는 판)로 시험했습니다. 이더넷 연결에는 이 두 파일이면
  되고, 기종별 DLL(`fwlib30i64.dll` 등)은 없어도 NC Guide 0i-F와 30i-B에서 기본·확장·현재값이 모두 됐습니다.
  `fwlibe64.dll`이 같은 폴더에 없으면 연결할 때 `EW_NODLL(-15)`입니다.
- Linux 판마다 다릅니다 (`nm -D` 로 확인, 2026-10-14):

  | 판 | 비트 | `cnc_sdt*` |
  |---|---|---|
  | FOCAS2 디스크 `FOCAS64/LINUX/X64` 의 `libfocas32.so.1.0.0` | 64 | 없음 (함수 335개) |
  | strangesast/fwlib 의 `libfwlib32-linux-x64.so.1.0.5` | 64 | 없음 |
  | FOCAS2 Library `Fwlib/Linux/x86/libfwlib32.so.1.0.2` | 32 (i386) | **있음** (함수 853개) |
  | FOCAS2 Library `Fwlib/Linux/arm/libfwlib32.so.1.0.1` | 32 (ARM) | **있음** |

  64비트 프로그램은 32비트 라이브러리를 불러올 수 없으므로, Linux에서 수집하려면 **32비트 deelook**
  (`bin/deelook-linux-386`, ARM은 `deelook-linux-arm`)을 씁니다. 64비트 Linux에서도 그대로 돌아갑니다
  (2026-10-14 WSL Ubuntu 24.04 x86_64 + 31i 벤치: 서보 0.25 ms + 주축 1 ms 수집·CSV 확인).
  대상 PC에는 32비트 `libc`·`libstdc++`·`libgcc_s` 가 있어야 합니다. Ubuntu 에서는 처음 한 번:

  ```bash
  sudo apt install -y libc6-i386 lib32stdc++6 lib32gcc-s1
  ```

  확인은 `ldd libfwlib32.so.1.0.2` 에서 `not found` 가 없으면 됩니다. 32비트 deelook 자체도 이 라이브러리로 실행됩니다.
- Linux 판은 핸들을 쓰는 스레드에서 `cnc_startupprocess` 를 먼저 불러야 합니다. 다른 스레드에서 부르면 연결은 되지만
  그 핸들의 모든 호출이 `EW_HANDLE(-8)` 입니다 (실측). deelook은 연결할 때마다 그 스레드에서 부르고, 끊을 때
  `cnc_exitthread` 로 정리합니다 (deemesh와 같은 방식).

## `cnc_sdt*`를 직접 만들 때

deelook의 수집 코드는 다른 프로그램에 옮겨 쓰기 쉽게 나뉘어 있습니다. 순서대로 보면 됩니다.

| 파일 | 볼 것 |
|---|---|
| [`internal/focas/focas.go`](internal/focas/focas.go) | FOCAS2 구조체를 메모리 배치 그대로 옮긴 것(`IDBSDTCHAN` 6바이트, `IDBSDTCHAN2` 102바이트, `PMC_DATA`, `TRG_DATA`, `ODBSD`), 각 `cnc_sdt*` 호출, 오류 상세 번호(`cnc_getdtailerr`). 배치는 [`focas_test.go`](internal/focas/focas_test.go)가 검사합니다. |
| [`internal/focas/load_windows.go`](internal/focas/load_windows.go), [`load_linux.go`](internal/focas/load_linux.go) | cgo 없이 실행 중에 DLL/.so를 불러오는 법. Windows는 `Fwlib64.dll`이 이름으로 불러오는 `fwlibe64.dll`을 찾게 `SetDllDirectory`, Linux는 핸들을 쓰는 스레드마다 `cnc_startupprocess`. |
| [`internal/sampling/config.go`](internal/sampling/config.go) | 채널 설정: 주기·시프트를 `shift`에 넣는 법, PMC 주소 해석, 미리 막을 수 있는 규칙, PMC 비트 채널을 맨 뒤로 보내는 것. |
| [`internal/sampling/run.go`](internal/sampling/run.go) | 수집 순서(정리 → 설정 → 시작 → 읽기 반복 → 끝). 핸들은 OS 스레드 하나에 고정하고, 읽기는 쉬지 않고, DB 쓰기는 다른 고루틴으로 넘깁니다(늦게 읽으면 `EW_BUFFER`). 끝낼 때 `cnc_sdtendsmpl`·`cnc_sdtendsmpl2`·`cnc_sdtcancelsmpl` 중 무엇을 부르는지. |
| [`internal/sampling/types.go`](internal/sampling/types.go) | 서보 0~46·주축 0~21 데이터 종류와 단위 표. |
| [`internal/focas/integration_test.go`](internal/focas/integration_test.go) | 실제 CNC에 기능을 하나씩 시험해 보는 코드(환경 변수로 켬, 아래 "개발"). |

레퍼런스에 없는 함정은 위 "[실측으로 알게 된 것](#실측으로-알게-된-것)"에 모아 두었습니다.
특히 PMC 비트 채널 순서와, 확장 수집이 실패한 뒤 `cnc_sdtendsmpl2`를 꼭 불러야 하는 것은 놓치기 쉽습니다.

## 실행

### Windows: 폴더째 들고 다니며 실행 (포터블)

설치가 없습니다. `deelook.exe` 하나에 화면(웹 UI)과 SQLite까지 들어 있고, 같은 폴더에 두는 것은 이것뿐입니다.

```
deelook\
  deelook.exe
  Fwlib64.dll, fwlibe64.dll   ← FANUC FOCAS2 (직접 구해 넣음, 이더넷 연결은 이 둘이면 됨)
  deelook.toml          ← 설정 (없으면 처음 실행할 때 기본값으로 생김)
  deelook-data\         ← deelook이 쓰는 것 (처음 실행할 때 생김)
    sessions\                 ← 수집 한 건마다 .db 파일 하나 (넘겨줄 때는 그 파일만)
    deelook.db                ← deelook 내부용: 세션 목록, 세션 번호, 기계 목록과 IP, 수집 화면 설정 (넘겨주지 않음)
```

- **`deelook.exe`를 더블클릭**하면 시작하고 브라우저에 도움말(`/`)이 열립니다. 거기서 HMI 화면·시계열 수집으로 갑니다. 검은 창(로그)을 닫으면 끝납니다.
  설정 파일이 잘못됐거나 포트가 이미 쓰이는 등 시작하지 못하면 그 창에 이유를 띄우고 Enter를 누를 때까지 기다립니다.
  FOCAS2 라이브러리가 없으면 시작은 하고, 그 창과 시계열 수집 화면에 넣는 법을 알려 줍니다.
- 설정은 `deelook.toml`(TOML)입니다. 없으면 deelook이 시작할 때 **지금 쓰이는 값(기본값)을 그대로 적은 파일**을 만들어 두니,
  그 값을 고치면 다음 실행부터 적용됩니다(내용: [`deelook.example.toml`](deelook.example.toml)). 지우면 다음 실행 때 기본값으로
  다시 생깁니다. 이름은 아래 옵션과 같고, 명령줄로 준 값이 우선합니다. `db`, `fwlib`의 상대 경로는 그 폴더 기준입니다.

  ```toml
  hub = "http://192.168.0.10:8080"
  listen = "127.0.0.1:8090"

  [lamps]
  1 = "ncguide"
  ```
- 수집 데이터는 어디서 실행하든 `deelook.exe` 옆 `deelook-data\`에 쌓입니다. 폴더를 통째로 옮기면 데이터도 따라갑니다.
- 처음 실행할 때 Windows 방화벽이 허용을 물을 수 있습니다(기본 `listen = ":8090"`이 다른 PC의 접속도 받기 때문).
  이 PC에서만 쓸 거면 `listen = "127.0.0.1:8090"`.
- 인터넷에서 받은 서명 없는 실행 파일이라 SmartScreen이 막으면 "추가 정보 → 실행"을 누릅니다.

### 명령줄

```bash
bin/deelook-windows-amd64/deelook.exe -hub http://127.0.0.1:8080 -machine 2 -lamps 1=ncguide
```

브라우저에서 `http://<서버 주소>:8090/`을 엽니다.

| 주소 | 내용 |
|---|---|
| `/`, `/help` | 도움말: 이 프로그램이 무엇이고 어떻게 쓰는지, 지금 상태(허브 연결, FOCAS2 라이브러리, 기계, 수집, 디스크), 설정, HTTP API. 브라우저에는 HTML, 그 밖(curl, 에이전트)에는 Markdown(`/help.md`는 늘 Markdown). 내용은 [`help.md`](help.md) |
| `/hmi` | HMI 화면. 기계는 화면 아래 메뉴에서 고르거나(주소도 따라 바뀜) 주소에 `/hmi?machine=2`, `/hmi?machine=bench-31i`. 없으면 `-machine` |
| `/sampling` | 시계열 수집 |

예전 주소도 열립니다: `/?machine=2`는 `/hmi?machine=2`로, `/#/sampling`은 `/sampling`으로 넘어갑니다.

| 옵션 | 기본값 | 설명 |
|---|---|---|
| `-listen` | `:8090` | 웹 서버 주소 |
| `-hub` | `http://127.0.0.1:8080` | deemesh-hub 주소 |
| `-machine` | (없음) | 주소에 machine이 없을 때 HMI 화면에 보여 줄 기계: machine_id 또는 이름. 없으면 허브 목록에서 번호가 가장 작은 FANUC 기계 |
| `-interval` | `1s` | 한 화면을 읽는 간격 |
| `-timeout` | `5s` | 한 번 읽을 때 허브 쪽 제한 시간 |
| `-lamps` | (없음) | PMC로 램프를 읽을 기계: `1=ncguide,13=ncguide` (`deelook.toml`에서는 `[lamps]` 표) |
| `-fwlib` | 실행 파일 옆 `Fwlib64.dll` / `libfwlib32.so*` | FOCAS2 라이브러리 파일 (실행 파일과 같은 비트) |
| `-data` | 실행 파일 옆 `deelook-data` | 데이터 폴더: `sessions\`(수집 한 건마다 SQLite 파일), `deelook.db`(목록·기계) |
| `-minfree` | `1GB` | 데이터 폴더 디스크의 남은 공간이 이보다 적으면 수집을 시작하지 않거나 멈춤 (`0`: 보지 않음) |
| `-open` | 더블클릭으로 실행했을 때만 | 시작하면 브라우저로 도움말(`/`) 열기 |

- 보는 브라우저가 몇 개든 (기계, 화면)마다 허브를 한 번만 읽습니다. 화면을 바꾸면 그 화면에 필요한 값만 읽고, 아무도 보지 않으면 5초 뒤에 읽기를 멈춥니다.
- 허브와 다른 PC에서 돌리면 그 PC의 IP를 허브 설정 `allowed_ips_read`에 넣어야 합니다.

## 빌드

Node(npm)와 Go가 필요합니다.

```bash
./build.sh
```

`bin/` 아래에 대상마다 폴더가 생기고, 실행 파일과 그 대상의 FOCAS2 라이브러리가 함께 들어갑니다. 폴더째 복사해 실행하면 됩니다.

| 폴더 | 용도 |
|---|---|
| `deelook-windows-amd64/` | Windows 64비트 |
| `deelook-linux-386/` | Linux 32비트 (64비트 Linux에서도 32비트 `libc`·`libstdc++`·`libgcc_s`가 있으면 동작) |
| `deelook-linux-arm/` | 32비트 ARM Linux (기기에서 시험하지 않음) |

## 개발

```bash
go run . -lamps 1=ncguide -fwlib fwlib/windows-x64/Fwlib64.dll -data deelook-data   # 서버 (:8090)
cd web && npm run dev                                                    # 화면 (Vite, /api 요청은 :8090으로 넘김)
```

FOCAS2 라이브러리는 저장소에 없으므로 먼저 [`fwlib/`](fwlib/README.md)에 넣습니다.

실제 CNC로 FOCAS 호출을 시험하는 테스트는 환경 변수를 줄 때만 돕니다:

```bash
FOCAS_LIB=$PWD/fwlib/windows-x64/Fwlib64.dll FOCAS_HOST=192.0.2.10 FOCAS_PORT=8193 go test -count=1 -run TestSampling -v ./internal/focas/
```

`FOCAS_LIB`는 절대 경로로 줍니다(테스트는 패키지 폴더에서 돕니다). 기능 하나씩은 `-run TestFeatures`에
`FOCAS_CASE=rate|fixed|oneshot|seq|ext|recover`. `ext`는 `FOCAS_EXT=servo,cnc,pmc`(채널 순서), `FOCAS_PMC=Y0.4`,
`FOCAS_POINTS`, `FOCAS_MS`(읽기 간격), `FOCAS_DUMP=1`(채널별 원시 워드)로 조절합니다.

| 위치 | 내용 |
|---|---|
| `main.go` | HTTP 서버, 화면 스트림(SSE) |
| `help.go`, `help.md` | 도움말 페이지(`/`, `/help`): Markdown에 지금 상태를 넣어 브라우저에는 HTML로 |
| `internal/hub` | deemesh-hub 읽기 클라이언트 |
| `internal/live` | (기계, 화면)마다 폴링 하나, 구독자에게 분배 |
| `internal/screens/fanuc.go` | Fanuc 화면별로 읽을 deemesh 주소 |
| `internal/focas` | FOCAS2 호출 (샘플링 함수만, cgo 없이 Windows DLL·Linux .so를 실행 중에 불러옴) |
| `internal/sampling` | 수집 세션(기계마다 OS 스레드에 고정된 고루틴 하나), SQLite 저장, CSV, API |
| `web/src/sampling` | 시계열 수집 화면 |
| `web/src/screens/fanuc` | Fanuc 화면(SVG, 원본 픽셀 좌표), 화면 전환(`nav.svelte.ts`), 조작반(`panels/`) |

## 라이선스

[MIT](LICENSE). FANUC FOCAS2 라이브러리는 포함되어 있지 않으며 FANUC의 이용 조건을 따릅니다.
