// Package sampling records data sampled by the CNC (FOCAS2 cnc_sdt*) into
// SQLite, one session per start: servo and spindle data, and with the
// cnc_sdt*2 calls PMC signals, CNC information and trigger marks.
package sampling

import "strings"

// DataType is one of the data a channel can carry, as the cnc_sdtsetchnl
// and cnc_sdtsetchnl2 references list them (revision 2026-03-24), with
// Korean names and notes for the browser.
type DataType struct {
	Type   int    `json:"type"`
	Code   string `json:"code"`
	Name   string `json:"name"`   // the reference's English name
	NameKo string `json:"nameKo"` // Korean name
	Units  []Unit `json:"units"`  // what the raw value means with shift 0
	Unit   string `json:"unit"`   // Units in one line, for chart captions
	Shift  int    `json:"shift"`  // right shift that fits the value into 16 bits, as the reference suggests
	Note   string `json:"note,omitempty"`
}

// Unit is one line of a data type's unit: Raw = Means, When a condition
// holds.
type Unit struct {
	Raw   string `json:"raw"`
	Means string `json:"means"`
	When  string `json:"when,omitempty"`
}

func u(raw, means string) Unit        { return Unit{Raw: raw, Means: means} }
func uw(raw, means, when string) Unit { return Unit{Raw: raw, Means: means, When: when} }

var (
	ampMax     = []Unit{u("7,282", "앰프 최대 전류(A)")}
	ampMax6554 = []Unit{u("6,554", "앰프 최대 전류(A)")}
	ampMax8027 = []Unit{u("8,027", "앰프 최대 전류(A)")}
	detect     = []Unit{u("1", "검출 단위 1 (1 펄스)")}
	internal   = []Unit{u("내부 단위", "공개되지 않음")}
	vcmd       = []Unit{u("16,777,216", "3750 min⁻¹"), uw("32,768", "3750 min⁻¹", "시프트 9일 때")}
	acc        = []Unit{uw("512", "0.1 G = 0.98 m/s²", "aiGS0.1-3D (0.1 G)"), uw("4,096", "1 G = 9.8 m/s²", "aiGS1-3D (1 G)")}
	algofs     = []Unit{uw("-32,000 … 0 … 32,000", "-10 V … 0 V … +10 V", "전압 입력 (AIM)"), uw("-32,768 … 0 … 32,767", "0 mA … 12.5 mA … 25 mA", "전류 입력 (AMU)")}
	maxTorque  = []Unit{u("16,384", "모터 최대 토크")}
	rpm4096    = []Unit{u("4,096", "1 min⁻¹")}
	sensorAD   = []Unit{u("65,536", "1.48 V")}
	csRev      = []Unit{uw("360,000 펄스", "1회전", "No.4005#0=0"), uw("3,600,000 펄스", "1회전", "No.4005#0=1")}
)

// ServoTypes are the data types of a servo axis channel (0..46).
var ServoTypes = []DataType{
	{Type: 0, Code: "POSF", Name: "Summation of position feedback pulse data", NameKo: "위치 피드백 펄스 누적값", Units: detect, Note: "CNC 전원을 켤 때 0에서 시작하는 누적값"},
	{Type: 1, Code: "ERR", Name: "Position error", NameKo: "위치 편차", Units: detect},
	{Type: 2, Code: "VCMD", Name: "Velocity command", NameKo: "속도 지령", Units: vcmd, Shift: 9},
	{Type: 3, Code: "TCMD", Name: "Torque command", NameKo: "토크 지령", Units: ampMax},
	{Type: 4, Code: "SPEED", Name: "Speed feedback signal", NameKo: "속도 피드백", Units: []Unit{u("262,144", "3750 min⁻¹"), uw("32,768", "3750 min⁻¹", "시프트 3일 때")}, Shift: 3},
	{Type: 5, Code: "DTRQ", Name: "Estimated disturbance torque", NameKo: "추정 외란 토크", Units: ampMax},
	{Type: 6, Code: "SYNC", Name: "Synchronous error on rigid tapping", NameKo: "리지드 탭 동기 오차", Units: []Unit{u("1", "주축 1/4096 회전")}},
	{Type: 7, Code: "OVCLV", Name: "OVC simulation data", NameKo: "OVC(과전류) 시뮬레이션", Units: []Unit{u("1", "1%")}},
	{Type: 8, Code: "IEFF", Name: "Effective current", NameKo: "실효 전류", Units: ampMax6554},
	{Type: 9, Code: "VDC", Name: "DC link voltage", NameKo: "DC 링크 전압", Units: []Unit{uw("8,192", "640 V", "200 V 계열"), uw("8,192", "1280 V", "400 V 계열")}},
	{Type: 10, Code: "SFERR", Name: "Semi-Full error", NameKo: "세미-풀 오차", Units: detect},
	{Type: 11, Code: "ALGCMD", Name: "Analog signal command", NameKo: "아날로그 신호 지령",
		Units: []Unit{u("32,767", "100% (아날로그 신호 지령 최대)"), uw("32,767", "최대 압력 지령값", "압력·위치 제어")}, Note: "압력·위치 제어에서는 압력 지령 (90BP만)"},
	{Type: 12, Code: "ALGFB", Name: "Analog signal feedback", NameKo: "아날로그 신호 피드백",
		Units: []Unit{u("32,767", "100% (아날로그 신호 피드백 최대)"), uw("32,767", "최대 압력 검출값", "압력·위치 제어")}, Note: "압력·위치 제어에서는 압력 피드백 (90BP만)"},
	{Type: 13, Code: "IR", Name: "R-phase current", NameKo: "R상 전류", Units: ampMax6554},
	{Type: 14, Code: "IR1", Name: "R-phase current (PDM slave 1)", NameKo: "R상 전류 (PDM 슬레이브 1)", Units: ampMax6554},
	{Type: 15, Code: "IR2", Name: "R-phase current (PDM slave 2)", NameKo: "R상 전류 (PDM 슬레이브 2)", Units: ampMax6554},
	{Type: 16, Code: "IR3", Name: "R-phase current (PDM slave 3)", NameKo: "R상 전류 (PDM 슬레이브 3)", Units: ampMax6554},
	{Type: 17, Code: "IR4", Name: "R-phase current (PDM slave 4)", NameKo: "R상 전류 (PDM 슬레이브 4)", Units: ampMax6554},
	{Type: 18, Code: "IS", Name: "S-phase current", NameKo: "S상 전류", Units: ampMax6554},
	{Type: 19, Code: "IS1", Name: "S-phase current (PDM slave 1)", NameKo: "S상 전류 (PDM 슬레이브 1)", Units: ampMax6554},
	{Type: 20, Code: "IS2", Name: "S-phase current (PDM slave 2)", NameKo: "S상 전류 (PDM 슬레이브 2)", Units: ampMax6554},
	{Type: 21, Code: "IS3", Name: "S-phase current (PDM slave 3)", NameKo: "S상 전류 (PDM 슬레이브 3)", Units: ampMax6554},
	{Type: 22, Code: "IS4", Name: "S-phase current (PDM slave 4)", NameKo: "S상 전류 (PDM 슬레이브 4)", Units: ampMax6554},
	{Type: 23, Code: "ID", Name: "Reactive current", NameKo: "무효 전류", Units: ampMax8027},
	{Type: 24, Code: "IQ", Name: "Active current", NameKo: "유효 전류", Units: ampMax8027},
	{Type: 25, Code: "MTTMP", Name: "Motor winding temperature", NameKo: "모터 권선 온도", Units: []Unit{u("1", "1 °C")}},
	{Type: 26, Code: "PCTMP", Name: "Pulsecoder temperature", NameKo: "펄스코더 온도", Units: []Unit{u("1", "1 °C")}},
	{Type: 27, Code: "VCOUT", Name: "Velocity command without FF", NameKo: "피드포워드를 뺀 속도 지령", Units: []Unit{u("VCMD와 같음", "")}, Shift: 9},
	{Type: 28, Code: "VCC0", Name: "Advanced velocity command", NameKo: "선행 속도 지령", Units: internal},
	{Type: 29, Code: "BLACL", Name: "Backlash acceleration amount", NameKo: "백래시 가속량", Units: internal},
	{Type: 30, Code: "ABS", Name: "Absolute position detected internal pulse coder", NameKo: "내장 펄스코더 절대 위치", Units: []Unit{u("1", "1/2²¹ 회전")}},
	{Type: 31, Code: "FREQ", Name: "Vibration frequency", NameKo: "진동 주파수", Units: []Unit{u("1", "1 Hz")}},
	{Type: 32, Code: "FRTCM", Name: "Vibration torque command", NameKo: "진동 토크 지령", Units: ampMax},
	{Type: 33, Code: "VDC", Name: "DC link voltage", NameKo: "DC 링크 전압", Units: []Unit{u("9번(VDC)과 같음", "")}},
	{Type: 34, Code: "ACC", Name: "Acceleration feedback in direction specified by No.2277#7,6,5", NameKo: "가속도 피드백 (No.2277#7,6,5로 정한 방향)", Units: acc},
	{Type: 35, Code: "ACC1", Name: "1st direction acceleration feedback", NameKo: "가속도 피드백 (1번째 방향)", Units: acc},
	{Type: 36, Code: "ACC2", Name: "2nd direction acceleration feedback", NameKo: "가속도 피드백 (2번째 방향)", Units: acc},
	{Type: 37, Code: "ACC3", Name: "3rd direction acceleration feedback", NameKo: "가속도 피드백 (3번째 방향)", Units: acc},
	{Type: 38, Code: "FRVCM", Name: "Vibration velocity command", NameKo: "진동 속도 지령", Units: []Unit{u("VCMD와 같음", "")}, Shift: 9},
	{Type: 39, Code: "BLCMP", Name: "Backlash compensation", NameKo: "백래시 보정량", Units: detect},
	{Type: 40, Code: "BLAC1", Name: "Backlash acceleration amount 1", NameKo: "백래시 가속량 1", Units: internal},
	{Type: 41, Code: "BLAC2", Name: "Backlash acceleration amount 2", NameKo: "백래시 가속량 2", Units: internal},
	{Type: 42, Code: "SMTBKL", Name: "Smart backlash compensation (position compensation amount)", NameKo: "스마트 백래시 보정 (위치 보정량)", Units: detect},
	{Type: 43, Code: "SMTTRQ", Name: "Estimated load torque for Smart backlash compensation", NameKo: "스마트 백래시 보정용 추정 부하 토크", Units: ampMax},
	{Type: 44, Code: "SMTBAC", Name: "Compensation amount of smart backlash (for full-closed)", NameKo: "스마트 백래시 보정량 (풀클로즈드용)", Units: detect},
	{Type: 45, Code: "ALGOFS", Name: "Analog signal offset", NameKo: "아날로그 신호 오프셋", Units: algofs},
	{Type: 46, Code: "ALGOF2", Name: "Analog signal offset 2", NameKo: "아날로그 신호 오프셋 2", Units: []Unit{u("ALGOFS(45번)와 같음", "")}},
}

// SpindleTypes are the data types of a spindle channel (0..21).
var SpindleTypes = []DataType{
	{Type: 0, Code: "CSPOS", Name: "Summation of position feedback pulse data", NameKo: "위치 피드백 펄스 누적값", Units: csRev, Note: "CNC 전원을 켤 때 0에서 시작하는 누적값"},
	{Type: 1, Code: "SPEED", Name: "Motor speed", NameKo: "모터 속도", Units: rpm4096},
	{Type: 2, Code: "TCMD", Name: "Torque command", NameKo: "토크 지령", Units: maxTorque},
	{Type: 3, Code: "VCMD", Name: "Motor speed command", NameKo: "모터 속도 지령", Units: rpm4096},
	{Type: 4, Code: "ERR", Name: "Position error", NameKo: "위치 편차",
		Units: []Unit{uw("360,000 펄스", "1회전", "Cs 윤곽 제어, No.4005#0=0"), uw("3,600,000 펄스", "1회전", "Cs 윤곽 제어, No.4005#0=1"), uw("4,096 펄스", "1회전", "Cs 이외")}},
	{Type: 5, Code: "LMDAT", Name: "Load meter data", NameKo: "로드미터", Units: []Unit{u("32,768", "10 V")}},
	{Type: 6, Code: "DTRQ", Name: "Spindle load torque", NameKo: "주축 부하 토크", Units: maxTorque},
	{Type: 7, Code: "INORM", Name: "Amplitude of motor current", NameKo: "모터 전류 진폭", Units: []Unit{u("모터·앰프 종류에 따라 다름", "")}},
	{Type: 8, Code: "MTTMP", Name: "Motor winding temperature", NameKo: "모터 권선 온도", Units: []Unit{u("1", "1 °C")}},
	{Type: 9, Code: "EPMTR", Name: "Spindle axis power consumption", NameKo: "주축 소비 전력",
		Units: []Unit{uw("1", "10 kW", "No.4541#2,#1 = 0,0"), uw("1", "1 kW", "No.4541#2,#1 = 0,1"), uw("1", "0.1 kW", "No.4541#2,#1 = 1,0"), uw("1", "0.01 kW", "No.4541#2,#1 = 1,1")},
		Note:  "소비 전력 모니터 기능을 설정해야 나옴 (No.4541#0=1 등, 사양서 A-92737E 등 참고). 주축 소프트웨어 9D80/T(20) 이후"},
	{Type: 10, Code: "FREQ", Name: "Frequency of disturbance torque", NameKo: "외란 토크 주파수", Units: []Unit{u("1", "1 Hz")}},
	{Type: 11, Code: "GAIN", Name: "Gain data", NameKo: "게인", Units: []Unit{u("4,096", "100%")}},
	{Type: 12, Code: "TCMD2", Name: "Torque command 2", NameKo: "토크 지령 2", Units: maxTorque},
	{Type: 13, Code: "PA1", Name: "A/D data of motor sensor (A phase)", NameKo: "모터 센서 A/D (A상)", Units: sensorAD},
	{Type: 14, Code: "PB1", Name: "A/D data of motor sensor (B phase)", NameKo: "모터 센서 A/D (B상)", Units: sensorAD},
	{Type: 15, Code: "PA2", Name: "A/D data of spindle sensor (A phase)", NameKo: "주축 센서 A/D (A상)", Units: sensorAD},
	{Type: 16, Code: "PB2", Name: "A/D data of spindle sensor (B phase)", NameKo: "주축 센서 A/D (B상)", Units: sensorAD},
	{Type: 17, Code: "VERR", Name: "Velocity error", NameKo: "속도 오차", Units: rpm4096},
	{Type: 18, Code: "SPSPD", Name: "Spindle speed", NameKo: "주축 속도", Units: rpm4096},
	{Type: 19, Code: "LMDTC", Name: "Present value of loadmeter (normalized by cont. rated output)", NameKo: "로드미터 현재값 (연속 정격 출력 기준)", Units: []Unit{u("1", "1% (100% = 연속 정격 출력)")}},
	{Type: 20, Code: "LMMAX", Name: "Maximum value of loadmeter at the present speed (normalized by cont. rated output)", NameKo: "지금 속도에서의 로드미터 최대값 (연속 정격 출력 기준)", Units: []Unit{u("1", "1% (100% = 연속 정격 출력)")}},
	{Type: 21, Code: "DURTM", Name: "Duration time for cutting", NameKo: "절삭 지속 시간", Units: []Unit{u("1", "1 초")}},
}

// CncInfoTypes are what a CNC information channel (cnc_sdtsetchnl2) carries,
// sampled every 4 ms (repeated to fill the channel's period).
var CncInfoTypes = []DataType{
	{Type: 1, Code: "ONO", Name: "Program number (O number)", NameKo: "프로그램 번호 (O 번호)", Units: []Unit{u("값", "번호 그대로")}, Note: "4 ms마다 갱신 (1 ms 채널이면 같은 값 4개). 수집을 시작하면 다음 블록이 시작될 때까지 0"},
	{Type: 2, Code: "NNO", Name: "Sequence number (N number)", NameKo: "시퀀스 번호 (N 번호)", Units: []Unit{u("값", "번호 그대로")}, Note: "4 ms마다 갱신 (1 ms 채널이면 같은 값 4개). 수집을 시작하면 다음 블록이 시작될 때까지 0"},
}

func init() {
	for _, list := range [][]DataType{ServoTypes, SpindleTypes, CncInfoTypes} {
		for i := range list {
			d := &list[i]
			var parts []string
			for _, x := range d.Units {
				s := x.Raw
				if x.Means != "" {
					s += " = " + x.Means
				}
				if x.When != "" {
					s += " (" + x.When + ")"
				}
				parts = append(parts, s)
			}
			d.Unit = strings.Join(parts, ", ")
		}
	}
}

func findType(kind string, t int) (DataType, bool) {
	var list []DataType
	switch kind {
	case "servo":
		list = ServoTypes
	case "spindle":
		list = SpindleTypes
	case "cnc":
		list = CncInfoTypes
	}
	for _, d := range list {
		if d.Type == t {
			return d, true
		}
	}
	return DataType{}, false
}
