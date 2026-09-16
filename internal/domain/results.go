package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

type CandleIndex uint32

type ExtremumIndex uint32

const (
	WilderATRVersion       = "wilder_atr_v1"
	NATRVersion            = "wilder_natr_v1"
	LocalExtremaVersion    = "local_extrema_v1"
	PercentReversalVersion = "reversal_percent_v1"
	ATRReversalVersion     = "reversal_atr_v1"
	TrendVersion           = "swing_structure_v1"
	ZonesVersion           = "pivot_zones_v1"
)

type ATRResult struct {
	Value       decimal.Decimal
	CandleIndex CandleIndex
	ValueTime   time.Time
}

type NATRResult struct {
	Value          decimal.Decimal
	ATR            decimal.Decimal
	ReferenceClose decimal.Decimal
	CandleIndex    CandleIndex
	ValueTime      time.Time
}

type ExtremumKind string

const (
	High ExtremumKind = "HIGH"
	Low  ExtremumKind = "LOW"
)

type ReversalEvidence struct {
	Threshold         decimal.Decimal
	ConfirmationPrice decimal.Decimal
	CandidateATR      *decimal.Decimal
}

type Extremum struct {
	Kind                    ExtremumKind
	CandleIndex             CandleIndex
	Time                    time.Time
	Price                   decimal.Decimal
	ConfirmationCandleIndex CandleIndex
	ConfirmationTime        time.Time
	Reversal                *ReversalEvidence
}

type ExtremaResult struct{ Points []Extremum }

type TrendState string

type TrendReason string

const (
	Up                    TrendState  = "UP"
	Down                  TrendState  = "DOWN"
	Sideways              TrendState  = "SIDEWAYS"
	Undetermined          TrendState  = "UNDETERMINED"
	FlatRange             TrendReason = "flat_range"
	InsufficientStructure TrendReason = "insufficient_structure"
	RisingStructure       TrendReason = "rising_structure"
	FallingStructure      TrendReason = "falling_structure"
	StructureBroken       TrendReason = "structure_broken"
	HorizontalStructure   TrendReason = "horizontal_structure"
	MixedStructure        TrendReason = "mixed_structure"
)

type TrendResult struct {
	State          TrendState
	Reason         TrendReason
	Tolerance      decimal.Decimal
	ReferenceClose decimal.Decimal
	Extrema        ExtremaResult
}

type ZoneRole string

const (
	Support    ZoneRole = "SUPPORT"
	Resistance ZoneRole = "RESISTANCE"
	AtPrice    ZoneRole = "AT_PRICE"
)

type PriceZone struct {
	LowerBound            decimal.Decimal
	UpperBound            decimal.Decimal
	RepresentativePrice   decimal.Decimal
	Role                  ZoneRole
	ExtremumIndices       []ExtremumIndex
	AcceptedCandleIndices []CandleIndex
	TouchCount            uint32
	FirstTouchTime        time.Time
	LastTouchTime         time.Time
}

type LevelsResult struct {
	Zones            []PriceZone
	Extrema          ExtremaResult
	ATR              ATRResult
	MaximumZoneWidth decimal.Decimal
	ReferenceClose   decimal.Decimal
}
