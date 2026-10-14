# FANUC FOCAS2 라이브러리 (저장소에 없음)

deelook의 시계열 수집(`cnc_sdt*`)은 FANUC의 FOCAS2 라이브러리를 불러옵니다. 이 라이브러리는 FANUC의 것이라
**저장소에 넣지 않습니다.** FANUC FOCAS2 라이브러리(개발 키트 디스크 등)에서 아래 파일을 구해 이 폴더의 플랫폼별
폴더에 넣으면 `build.sh`가 실행 파일 옆에 복사합니다. deelook은 `-fwlib`가 없으면 실행 파일 옆에서 찾고, 라이브러리가
없거나 `cnc_sdt*`가 빠져 있으면 시작하지 않습니다. 이 폴더의 `.dll`, `.so*` 파일은 git이 무시합니다.

**라이브러리와 실행 파일의 비트가 같아야 합니다.**

| 폴더 | deelook 빌드 | 파일 | 시험한 판 |
|---|---|---|---|
| `windows-x64/` | `windows-amd64` | `Fwlib64.dll` | 7.3.0.1 |
| | | `fwlibe64.dll` (이더넷) | 5.3.0.1 |
| `linux-x86/` | `linux-386` | `libfwlib32.so.1.0.2` (ELF 32비트 i386) | 1.0.2 |
| `linux-arm/` | `linux-arm` | `libfwlib32.so.1.0.1` (ELF 32비트 ARM) | 1.0.1 |

Windows는 이더넷 연결에 이 두 파일이면 됩니다. `fwlibe64.dll`은 `Fwlib64.dll`이 이름으로 불러오므로 같은 폴더에 있어야 합니다
(없으면 연결할 때 `EW_NODLL(-15)`). 기종별 DLL(`fwlib30i64.dll`, `fwlib0iD64.dll` 등)은 없어도 NC Guide 0i-F와 30i-B에서
기본·확장·현재값이 모두 됐습니다(2026-10-14). Linux 판은 FOCAS2 Library의 `Fwlib/Linux/x86`, `Fwlib/Linux/arm`에 있는 32비트 판입니다.

## 왜 이 판인가 (2026-10-14 확인)

- `cnc_sdt*`(서보·주축 샘플링)는 0i-D, 0i-F, 30i-A, 30i-B (그리고 Power Motion i-A)만 지원합니다 (FOCAS2 스펙
  `cnc_sdtsetchnl` 지원표).
- Windows는 64비트 7.3.0.1을 씁니다. 같은 IP의 다른 포트(포트 포워딩 뒤 장비)를 한 프로세스에서 열 때
  6.9.0.1은 먼저 연 장비의 연결을 내주고 7.3.0.1은 포트까지 가립니다.
- 확인한 Linux 64비트 판(`libfocas32.so.1.0.0` 등)에는 `cnc_sdt*`가 없습니다. 그래서 Linux는 32비트 판과 32비트
  deelook을 씁니다. 64비트 Linux에서도 돌아갑니다 (32비트 `libc`·`libstdc++`·`libgcc_s` 필요:
  Ubuntu `sudo apt install -y libc6-i386 lib32stdc++6 lib32gcc-s1`).
- Linux 32비트 판에는 확장 함수(`cnc_sdt*2`: PMC 신호, CNC 정보, 트리거)가 없어 기본 수집만 됩니다.

## 확인 (SHA-256)

시험한 파일입니다. 다른 판도 `cnc_sdt*`가 있으면 쓸 수 있습니다 (deelook이 시작할 때 확인).

```
7b3837e3925902d1e06c5c7e4f9ce39fe7f39aa9caed029096187f5641ec04ab  windows-x64/Fwlib64.dll
d3341b43bf3945bdb49d03d1d96436a75911bf3c8d1cdf4f30b9167ef935bfd6  windows-x64/fwlibe64.dll
5c10629358c8af28e83a451070436229c180f86f183ac34b9629cfb0d9a71194  linux-x86/libfwlib32.so.1.0.2
d62b5efc41688c86b31fd029d230c42af9986689b0d3d8ae9739563d171d233d  linux-arm/libfwlib32.so.1.0.1
```
