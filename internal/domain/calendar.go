package domain

import (
	"time"
	"unicode"
	"unicode/utf8"
)

type Instrument struct {
	Exchange string
	Market   string
	Symbol   string
}

func (i Instrument) Validate() error {
	if i.Exchange != "binance" && i.Exchange != "bybit" {
		return invalid("exchange", "unsupported exchange")
	}

	if i.Market != "spot" && i.Market != "linear" {
		return invalid("market", "unsupported market")
	}

	if len(i.Symbol) == 0 || len(i.Symbol) > 128 || !utf8.ValidString(i.Symbol) {
		return invalid("symbol", "must contain 1 to 128 UTF-8 bytes")
	}

	for _, r := range i.Symbol {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return invalid("symbol", "must not contain whitespace or control characters")
		}
	}

	return nil
}

type Interval string

func (i Interval) Validate(instrument Instrument) error {
	if err := instrument.Validate(); err != nil {
		return err
	}

	if _, _, ok := i.slot(); !ok {
		return invalid("interval", "unsupported interval")
	}

	if (i == "1s" && (instrument.Exchange != "binance" || instrument.Market != "spot")) ||
		((i == "8h" || i == "3d") && instrument.Exchange != "binance") {
		return invalid("interval", "unsupported exchange and market combination")
	}

	return nil
}

func (i Interval) slot() (seconds, anchor int64, ok bool) {
	switch i {
	case "1s":
		return 1, 0, true
	case "1m":
		return 60, 0, true
	case "3m":
		return 180, 0, true
	case "5m":
		return 300, 0, true
	case "15m":
		return 900, 0, true
	case "30m":
		return 1800, 0, true
	case "1h":
		return 3600, 0, true
	case "2h":
		return 7200, 0, true
	case "4h":
		return 14400, 0, true
	case "6h":
		return 21600, 0, true
	case "8h":
		return 28800, 0, true
	case "12h":
		return 43200, 0, true
	case "1d":
		return 86400, 0, true
	case "3d":
		return 259200, 86400, true
	case "1w":
		return 604800, 345600, true
	case "1M":
		return 0, 0, true
	default:
		return 0, 0, false
	}
}

const maxTimestampSeconds int64 = 253402300799

func validateTime(field string, value time.Time) *ValidationError {
	value = value.UTC()
	if value.Year() < 1970 || value.Year() > 9999 || value.Unix() < 0 || value.Unix() > maxTimestampSeconds {
		return invalid(field, "must be between the Unix epoch and year 9999")
	}

	return nil
}

func (i Interval) Floor(value time.Time) (time.Time, error) {
	if err := validateTime("time", value); err != nil {
		return time.Time{}, err
	}

	seconds, anchor, ok := i.slot()
	if !ok {
		return time.Time{}, invalid("interval", "unsupported interval")
	}

	value = value.UTC()
	if i == "1M" {
		return time.Date(value.Year(), value.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	}

	offset := (value.Unix() - anchor) % seconds
	if offset < 0 {
		offset += seconds
	}

	result := time.Unix(value.Unix()-offset, 0).UTC()
	if err := validateTime("range", result); err != nil {
		return time.Time{}, err
	}

	return result, nil
}

// Shift moves an aligned boundary by calendar slots, without duration arithmetic.
func (i Interval) Shift(boundary time.Time, slots int64) (time.Time, error) {
	aligned, err := i.Floor(boundary)
	if err != nil {
		return time.Time{}, err
	}

	if !aligned.Equal(boundary) {
		return time.Time{}, invalid("range", "boundary must be aligned")
	}

	if i == "1M" {
		month := int64((aligned.Year()-1970)*12 + int(aligned.Month()) - 1)
		const lastMonth = (9999-1970)*12 + 11
		if slots < -month || slots > lastMonth-month {
			return time.Time{}, invalid("range", "calendar shift is outside supported timestamps")
		}

		month += slots
		return time.Date(1970+int(month/12), time.Month(month%12+1), 1, 0, 0, 0, 0, time.UTC), nil
	}

	seconds, _, _ := i.slot()
	unix := aligned.Unix()
	if slots < -(unix/seconds) || slots > (maxTimestampSeconds-unix)/seconds {
		return time.Time{}, invalid("range", "slot shift is outside supported timestamps")
	}

	return time.Unix(unix+slots*seconds, 0).UTC(), nil
}

type CandleRange struct {
	From time.Time
	To   time.Time
}

type CandleSelection struct {
	Instrument  Instrument
	Interval    Interval
	To          time.Time
	CandleCount uint32
}

func (s CandleSelection) Range() (CandleRange, error) {
	if err := s.Interval.Validate(s.Instrument); err != nil {
		return CandleRange{}, err
	}

	if s.CandleCount == 0 {
		return CandleRange{}, invalid("candle_count", "must be positive")
	}

	if err := validateTime("to", s.To); err != nil {
		return CandleRange{}, err
	}

	end, err := s.Interval.Floor(s.To)
	if err != nil {
		return CandleRange{}, err
	}

	start, err := s.Interval.Shift(end, -int64(s.CandleCount))
	if err != nil {
		return CandleRange{}, err
	}

	return CandleRange{From: start, To: end}, nil
}
