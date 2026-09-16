package domain

import (
	"math"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDivide(t *testing.T) {
	cases := []struct{ name, a, b, want string }{
		{"third", "11", "3", "3.666666666666667"},
		{"zero", "0", "3", "0"},
		{"tiny", "0.000000000000000011", "3", "0.000000000000000003666666666666667"},
		{"negative scale", "12345678901234567890", "1", "12345678901234570000"},
		{"half positive", "1.2345678901234565", "1", "1.234567890123457"},
		{"half negative", "-1.2345678901234565", "1", "-1.234567890123457"},
		{"negative divisor", "1.2345678901234565", "-1", "-1.234567890123457"},
		{"both negative", "-11", "-3", "3.666666666666667"},
		{"power boundary below", "0.99999999999999994", "1", "0.9999999999999999"},
		{"power boundary carry", "0.99999999999999995", "1", "1"},
		{"power boundary above", "1.0000000000000005", "1", "1.000000000000001"},
		{"coefficient lengths", "1", "300", "0.003333333333333333"},
		{"equal magnitude coefficients", "9", "8", "1.125"},
		{"maximum source integer", strings.Repeat("9", 1024), "3", strings.Repeat("3", 16) + strings.Repeat("0", 1008)},
		{"maximum source fraction", "0." + strings.Repeat("0", 1021) + "3", "3", "0." + strings.Repeat("0", 1021) + "1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			result, err := Divide(dec(tc.a), dec(tc.b))

			require.NoError(t, err)
			assert.Equal(t, tc.want, result.String())
		})
	}
}

func TestDivideRejectsZero(t *testing.T) {

	_, err := Divide(dec("1"), decimal.Zero)

	checkValidation(t, err, "denominator")
}

func TestDivideRejectsScaleOverflow(t *testing.T) {

	_, err := Divide(decimal.New(1, math.MinInt32), decimal.New(1, math.MaxInt32))

	checkValidation(t, err, "decimal")
}

func TestFormatting(t *testing.T) {
	cases := []struct{ value, number, percent string }{
		{"3.666666666666667", "3.6666667", "3.666667"},
		{"0", "0", "0"}, {"2.500000", "2.5", "2.5"},
		{"0.0000000012345678", "0.0000000012345678", "0"},
		{"1.23456785", "1.2345679", "1.234568"},
		{"-1.23456785", "-1.2345679", "-1.234568"},
		{"1.2345675", "1.2345675", "1.234568"},
		{"-1.2345675", "-1.2345675", "-1.234568"},
		{"12345678500", "12345679000", "12345678500"},
		{"-12345678500", "-12345679000", "-12345678500"},
		{"9.99999995", "10", "10"},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			value := dec(tc.value)

			result, err := FormatNumber(value)

			require.NoError(t, err)
			assert.Equal(t, tc.number, result)
			assert.Equal(t, tc.percent, FormatPercentage(value))
			assert.Equal(t, dec(tc.value), value)
		})
	}
}

func TestFormatRejectsScaleOverflow(t *testing.T) {

	_, err := FormatNumber(decimal.New(1, math.MinInt32))

	checkValidation(t, err, "decimal")
}

func TestExactOperationsAndRounding(t *testing.T) {
	assert.Equal(t, "0.3", dec("0.1").Add(dec("0.2")).String())
	assert.Equal(t, "0.1", dec("0.3").Sub(dec("0.2")).String())
	assert.Equal(t, "0.02", dec("0.1").Mul(dec("0.2")).String())
	assert.Equal(t, "1.24", dec("1.235").Round(2).String())
	assert.Equal(t, "-1.24", dec("-1.235").Round(2).String())
}

func TestOutputDoesNotChangeComparisons(t *testing.T) {
	a, b := dec("1.00000001"), dec("1.00000002")

	left, err := FormatNumber(a)

	require.NoError(t, err)

	right, err := FormatNumber(b)

	require.NoError(t, err)
	assert.Equal(t, left, right)
	assert.True(t, a.LessThan(b))
}

func TestConcurrentNumericalOperations(t *testing.T) {
	precision := decimal.DivisionPrecision
	t.Cleanup(func() { assert.Equal(t, precision, decimal.DivisionPrecision) })
	for i := 0; i < 16; i++ {
		t.Run("independent", func(t *testing.T) {
			t.Parallel()

			value, err := Divide(dec("11"), dec("3"))

			require.NoError(t, err)

			result, err := FormatNumber(value)

			require.NoError(t, err)
			assert.Equal(t, "3.6666667", result)
			assert.Equal(t, "3.666667", FormatPercentage(value))
		})
	}
}
