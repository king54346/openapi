package cron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule 解析后的标准 5 字段 cron 表达式（分 时 日 月 周）。
// 每个字段支持 *、n、a-b、*/s、a-b/s 以及逗号组合；周字段 0 和 7 都表示周日。
type Schedule struct {
	minute, hour, dom, month, dow uint64 // 位图：第 n 位为 1 表示 n 命中
	domStar, dowStar              bool   // 日 / 周字段是否为 *
}

var fieldRanges = [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 7}}

// Parse 解析 cron 表达式。
func Parse(expr string) (*Schedule, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression must have 5 fields, got %d", len(fields))
	}
	var bits [5]uint64
	for i, f := range fields {
		b, err := parseField(f, fieldRanges[i][0], fieldRanges[i][1])
		if err != nil {
			return nil, fmt.Errorf("field %d (%q): %w", i+1, f, err)
		}
		bits[i] = b
	}
	// 周日可写成 0 或 7，统一到 0
	if bits[4]&(1<<7) != 0 {
		bits[4] = bits[4]&^(1<<7) | 1
	}
	return &Schedule{
		minute: bits[0], hour: bits[1], dom: bits[2], month: bits[3], dow: bits[4],
		domStar: fields[2] == "*", dowStar: fields[4] == "*",
	}, nil
}

// ValidateCron 表达式是否合法。
func ValidateCron(expr string) bool {
	_, err := Parse(expr)
	return err == nil
}

// Match 时间 t（精确到分钟）是否命中。
// 与标准 cron 一致：日和周都不是 * 时，两者满足其一即可。
func (s *Schedule) Match(t time.Time) bool {
	if s.minute&(1<<t.Minute()) == 0 || s.hour&(1<<t.Hour()) == 0 || s.month&(1<<int(t.Month())) == 0 {
		return false
	}
	domHit := s.dom&(1<<t.Day()) != 0
	dowHit := s.dow&(1<<int(t.Weekday())) != 0
	if !s.domStar && !s.dowStar {
		return domHit || dowHit
	}
	return domHit && dowHit
}

func parseField(field string, min, max int) (uint64, error) {
	var bits uint64
	for _, part := range strings.Split(field, ",") {
		if part == "" {
			return 0, errors.New("empty item")
		}
		rangePart, step := part, 1
		if i := strings.Index(part, "/"); i >= 0 {
			n, err := strconv.Atoi(part[i+1:])
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("invalid step in %q", part)
			}
			rangePart, step = part[:i], n
		}
		lo, hi := min, max
		switch {
		case rangePart == "*":
		case strings.Contains(rangePart, "-"):
			i := strings.Index(rangePart, "-")
			a, err1 := strconv.Atoi(rangePart[:i])
			b, err2 := strconv.Atoi(rangePart[i+1:])
			if err1 != nil || err2 != nil || a > b {
				return 0, fmt.Errorf("invalid range %q", rangePart)
			}
			lo, hi = a, b
		default:
			n, err := strconv.Atoi(rangePart)
			if err != nil {
				return 0, fmt.Errorf("invalid value %q", rangePart)
			}
			lo, hi = n, n
			if step > 1 { // n/s 表示从 n 开始到最大值，每 s 一次
				hi = max
			}
		}
		if lo < min || hi > max {
			return 0, fmt.Errorf("value out of range [%d,%d] in %q", min, max, part)
		}
		for v := lo; v <= hi; v += step {
			bits |= 1 << v
		}
	}
	return bits, nil
}
