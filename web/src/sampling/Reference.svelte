<script lang="ts">
  // The cnc_sdt* reference in Korean: every data type with its unit and
  // notes, the PMC address kinds, the setting rules, error codes and the
  // CNCs that have the calls, so nobody has to look them up elsewhere.
  // A data row's + adds a channel of that type.
  import type { DataType, Kind, Types, Unit } from './api'
  import { ERROR_TABLE, MAX_POINTS, PMC_KINDS, PMC_UNITS, SUPPORTED } from './reference'

  let {
    types,
    canAdd,
    onadd,
  }: {
    types: Types
    canAdd: (kind: Kind) => string // '' when it can be added, else why not
    onadd: (kind: Kind, type: number) => void
  } = $props()

  const TABS = [
    { id: 'servo', label: '서보 데이터 (0–46)' },
    { id: 'spindle', label: '주축 데이터 (0–21)' },
    { id: 'cnc', label: 'CNC 정보' },
    { id: 'pmc', label: 'PMC 신호' },
    { id: 'rules', label: '설정 규칙' },
    { id: 'errors', label: '오류 코드' },
    { id: 'cncs', label: '지원 기종' },
    { id: 'measured', label: 'deelook 실측' },
  ] as const
  type Tab = (typeof TABS)[number]['id']

  let tab = $state<Tab>('servo')
  let query = $state('')

  const unitText = (x: Unit) => `${x.raw}${x.means ? ` = ${x.means}` : ''}`
  function rows(kind: 'servo' | 'spindle' | 'cnc'): DataType[] {
    const q = query.trim().toLowerCase()
    if (!q) return types[kind]
    return types[kind].filter((d) =>
      [String(d.type), d.code, d.name, d.nameKo, d.unit, d.note ?? ''].some((s) => s.toLowerCase().includes(q)),
    )
  }
</script>

<section class="card ref">
  <details open>
    <summary><h2>cnc_sdt* 안내</h2> <span class="hint">데이터 종류·단위·규칙·오류 코드 (FANUC FOCAS2 레퍼런스 정리)</span></summary>

    <div class="tabs" role="tablist">
      {#each TABS as t (t.id)}
        <button role="tab" aria-selected={tab === t.id} class:on={tab === t.id} onclick={() => (tab = t.id)}>{t.label}</button>
      {/each}
    </div>

    {#if tab === 'servo' || tab === 'spindle' || tab === 'cnc'}
      {@const kind = tab}
      <div class="tools">
        <input type="search" bind:value={query} placeholder="번호·코드·이름·단위로 찾기" aria-label="데이터 종류 찾기" />
        <span class="hint">
          {#if kind === 'servo'}type 값 그대로 cnc_sdtsetchnl의 channel.type에 들어갑니다. 축 번호는 1~32 (모든 계통의 축을 순서대로).
          {:else if kind === 'spindle'}주축은 축 번호를 음수(-1~-8)로 줍니다. 주기는 항상 1 ms입니다.
          {:else}CNC 정보는 cnc_sdtsetchnl2에서만, 축 번호 101~115 = 계통 1~15. 4 ms마다 갱신되어 1 ms 채널이면 같은 값이 4개, 0.25 ms면 16개
            나옵니다. 시프트는 무시됩니다.{/if}
        </span>
      </div>
      <div class="scroll">
        <table class="types">
          <thead>
            <tr><th>type</th><th>코드</th><th>데이터</th><th>단위 (시프트 0일 때)</th><th>시프트</th><th>비고</th><th></th></tr>
          </thead>
          <tbody>
            {#each rows(kind) as d (d.type)}
              {@const why = canAdd(kind)}
              <tr>
                <td class="num">{d.type}</td>
                <td><code>{d.code}</code></td>
                <td>{d.nameKo}<br /><span class="en">{d.name}</span></td>
                <td>
                  {#each d.units as x, i (i)}
                    <div class="unit">{unitText(x)}{#if x.when}{' '}<span class="when">({x.when})</span>{/if}</div>
                  {/each}
                </td>
                <td class="num">{d.shift || ''}</td>
                <td class="note">{d.note ?? ''}</td>
                <td>
                  <button class="add" disabled={!!why} title={why || `이 데이터로 채널 추가`} aria-label="{d.code} 채널 추가"
                    onclick={() => onadd(kind, d.type)}>+</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
      <p class="hint">
        "시프트"는 값을 16비트에 맞추려고 레퍼런스가 예로 든 오른쪽 시프트이고, 데이터를 고르면 그 값이 들어갑니다. 값은 CNC가 주는 그대로이므로
        단위 열의 비율로 환산합니다 (시프트 n이면 값 × 2ⁿ). 서보 소프트웨어 버전이나 옵션에 따라 없는 종류는 설정할 때 EW_DATA(20)이 납니다.
      </p>
    {:else if tab === 'pmc'}
      <div class="cols2">
        <div>
          <h3>채널 (cnc_sdtsetchnl2)</h3>
          <ul>
            <li><b>비트</b> (axis -127): 한 채널에만, 신호 최대 16개 (바이트·워드 채널이 함께 있으면 8개). 값의 비트 i가 i번째 신호.</li>
            <li><b>바이트</b> (axis -126), <b>워드</b> (axis -125): 30i-B 이후. 채널마다 신호 1개, io[1]~io[15]는 0.</li>
            <li>type과 시프트는 무시됩니다. 주소는 PMC_DATA {'{'}unittype, adr, bit, no{'}'}로 줍니다 (바이트·워드는 bit 0).</li>
            <li>unittype이 0인 칸부터 뒤는 무시됩니다.</li>
          </ul>
          <h3>트리거 = 책갈피 (cnc_sdtstartsmpl2)</h3>
          <ul>
            <li>condition 비트: SEQ = 블록 N 실행, P_UP = 신호 0→1 (상승), P_DW = 신호 1→0 (하강). 변화 전부를 보려면 P_UP과 P_DW 둘 다.</li>
            <li>TRG_DATA {'{'}seq_no, pmc_trg{'}'}: 블록 번호와 PMC 비트 하나. 블록 번호는 path로 정한 계통에서 봅니다.</li>
            <li>트리거는 샘플링을 시작·종료하지 않고 그 순간에 표시만 남깁니다(그래서 deelook 화면에서는 책갈피라고 부릅니다). cnc_sdtreadsmpl2의 trg에 표시되고, 그 읽기의 마지막 점이 그 순간입니다.</li>
            <li>T(타이머)·C(카운터) 주소는 책갈피(트리거) 신호로 쓸 수 없습니다 (Z도 목록에 없음).</li>
          </ul>
        </div>
        <div>
          <h3>주소 종류 (adr)</h3>
          <table>
            <thead><tr><th>adr</th><th>주소</th><th>뜻</th><th>책갈피</th></tr></thead>
            <tbody>
              {#each PMC_KINDS as k, i (k.letter)}
                <tr><td class="num">{i}</td><td><code>{k.letter}</code></td><td>{k.meaning}{#if k.note} <span class="hint">({k.note})</span>{/if}</td><td>{k.trigger ? '가능' : '불가'}</td></tr>
              {/each}
            </tbody>
          </table>
          <h3>PMC 경로 (unittype)</h3>
          <table>
            <tbody>
              {#each PMC_UNITS as p (p.unit)}<tr><td class="num">{p.unit}</td><td>{p.meaning}</td></tr>{/each}
            </tbody>
          </table>
        </div>
      </div>
    {:else if tab === 'rules'}
      <div class="cols2">
        <div>
          <h3>채널</h3>
          <ul>
            <li>최대 8채널. 서보는 DSP(2축)당 4채널, 주축은 앰프 하나당 2채널까지 (넘으면 EW_DATA(4)).</li>
            <li>축 번호: 서보 1~32, 주축 -1~-8, PMC -125/-126/-127, CNC 정보 101~115.</li>
            <li>위치 피드백 누적값(서보 0번, 주축 0번)은 CNC 전원을 켤 때 0에서 시작합니다.</li>
            <li>Servo Guide·Servo Guide Mate와 동시에 쓸 수 없습니다 (EW_REJECT).</li>
          </ul>
          <h3>주기와 시프트 (channel.shift)</h3>
          <ul>
            <li>윗바이트 = 주기: 0 = 1 ms, 2 = 0.25 ms (서보만). 주축은 항상 1 ms. 채널끼리 주기가 맞지 않으면 EW_DATA(24).</li>
            <li>아랫바이트 = 오른쪽 시프트 0~15: 값을 2ⁿ으로 나눠 16비트에 맞춥니다. 0이면 1 검출 단위(또는 1회전), 1이면 2, 2면 4,
              7이면 128, 15면 32,768 검출 단위가 1이 됩니다. PMC·CNC 정보 채널에는 무시.</li>
            <li>예: 속도 지령(VCMD)은 시프트 0이면 3750 min⁻¹ = 16,777,216이라 16비트를 넘습니다. 시프트 9면 32,768 = 3750 min⁻¹.</li>
            <li>deelook: 0.25 ms 서보와 주축을 함께 모을 때는 "종료할 때까지"만 허용합니다.</li>
          </ul>
        </div>
        <div>
          <h3>점 수 (datanum)</h3>
          <ul>
            <li>8 이상, 또는 -1 = 제한 없음 (cnc_sdtsetchnl, cnc_sdtsetchnl2 모두). deelook의 "종료할 때까지"가 -1입니다.</li>
            <li>수집하는 동안 계속 읽으면 제한이 없습니다. 다 모은 뒤 한꺼번에 읽는다면 채널 수에 따라 최대:</li>
          </ul>
          <table class="small">
            <thead><tr><th>채널</th>{#each MAX_POINTS as _, i (i)}<th class="num">{i + 1}</th>{/each}</tr></thead>
            <tbody><tr><td>최대 점</td>{#each MAX_POINTS as n, i (i)}<td class="num">{n.toLocaleString('ko-KR')}</td>{/each}</tr></tbody>
          </table>
          <h3>시작·읽기·끝</h3>
          <ul>
            <li>cnc_sdtstartsmpl(path 1~10, seq_no): 0이면 즉시, N이면 그 시퀀스 번호 블록이 실행될 때 시작.</li>
            <li>cnc_sdtreadsmpl의 stat: -1 시작 대기, 1 수집 중 (데이터 더 있음), 0 끝 → cnc_sdtendsmpl.</li>
            <li>읽기는 8워드 단위라 버퍼 크기는 8의 배수로. CNC 안의 버퍼가 차면 EW_BUFFER.</li>
            <li>cnc_sdtcancelsmpl: 시작 전·수집 중 언제든 취소. 읽기 오류 뒤에는 cnc_sdtendsmpl(2).</li>
            <li>cnc_sdtstartsmpl2는 바로 시작하므로 곧바로 cnc_sdtreadsmpl2로 읽기 시작합니다. trg의 STSM = 다 모음.</li>
            <li>cnc_sdtread1shot: cnc_sdtsetchnl로 설정한 채널(최대 8)의 지금 값.</li>
            <li>채널 수를 바꾸려면 먼저 cnc_sdtclrchnl.</li>
          </ul>
        </div>
      </div>
    {:else if tab === 'errors'}
      <p class="hint">EW_DATA·EW_REJECT의 숫자는 cnc_getdtailerr가 주는 상세 번호(detail)입니다. deelook의 오류 메시지에는 이 설명이 함께 붙습니다.</p>
      <div class="scroll">
        <table>
          <thead><tr><th>코드</th><th>detail</th><th>함수</th><th>뜻</th></tr></thead>
          <tbody>
            {#each ERROR_TABLE as e, i (i)}
              <tr><td><code>{e.code}</code></td><td class="num">{e.detail ?? ''}</td><td class="fns">{e.fns}</td><td>{e.text}</td></tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else if tab === 'cncs'}
      <table class="small">
        <thead><tr><th>CNC (M·T 계열)</th><th>cnc_sdtsetchnl 계열 (기본)</th><th>cnc_sdtsetchnl2 계열 (확장)</th></tr></thead>
        <tbody>
          {#each SUPPORTED as c (c.cnc)}
            <tr><td>{c.cnc}</td><td>{c.basic ? '가능' : '불가'}</td><td>{c.ext ? '가능' : '불가'}</td></tr>
          {/each}
        </tbody>
      </table>
      <ul>
        <li>0i-A/B/C, 15/15i, 16/18/21, 16i/18i/21i, Power Mate i, 펀치·레이저·와이어 계열은 쓸 수 없습니다.</li>
        <li>Ethernet·HSSB 모두 됩니다. 0i-D/F·30i·PMi-A의 내장 이더넷은 별도 옵션이 필요 없습니다.</li>
        <li>FOCAS2 라이브러리: Windows Fwlib64에는 두 계열 다, 리눅스 32비트(J555) 라이브러리에는 기본 계열만 있습니다.</li>
      </ul>
    {:else}
      <ul class="measured">
        <li><b>PMC 비트 채널은 맨 마지막이어야 합니다.</b> 앞에 두면 뒤 채널 데이터가 8워드마다 2워드씩 밀리고, 그 값이 trg로 새어 나와
          꾸준히 1인 신호가 SEQ로 읽힙니다 (30i-B·NC Guide, Fwlib64 6.9·7.3). deelook은 자동으로 마지막에 보냅니다.</li>
        <li>trg 비트 위치: SEQ 0x0001, P_UP 0x0002, P_DW 0x0004, STSM 0x8000. 끝난 뒤의 읽기는 0x8080.</li>
        <li>시작할 때 이미 1인 신호에는 P_UP이 찍히지 않습니다. 늘 1인 신호로 31i 벤치·NC Guide에서 각 10번 해 봐도 0번이었고, 0.5초마다 바뀌는 R9091.6은 모든 변화가 찍혔습니다. 점 수를 정한 수집 끝의 PMC 0점들도 P_DW로 찍히지 않습니다.</li>
        <li>블록 N 책갈피(트리거)는 N 번호 채널이 바뀌기 몇 ms 전에 찍힙니다 (N 번호는 4 ms마다 갱신).</li>
        <li>NC Guide 0i-F는 서보 0~46번·주축 0~21번 모든 종류를 받고 값은 모두 0입니다. 어떤 종류가 되는지는 실제 CNC에서 봐야 합니다.</li>
        <li>점 수를 정한 확장 수집이 끝나면 PMC·CNC 정보 채널의 마지막 몇 점(4~8점)이 0으로 옵니다. deelook은 CNC에 16점 더 요청하고 끝의 16점을 버려서, 저장되는 데이터에는 이 0이 없습니다(datanum = 정한 점 수 + 16).</li>
        <li>CNC 정보 채널(O·N 번호)은 수집을 시작한 뒤 <b>다음 블록이 시작될 때까지 0</b>으로 옵니다. 그 전부터 실행 중이던 블록의 번호는 나오지 않습니다. 1초짜리 블록이면 0~1초, 블록이 길면(긴 절삭, 휴지) 그만큼 깁니다. 프로그램이 돌지 않으면 계속 0입니다. PMC 채널은 그렇지 않습니다.</li>
        <li>cnc_sdtreadsmpl2 오류 뒤 cnc_sdtendsmpl2를 안 부르면 CNC 샘플러가 멈춰 NC Guide가 새 연결까지 거부했습니다.</li>
        <li>한 번 읽기에 오는 양은 모든 채널 합쳐 약 650~690워드(1.3 KB). 인터넷 너머 31i 벤치: 1 ms 3채널은 따라가고,
          0.25 ms 서보 2 + 주축 1은 초당 약 3000~3600점으로 4000에 못 미칩니다.</li>
        <li>31i 벤치는 PMC 바이트·워드에 EW_VERSION을 줍니다. NC Guide는 서보 값이 0이고 초당 약 1300점(시뮬레이터 시간)입니다.</li>
      </ul>
    {/if}
  </details>
</section>

<style>
  .card {
    background: #252525;
    border: 1px solid #333;
    border-radius: 6px;
    padding: 12px 14px;
    min-width: 0;
    font-size: 13px;
  }
  summary {
    cursor: pointer;
    display: flex;
    align-items: baseline;
    gap: 10px;
    flex-wrap: wrap;
  }
  h2 {
    display: inline;
    margin: 0;
    font-size: 15px;
    color: var(--text-primary);
  }
  h3 {
    margin: 12px 0 4px;
    font-size: 13px;
    color: var(--text-primary);
  }
  .tabs {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin: 10px 0 8px;
  }
  .tabs button {
    font: inherit;
    font-size: 12px;
    padding: 4px 10px;
    border-radius: 4px;
    border: 1px solid #444;
    background: #1f1f1e;
    color: var(--text-secondary);
    cursor: pointer;
  }
  .tabs button.on {
    background: #1f3b63;
    border-color: #2f5f9f;
    color: var(--text-primary);
  }
  .tools {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    align-items: center;
    margin-bottom: 6px;
  }
  input[type='search'] {
    font: inherit;
    color: var(--text-primary);
    background: #1a1a19;
    border: 1px solid #444;
    border-radius: 4px;
    padding: 3px 8px;
    width: 220px;
    max-width: 100%;
  }
  .scroll {
    overflow: auto;
    max-height: 460px;
  }
  table {
    border-collapse: collapse;
    width: 100%;
  }
  table.small {
    width: auto;
  }
  th {
    text-align: left;
    font-weight: 500;
    color: var(--text-secondary);
    padding: 4px 6px;
    border-bottom: 1px solid #3a3a38;
    position: sticky;
    top: 0;
    background: #252525;
    white-space: nowrap;
  }
  td {
    padding: 4px 6px;
    border-bottom: 1px solid #2f2f2e;
    vertical-align: top;
  }
  .num {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  .types td:nth-child(3) {
    min-width: 180px;
  }
  .types td:nth-child(4) {
    min-width: 220px;
  }
  .en,
  .when,
  .note,
  .fns {
    color: var(--text-secondary);
    font-size: 12px;
  }
  .note {
    min-width: 140px;
  }
  .unit + .unit {
    margin-top: 2px;
  }
  button.add {
    font: inherit;
    padding: 0 8px;
    border-radius: 4px;
    border: 1px solid #555;
    background: #2d2d2d;
    color: var(--text-primary);
    cursor: pointer;
  }
  button.add:disabled {
    opacity: 0.35;
    cursor: default;
  }
  .cols2 {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 16px;
  }
  @media (max-width: 900px) {
    .cols2 {
      grid-template-columns: minmax(0, 1fr);
    }
  }
  ul {
    margin: 4px 0;
    padding-left: 18px;
  }
  li {
    margin: 3px 0;
  }
  .hint {
    color: var(--text-secondary);
    font-size: 12px;
  }
  code {
    background: #1a1a19;
    padding: 0 4px;
    border-radius: 3px;
  }
</style>
