package domain

import (
	"math"
	"math/big"

	"github.com/shopspring/decimal"
)

const NumericPolicy = "decimal_sig16_output8_pct6_v1"

// Divide rounds each division to 16 significant digits, independent of global precision.
func Divide(numerator, denominator decimal.Decimal) (decimal.Decimal, error) {
	if denominator.IsZero() {
		return decimal.Zero, invalid("denominator", "must not be zero")
	}

	if numerator.IsZero() {
		return decimal.Zero, nil
	}

	a := new(big.Int).Abs(numerator.Coefficient())
	b := new(big.Int).Abs(denominator.Coefficient())
	digits := int64(len(a.String()) - len(b.String()))
	power := new(big.Int).Exp(big.NewInt(10), big.NewInt(absInt(digits)), nil)
	if digits >= 0 {
		b.Mul(b, power)
	} else {
		a.Mul(a, power)
	}

	magnitude := digits + int64(numerator.Exponent()) - int64(denominator.Exponent())
	if a.Cmp(b) < 0 {
		magnitude--
	}

	scale := int64(15) - magnitude
	// DivRound also computes the exponent difference plus the requested scale.
	shift := int64(numerator.Exponent()) - int64(denominator.Exponent()) + scale
	if !fitsScale(scale) || !fitsScale(shift) || !fitsScale(int64(numerator.Exponent())+scale) || !fitsScale(int64(denominator.Exponent())-scale) {
		return decimal.Zero, invalid("decimal", "division scale overflow")
	}

	return numerator.DivRound(denominator, int32(scale)), nil
}

func absInt(value int64) int64 {
	if value < 0 {
		return -value
	}

	return value
}

func fitsScale(value int64) bool {
	return value > math.MinInt32 && value < math.MaxInt32
}

func FormatNumber(value decimal.Decimal) (string, error) {
	if value.IsZero() {
		return "0", nil
	}

	magnitude := int64(len(new(big.Int).Abs(value.Coefficient()).String())-1) + int64(value.Exponent())
	scale := 7 - magnitude
	if !fitsScale(scale) || !fitsScale(scale+int64(value.Exponent())) {
		return "", invalid("decimal", "output scale overflow")
	}

	return value.Round(int32(scale)).String(), nil
}

func FormatPercentage(value decimal.Decimal) string {
	return value.Round(6).String()
}
