// What the cnc_sdt* references (inventcom, revision 2026-03-24) say beyond
// the data type tables (those come from the server), in Korean, so the
// page answers what the reference pages would. Plus what deelook measured.

/** PMC address kinds in PMC_DATA.adr order (0..13). */
export const PMC_KINDS: { letter: string; meaning: string; trigger: boolean; note?: string }[] = [
  { letter: 'G', meaning: 'PMC → CNC 신호', trigger: true },
  { letter: 'F', meaning: 'CNC → PMC 신호', trigger: true },
  { letter: 'Y', meaning: 'PMC → 기계 신호 (출력)', trigger: true },
  { letter: 'X', meaning: '기계 → PMC 신호 (입력)', trigger: true },
  { letter: 'A', meaning: '메시지 표시', trigger: true },
  { letter: 'R', meaning: '내부 릴레이', trigger: true },
  { letter: 'T', meaning: '타이머', trigger: false },
  { letter: 'K', meaning: '킵 릴레이', trigger: true },
  { letter: 'C', meaning: '카운터', trigger: false },
  { letter: 'D', meaning: '데이터 테이블', trigger: true },
  { letter: 'M', meaning: '다른 PMC 경로에서 오는 입력', trigger: true },
  { letter: 'N', meaning: '다른 PMC 경로로 가는 출력', trigger: true },
  { letter: 'E', meaning: '엑스트라 릴레이', trigger: true },
  { letter: 'Z', meaning: '내부 릴레이', trigger: false, note: 'PMC 메모리 C·D형에서만' },
]

/** PMC_DATA.unittype. */
export const PMC_UNITS: { unit: number; meaning: string }[] = [
  { unit: 1, meaning: '1번째 PMC (PMC-L 포함)' },
  { unit: 2, meaning: '2번째 PMC (옵션)' },
  { unit: 3, meaning: '3번째 PMC (옵션)' },
  { unit: 4, meaning: '4번째 PMC (옵션)' },
  { unit: 5, meaning: '5번째 PMC (옵션)' },
  { unit: 9, meaning: '듀얼 체크 세이프티 PMC (옵션)' },
]

/** How many points one channel set can take when read only after sampling ends (cnc_sdtsetchnl). */
export const MAX_POINTS = [262144, 131072, 87376, 65536, 52424, 43688, 37448, 32768]

/** Which CNCs have the calls: cnc_sdtsetchnl family / cnc_sdtsetchnl2 family (M and T series). */
export const SUPPORTED = [
  { cnc: '0i-D', basic: true, ext: false },
  { cnc: '0i-F', basic: true, ext: true },
  { cnc: '30i-A', basic: true, ext: false },
  { cnc: '30i-B', basic: true, ext: true },
  { cnc: 'Power Motion i-A', basic: true, ext: false },
]

type Explain = { code: string; detail?: number; fns?: string[]; text: string; cleanup?: boolean }

// most specific first: function + detail, then code + detail, then code
const EXPLAIN: Explain[] = [
  { code: 'EW_REJECT', fns: ['cnc_sdtcancelsmpl', 'cnc_sdtendsmpl', 'cnc_sdtendsmpl2'], text: '끝낼 샘플링이 없음 (설정 전 정리라면 정상)', cleanup: true },
  { code: 'EW_REJECT', detail: 1, fns: ['cnc_sdtstartsmpl', 'cnc_sdtstartsmpl2'], text: '이미 샘플링이 시작되어 있음 — cnc_sdtcancelsmpl 또는 cnc_sdtendsmpl 뒤에 다시' },
  { code: 'EW_REJECT', detail: 2, fns: ['cnc_sdtstartsmpl', 'cnc_sdtstartsmpl2'], text: '채널 설정(cnc_sdtsetchnl)이 끝나지 않음' },
  { code: 'EW_REJECT', detail: 1, fns: ['cnc_sdtreadsmpl', 'cnc_sdtreadsmpl2'], text: '샘플링 시작 함수를 부르지 않았음' },
  { code: 'EW_REJECT', detail: 2, fns: ['cnc_sdtread1shot'], text: '채널 설정이 끝나지 않음' },
  { code: 'EW_REJECT', detail: 3, fns: ['cnc_sdtread1shot'], text: '그 밖의 오류' },
  { code: 'EW_REJECT', fns: ['cnc_sdtsetchnl', 'cnc_sdtsetchnl2'], text: '이미 샘플링 중 (다른 프로그램 포함) — cnc_sdtcancelsmpl 또는 cnc_sdtendsmpl 뒤에 다시. Servo Guide를 쓰는 중이어도 거부됨' },
  { code: 'EW_REJECT', text: 'CNC가 실행을 거부함. Servo Guide(Mate)를 쓰는 중이면 cnc_sdt* 전체가 거부됨' },
  { code: 'EW_DATA', fns: ['cnc_sdtstartsmpl'], text: '채널 설정이 안 되었거나 틀림' },
  { code: 'EW_DATA', fns: ['cnc_sdtstartsmpl2'], text: '트리거 조건(condition)이 틀림' },
  { code: 'EW_DATA', detail: 3, text: '총 점 수(datanum)가 8 미만' },
  { code: 'EW_DATA', detail: 4, text: '지정한 채널의 서보 모터·주축·PMC 신호를 쓸 수 없음 (축이 없거나, DSP당 서보 4채널·앰프당 주축 2채널 초과 등)' },
  { code: 'EW_DATA', detail: 20, text: '데이터 종류(type)가 틀림 — 이 서보·주축 소프트웨어에 없는 종류일 수 있음' },
  { code: 'EW_DATA', detail: 21, text: '채널 번호(chno)가 틀림' },
  { code: 'EW_DATA', detail: 22, text: '축 번호(axis)가 틀림' },
  { code: 'EW_DATA', detail: 23, text: '오른쪽 시프트 값이 틀림' },
  { code: 'EW_DATA', detail: 24, text: '샘플링 주기가 틀리거나 채널끼리 다름' },
  { code: 'EW_DATA', detail: 30, text: 'CNC 정보 설정이 틀림' },
  { code: 'EW_DATA', detail: 50, text: 'PMC 신호의 PMC 경로(unittype)가 틀림' },
  { code: 'EW_DATA', detail: 51, text: 'PMC 주소 종류(adr)가 틀림' },
  { code: 'EW_DATA', detail: 52, text: 'PMC 주소 번호가 틀림 (그 PMC에 없는 주소)' },
  { code: 'EW_DATA', detail: 53, text: 'PMC 비트 번호가 틀림' },
  { code: 'EW_DATA', detail: 54, text: 'PMC 비트 신호 수가 한도를 넘음 (16개, 바이트·워드 채널이 있으면 8개)' },
  { code: 'EW_DATA', detail: 56, text: 'PMC 비트 채널을 둘 이상 지정함' },
  { code: 'EW_DATA', text: '설정 값 오류' },
  { code: 'EW_NUMBER', fns: ['cnc_sdtreadsmpl', 'cnc_sdtreadsmpl2'], text: '읽을 점 수(datanum)가 8 미만' },
  { code: 'EW_NUMBER', text: '채널 수(num)가 틀림' },
  { code: 'EW_PATH', text: '지정한 축이나 계통(path)에 해당하는 계통이 없음' },
  { code: 'EW_BUFFER', text: 'CNC 안의 읽기 버퍼가 가득 차 처리가 취소됨 — CNC가 모으는 속도보다 늦게 읽음' },
  { code: 'EW_VERSION', text: '이 CNC가 지정한 데이터 종류(축)를 지원하지 않음 (PMC 바이트·워드는 30i-B 이후, 확장 함수는 0i-F·30i-B)' },
  { code: 'EW_FUNC', text: '이 CNC나 라이브러리에 없는 함수' },
  { code: 'EW_NOOPT', text: '필요한 CNC 옵션이 없음' },
  { code: 'EW_SOCKET', text: '통신이 끊김 (네트워크, CNC 전원)' },
  { code: 'EW_HANDLE', text: '연결 핸들이 유효하지 않음 (연결이 끊겼거나, 리눅스에서 다른 스레드의 핸들)' },
  { code: 'EW_PROTOCOL', text: '통신 데이터 오류' },
  { code: 'EW_NODLL', text: '기종별 DLL이 없음 (Fwlib64.dll 옆에 fwlib30i64.dll 등이 있어야 함)' },
  { code: 'EW_BUSY', text: 'CNC가 바쁨 — 잠시 뒤 다시' },
]

/** A Korean explanation of the FOCAS2 error in text ("cnc_x: EW_Y(n) detail d"), or ''. */
export function explainError(text: string): string {
  const m = /(cnc_\w+): (EW_\w+)\(-?\d+\)(?: detail (\d+))?/.exec(text)
  if (!m) return ''
  const [, fn, code, d] = m
  const detail = d ? Number(d) : undefined
  const hit = EXPLAIN.find(
    (e) => e.code === code && (e.detail === undefined || e.detail === detail) && (!e.fns || e.fns.includes(fn)),
  )
  return hit?.text ?? ''
}

/** The error codes as a table, for the reference panel. */
export const ERROR_TABLE = EXPLAIN.map((e) => ({
  code: e.code,
  detail: e.detail,
  fns: e.fns?.join(', ') ?? '',
  text: e.text,
}))
