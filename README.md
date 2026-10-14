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
| **HMI 화면** (`#/`) | CNC 공통 인터페이스로 제어기 화면을 만드는 법 | 값은 전부 **deemesh-hub**에서 읽습니다(읽기 전용, 기계에 쓰는 경로 없음). deemesh는 FANUC·SIEMENS·Mitsubishi 등 여러 제어기를 같은 주소 체계로 다루는 공통 인터페이스입니다(무료). |
| **시계열 수집** (`#/sampling`) | deemesh가 다루지 않는 FANUC 고속 샘플링을 **직접 만드는 법** | deelook이 FOCAS2 라이브러리를 직접 불러 CNC에 연결합니다. 서보·주축 데이터, PMC 신호, O·N 번호, 트리거까지 `cnc_sdt*`의 모든 기능을 화면에서 써 보고 SQLite에 저장합니다. |

**무엇을 쓰면 되나**

- 여러 메이커 CNC의 상태·좌표·프로그램·알람 같은 값을 한 가지 방식으로 읽고 쓰고 싶다 → **deemesh**
- FANUC 서보·주축 파형(토크, 속도, 위치 편차 …)을 ms 단위로 모으고 싶다 → 이 저장소의 코드를 보고 직접 만드세요
  (MIT라 그대로 가져다 써도 됩니다). 어디부터 보면 되는지는 아래 "[`cnc_sdt*`를 직접 만들 때](#cnc_sdt를-직접-만들-때)".

`cnc_sdt*`는 필수입니다. 그래서 deelook은 **Windows 64비트와 Linux 32비트**로만 나오고(그 함수가 든 FOCAS2
라이브러리가 있는 조합), 시작할 때 라이브러리를 불러와 없거나 함수가 빠졌으면 시작하지 않습니다.

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

- 허브에서 받은 Fanuc 기계 목록(IP·포트, 축 이름)은 받을 때마다 SQLite(`-db`)에 저장합니다. 허브에 연결이 안 되면 화면에
  알림을 띄우고 마지막으로 받은 목록으로 수집합니다. 허브 목록에서 빠진 기계는 다음에 목록을 받을 때 같이 빠집니다.
- 허브에 없는 기계는 화면의 **기계 관리**에서 이름·IP·FOCAS2 포트로 직접 추가합니다(번호 `L1`, `L2`…). 추가할 때 연결해서
  축·주축 이름을 FOCAS2(`cnc_getpath`, `cnc_setpath`, `cnc_rdaxisname`, `cnc_rdspdlname`)로 읽으므로 기계가 켜져 있어야 합니다.
  "축 이름을 CNC에서 읽기"는 허브 기계에도 쓸 수 있습니다(허브가 꺼져 있고 저장된 이름이 없을 때 등).
- 축 이름을 모르면 축은 번호(축1~32, S1~8)로 고릅니다.
- deelook을 디메시와 다른 PC에서 돌리면 deelook PC에서 CNC의 FOCAS2 포트로 직접 접속이 되어야 합니다.

| 방식 | 함수 | 할 수 있는 것 |
|---|---|---|
| 기본 | `cnc_sdtsetchnl` `cnc_sdtstartsmpl` `cnc_sdtreadsmpl` `cnc_sdtendsmpl` `cnc_sdtcancelsmpl` | 서보(0~46번)·주축(0~21번) 데이터. **종료할 때까지**(datanum -1) 또는 **점 수 지정**(다 모으면 CNC가 끝냄 → `cnc_sdtendsmpl`). **즉시** 또는 **블록 N이 실행될 때부터** 시작(그때까지 "시작 블록 대기") |
| 확장 | `cnc_sdtsetchnl2` `cnc_sdtstartsmpl2` `cnc_sdtreadsmpl2` `cnc_sdtendsmpl2` | 위에 더해 **PMC 신호**(비트 최대 16개를 한 채널에, 30i-B 이후는 바이트·워드)와 **CNC 정보**(O·N 번호, 계통별)를 같은 시간축으로. **트리거**: 블록 N 실행, PMC 비트의 상승/하강을 CNC가 표시(수집을 멈추지 않음). 종료할 때까지 또는 점 수 지정 |
| 현재값 | `cnc_sdtsetchnl` `cnc_sdtread1shot` | 수집 없이 서보·주축 데이터의 지금 값을 한 번(또는 1초마다) 읽기 |

- 주기: 서보 1 ms 또는 0.25 ms, 주축 1 ms, CNC 정보는 4 ms마다 갱신(1 ms 채널이면 같은 값 4번).
  0.25 ms 서보와 주축을 같이 모으면 "종료할 때까지"만 됩니다(CNC 제약).
- 값은 CNC가 주는 16비트 원시값입니다(물리 단위로 바꾸지 않음). 데이터별 환산은 화면의 단위 설명 참고.
  서보·주축은 부호 있는 값, PMC·CNC 정보는 부호 없는 값으로 다룹니다.
- 화면: 수집 중에는 최근 3초를 채널마다 차트로(PMC 비트는 신호별 파형), 트리거는 점선과 목록으로. 상태에는 읽기 호출 수,
  한 번에 오는 점 수, 호출 시간, 읽는 속도 대 CNC 샘플 속도, 밀릴 때 경고, FOCAS2 호출 기록(결과 코드 포함)이 나옵니다.
  기록된 세션은 **보기**로 전체를 차트로 보고(확대·이동, 트리거로 이동) **CSV**로 받습니다.
- **cnc_sdt\* 안내** 패널: 레퍼런스를 찾아보지 않아도 되도록 서보 0~46·주축 0~21·CNC 정보의 모든 데이터 종류를
  한국어 이름, 조건별 단위(시프트 0일 때), 권장 시프트, 비고와 함께 표로 보여 주고(찾기, 표에서 바로 채널 추가),
  PMC 주소 종류·경로, 주기·시프트·점 수 규칙, 오류 코드와 상세 번호, 지원 기종, deelook 실측 내용을 정리해 둡니다.
  채널마다 고른 데이터의 단위 전부가 그 밑에 나오고, FOCAS2 오류 메시지에는 한국어 설명이 붙습니다.
- CSV: `time_ms`(첫 샘플부터), 채널마다 한 열(PMC 비트는 신호마다 0/1 한 열), 트리거가 있으면 `trigger` 열(그 시점의 비트).
- 저장: `-db`(기본 `deelook-data/sampling.db`). 세션 하나 = 시작~종료, 샘플은 채널별 묶음(BLOB), 트리거는 따로.
  1 ms·8채널이면 시간당 약 58 MB입니다.
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
  트리거가 있는 읽기는 그 순간에서 끊겨 돌아옵니다. 신호가 처음부터 1이면 시작 직후 상승으로 표시되기도 했습니다.
  블록 트리거는 CNC 정보 채널의 N 번호가 바뀌기 몇 ms 전에 찍힙니다(N 번호는 4 ms마다 갱신).
- 점 수를 정한 확장 수집이 끝나면 **PMC·CNC 정보 채널의 마지막 몇 점이 0**으로 옵니다(PMC 4점, N 번호 8점을 봄).
- `cnc_sdtreadsmpl2`가 실패한 뒤에는 `cnc_sdtendsmpl2`를 불러야 합니다. 안 부르면 CNC 샘플러가 멈춰 NC Guide는
  새 연결까지 거부했습니다. deelook은 실패·종료 때마다 부르고, 설정 전에도 남은 세션을 정리합니다
  (정리할 것이 없으면 `EW_REJECT`가 정상).
- 한 번의 읽기로 오는 양은 모든 채널 합쳐 약 650~690워드(약 1.3 KB)입니다. 그래서 채널이 많거나 0.25 ms이거나
  네트워크가 느리면 CNC보다 늦게 읽게 됩니다. 31i 벤치(인터넷 너머): 1 ms 3채널은 초당 약 1000점으로 따라가지만,
  0.25 ms 서보 2 + 주축 1은 초당 약 3000~3600점으로 4000에 못 미칩니다. 기본 방식은 그러다 `EW_BUFFER`로 멈추고,
  점 수를 정한 방식은 CNC가 다 모은 순간 끝나서 못 읽은 점이 사라집니다. 화면에 경고가 뜹니다.
- 이 31i 벤치는 PMC 바이트·워드 채널에 `EW_VERSION`을 줍니다(그 기능이 없는 소프트웨어).
- NC Guide는 실제 서보가 없어 서보 값이 0이고, 초당 샘플 수가 1000이 아니라 약 1300입니다(시뮬레이터 시간).
  PMC 신호, N 번호, 트리거, 블록 N 시작은 NC Guide에서 MDI 프로그램을 돌려 확인했습니다.

FOCAS2 라이브러리는 시작할 때 불러옵니다 (`-fwlib`, 없으면 실행 파일 옆).

| OS | 필요한 파일 (`-fwlib`로 지정, 기본은 실행 파일 옆) | 상태 |
|---|---|---|
| Windows 64비트 | `Fwlib64.dll` 7.3, `fwlibe64.dll`, 기종별 DLL(`fwlib30i64.dll`, `fwlib0iD64.dll`, `fwlibNCG64.dll`, `fwlib0DN64.dll`)을 한 폴더에 | 기본·확장·현재값 모두, 31i 벤치·NC Guide에서 확인 |
| Linux 32비트 | `libfwlib32.so.1.0.2`(x86) / ARM은 `1.0.1` | 기본·현재값 확인. **확장(`cnc_sdt*2`)은 이 라이브러리에 없어** 화면에서 꺼짐 |

라이브러리는 FANUC의 것이라 **저장소에 없습니다.** 구해서 [`fwlib/`](fwlib/README.md)의 플랫폼별 폴더에 넣으면(파일 이름·시험한 판·해시는 그 문서)
`build.sh`가 실행 파일 옆에 복사합니다.

- Windows는 `Fwlib64.dll` 7.3.0.1 + `fwlibe64.dll` 5.3.0.1(포트까지 연결을 가르는 판)에 기종별 DLL 4.9.0.1을 함께 둔 구성으로 시험했습니다.
  기종별 DLL이 없으면 연결할 때 `EW_NODLL(-15)`입니다.
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
| [`internal/focas/load_windows.go`](internal/focas/load_windows.go), [`load_linux.go`](internal/focas/load_linux.go) | cgo 없이 실행 중에 DLL/.so를 불러오는 법. Windows는 기종별 DLL을 찾게 `SetDllDirectory`, Linux는 핸들을 쓰는 스레드마다 `cnc_startupprocess`. |
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
  Fwlib64.dll, fwlibe64.dll, fwlib30i64.dll, fwlib0iD64.dll, fwlibNCG64.dll, fwlib0DN64.dll   ← FANUC FOCAS2 (직접 구해 넣음)
  deelook.conf          ← 설정 (없으면 기본값, deelook.conf.example 참고)
  deelook-data\         ← 수집 데이터 (처음 실행할 때 생김)
```

- **`deelook.exe`를 더블클릭**하면 시작하고 브라우저가 열립니다. 검은 창(로그)을 닫으면 끝납니다.
  라이브러리가 없는 등 시작하지 못하면 그 창에 이유를 띄우고 Enter를 누를 때까지 기다립니다.
- 설정은 `deelook.conf`에 `이름 = 값`으로 적습니다(예: `hub = http://192.168.0.10:8080`). 이름은 아래 옵션과 같고,
  명령줄에 준 값이 이깁니다. `db`, `fwlib`의 상대 경로는 그 폴더 기준입니다.
- 수집 데이터는 어디서 실행하든 `deelook.exe` 옆 `deelook-data\`에 쌓입니다. 폴더를 통째로 옮기면 데이터도 따라갑니다.
- 처음 실행할 때 Windows 방화벽이 허용을 물을 수 있습니다(`listen = :8090`이 다른 PC의 접속도 받기 때문).
  이 PC에서만 쓸 거면 `listen = 127.0.0.1:8090`.
- 인터넷에서 받은 서명 없는 실행 파일이라 SmartScreen이 막으면 "추가 정보 → 실행"을 누릅니다.

### 명령줄

```bash
bin/deelook-windows-amd64/deelook.exe -hub http://127.0.0.1:8080 -machine 1 -lamps 1=ncguide
```

브라우저에서 `http://<서버 주소>:8090/` 을 엽니다. 다른 기계는 `/?machine=2` 또는 `/?machine=bench-31i`.

| 옵션 | 기본값 | 설명 |
|---|---|---|
| `-listen` | `:8090` | 웹 서버 주소 |
| `-hub` | `http://127.0.0.1:8080` | deemesh-hub 주소 |
| `-machine` | `1` | 주소에 machine이 없을 때 보여줄 기계 |
| `-interval` | `1s` | 한 화면을 읽는 간격 |
| `-timeout` | `5s` | 한 번 읽을 때 허브 쪽 제한 시간 |
| `-lamps` | (없음) | PMC로 램프를 읽을 기계: `1=ncguide,13=ncguide` |
| `-fwlib` | 실행 파일 옆 `Fwlib64.dll` / `libfwlib32.so*` | FOCAS2 라이브러리 파일 (실행 파일과 같은 비트) |
| `-db` | 실행 파일 옆 `deelook-data/sampling.db` | 수집 데이터 SQLite 파일 |
| `-open` | 더블클릭으로 실행했을 때만 | 시작하면 브라우저로 화면 열기 |

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
go run . -lamps 1=ncguide -fwlib fwlib/windows-x64/Fwlib64.dll -db deelook-data/sampling.db   # 서버 (:8090)
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
| `internal/hub` | deemesh-hub 읽기 클라이언트 |
| `internal/live` | (기계, 화면)마다 폴링 하나, 구독자에게 분배 |
| `internal/screens/fanuc.go` | Fanuc 화면별로 읽을 deemesh 주소 |
| `internal/focas` | FOCAS2 호출 (샘플링 함수만, cgo 없이 Windows DLL·Linux .so를 실행 중에 불러옴) |
| `internal/sampling` | 수집 세션(기계마다 OS 스레드에 고정된 고루틴 하나), SQLite 저장, CSV, API |
| `web/src/sampling` | 시계열 수집 화면 |
| `web/src/screens/fanuc` | Fanuc 화면(SVG, 원본 픽셀 좌표), 화면 전환(`nav.svelte.ts`), 조작반(`panels/`) |

## 라이선스

[MIT](LICENSE). FANUC FOCAS2 라이브러리는 포함되어 있지 않으며 FANUC의 이용 조건을 따릅니다.
