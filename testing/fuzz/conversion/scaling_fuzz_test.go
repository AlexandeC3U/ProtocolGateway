//go:build fuzz
// +build fuzz

// Package conversion provides additional fuzz tests for data scaling operations.
package conversion

import (
	"math"
	"testing"
)

// =============================================================================
// Extended Scaling Functions
// =============================================================================

// clampedApplyScaling applies linear scaling with overflow protection.
func clampedApplyScaling(value float64, scaleFactor, offset float64) float64 {
	result := (value * scaleFactor) + offset

	// Clamp to prevent extreme values
	if result > math.MaxFloat64/2 {
		return math.MaxFloat64 / 2
	}
	if result < -math.MaxFloat64/2 {
		return -math.MaxFloat64 / 2
	}
	return result
}

// clampedReverseScaling reverses linear scaling with zero protection.
func clampedReverseScaling(value float64, scaleFactor, offset float64) float64 {
	if scaleFactor == 0 {
		return 0
	}
	// Handle very small scale factors that could cause overflow
	if math.Abs(scaleFactor) < 1e-300 {
		return 0
	}
	return (value - offset) / scaleFactor
}

// rangeScale maps value from one range to another: [inMin, inMax] -> [outMin, outMax]
func rangeScale(value, inMin, inMax, outMin, outMax float64) float64 {
	if inMax == inMin {
		return outMin // Avoid division by zero
	}
	return ((value-inMin)/(inMax-inMin))*(outMax-outMin) + outMin
}

// reverseRangeScale reverses range mapping.
func reverseRangeScale(value, inMin, inMax, outMin, outMax float64) float64 {
	if outMax == outMin {
		return inMin
	}
	return ((value-outMin)/(outMax-outMin))*(inMax-inMin) + inMin
}

// percentScale converts value to percentage of range.
func percentScale(value, min, max float64) float64 {
	if max == min {
		return 0
	}
	return ((value - min) / (max - min)) * 100
}

// fromPercentScale converts percentage back to value in range.
func fromPercentScale(percent, min, max float64) float64 {
	return (percent/100)*(max-min) + min
}

// =============================================================================
// Fuzz Tests for Clamped Scaling
// =============================================================================

// FuzzClampedScalingRoundtrip fuzzes clamped scaling roundtrip.
func FuzzClampedScalingRoundtrip(f *testing.F) {
	// Seed corpus with various cases
	f.Add(float64(100), float64(1.0), float64(0))
	f.Add(float64(100), float64(0.1), float64(0))
	f.Add(float64(100), float64(0.001), float64(100))
	f.Add(float64(0), float64(1.0), float64(0))
	f.Add(float64(-100), float64(2.0), float64(-50))
	f.Add(float64(1e10), float64(1e5), float64(1e10))
	f.Add(float64(-1e10), float64(1e-5), float64(-1e10))
	f.Add(float64(1e-10), float64(1e10), float64(0))

	f.Fuzz(func(t *testing.T, rawValue, scaleFactor, offset float64) {
		// Skip NaN and Inf inputs
		if math.IsNaN(rawValue) || math.IsInf(rawValue, 0) {
			return
		}
		if math.IsNaN(scaleFactor) || math.IsInf(scaleFactor, 0) || scaleFactor == 0 {
			return
		}
		if math.IsNaN(offset) || math.IsInf(offset, 0) {
			return
		}

		// Apply clamped scaling
		scaled := clampedApplyScaling(rawValue, scaleFactor, offset)

		// Result should not be NaN or Inf
		if math.IsNaN(scaled) {
			t.Errorf("clamped scaling produced NaN: raw=%v, scale=%v, offset=%v",
				rawValue, scaleFactor, offset)
			return
		}
		if math.IsInf(scaled, 0) {
			t.Errorf("clamped scaling produced Inf: raw=%v, scale=%v, offset=%v",
				rawValue, scaleFactor, offset)
			return
		}

		// Reverse scaling
		unscaled := clampedReverseScaling(scaled, scaleFactor, offset)

		// Skip clamped values for roundtrip check
		if scaled == math.MaxFloat64/2 || scaled == -math.MaxFloat64/2 {
			return
		}

		// Check roundtrip (with floating point tolerance)
		if !math.IsNaN(unscaled) && !math.IsInf(unscaled, 0) {
			diff := math.Abs(unscaled - rawValue)
			tolerance := math.Max(math.Abs(rawValue)*1e-9, 1e-9)

			if diff > tolerance {
				t.Errorf("clamped scaling roundtrip failed: raw=%v, scale=%v, offset=%v, scaled=%v, unscaled=%v, diff=%v",
					rawValue, scaleFactor, offset, scaled, unscaled, diff)
			}
		}
	})
}

// =============================================================================
// Fuzz Tests for Range Scaling
// =============================================================================

// FuzzRangeScaleRoundtrip fuzzes range scaling roundtrip.
func FuzzRangeScaleRoundtrip(f *testing.F) {
	// Seed corpus: value, inMin, inMax, outMin, outMax
	f.Add(float64(50), float64(0), float64(100), float64(0), float64(1000))
	f.Add(float64(0), float64(0), float64(100), float64(0), float64(1))
	f.Add(float64(100), float64(0), float64(100), float64(-50), float64(50))
	f.Add(float64(25), float64(0), float64(100), float64(32), float64(212)) // Celsius to Fahrenheit range
	f.Add(float64(-10), float64(-100), float64(100), float64(0), float64(1))

	f.Fuzz(func(t *testing.T, value, inMin, inMax, outMin, outMax float64) {
		// Skip invalid inputs
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return
		}
		if math.IsNaN(inMin) || math.IsInf(inMin, 0) {
			return
		}
		if math.IsNaN(inMax) || math.IsInf(inMax, 0) {
			return
		}
		if math.IsNaN(outMin) || math.IsInf(outMin, 0) {
			return
		}
		if math.IsNaN(outMax) || math.IsInf(outMax, 0) {
			return
		}

		// Skip degenerate ranges
		if inMax == inMin || outMax == outMin {
			return
		}

		// Apply range scaling
		scaled := rangeScale(value, inMin, inMax, outMin, outMax)

		// Skip if result is invalid
		if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
			return
		}

		// Reverse range scaling
		unscaled := reverseRangeScale(scaled, inMin, inMax, outMin, outMax)

		// Check roundtrip
		if !math.IsNaN(unscaled) && !math.IsInf(unscaled, 0) {
			diff := math.Abs(unscaled - value)
			tolerance := math.Max(math.Abs(value)*1e-9, 1e-9)

			if diff > tolerance {
				t.Errorf("range scale roundtrip failed: value=%v, inRange=[%v,%v], outRange=[%v,%v], scaled=%v, unscaled=%v",
					value, inMin, inMax, outMin, outMax, scaled, unscaled)
			}
		}
	})
}

// FuzzRangeScaleBoundaries tests that range scaling preserves boundaries.
func FuzzRangeScaleBoundaries(f *testing.F) {
	f.Add(float64(0), float64(100), float64(-50), float64(50))
	f.Add(float64(-100), float64(100), float64(0), float64(1000))
	f.Add(float64(0), float64(65535), float64(0), float64(100))

	f.Fuzz(func(t *testing.T, inMin, inMax, outMin, outMax float64) {
		// Skip invalid inputs
		for _, v := range []float64{inMin, inMax, outMin, outMax} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return
			}
		}
		if inMax == inMin {
			return
		}

		// inMin should map to outMin
		scaledMin := rangeScale(inMin, inMin, inMax, outMin, outMax)
		if !almostEqual(scaledMin, outMin, 1e-9) {
			t.Errorf("inMin didn't map to outMin: rangeScale(%v, %v, %v, %v, %v) = %v, expected %v",
				inMin, inMin, inMax, outMin, outMax, scaledMin, outMin)
		}

		// inMax should map to outMax
		scaledMax := rangeScale(inMax, inMin, inMax, outMin, outMax)
		if !almostEqual(scaledMax, outMax, 1e-9) {
			t.Errorf("inMax didn't map to outMax: rangeScale(%v, %v, %v, %v, %v) = %v, expected %v",
				inMax, inMin, inMax, outMin, outMax, scaledMax, outMax)
		}
	})
}

// =============================================================================
// Fuzz Tests for Percent Scaling
// =============================================================================

// FuzzPercentScaleRoundtrip fuzzes percent scaling roundtrip.
func FuzzPercentScaleRoundtrip(f *testing.F) {
	f.Add(float64(50), float64(0), float64(100))
	f.Add(float64(25), float64(0), float64(200))
	f.Add(float64(-10), float64(-100), float64(100))
	f.Add(float64(75), float64(0), float64(65535))
	f.Add(float64(0), float64(0), float64(4095)) // 12-bit ADC

	f.Fuzz(func(t *testing.T, value, min, max float64) {
		// Skip invalid inputs
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return
		}
		if math.IsNaN(min) || math.IsInf(min, 0) {
			return
		}
		if math.IsNaN(max) || math.IsInf(max, 0) {
			return
		}
		if max == min {
			return
		}

		// Convert to percent
		percent := percentScale(value, min, max)

		// Skip invalid results
		if math.IsNaN(percent) || math.IsInf(percent, 0) {
			return
		}

		// Convert back
		unscaled := fromPercentScale(percent, min, max)

		// Check roundtrip
		if !math.IsNaN(unscaled) && !math.IsInf(unscaled, 0) {
			diff := math.Abs(unscaled - value)
			tolerance := math.Max(math.Abs(value)*1e-9, 1e-9)

			if diff > tolerance {
				t.Errorf("percent scale roundtrip failed: value=%v, range=[%v,%v], percent=%v, unscaled=%v",
					value, min, max, percent, unscaled)
			}
		}
	})
}

// FuzzPercentScaleBoundaries tests percent scaling boundary values.
func FuzzPercentScaleBoundaries(f *testing.F) {
	f.Add(float64(0), float64(100))
	f.Add(float64(-100), float64(100))
	f.Add(float64(0), float64(65535))
	f.Add(float64(0), float64(4095))

	f.Fuzz(func(t *testing.T, min, max float64) {
		if math.IsNaN(min) || math.IsInf(min, 0) {
			return
		}
		if math.IsNaN(max) || math.IsInf(max, 0) {
			return
		}
		if max == min {
			return
		}

		// min should map to 0%
		p0 := percentScale(min, min, max)
		if !almostEqual(p0, 0, 1e-9) {
			t.Errorf("min didn't map to 0%%: percentScale(%v, %v, %v) = %v", min, min, max, p0)
		}

		// max should map to 100%
		p100 := percentScale(max, min, max)
		if !almostEqual(p100, 100, 1e-9) {
			t.Errorf("max didn't map to 100%%: percentScale(%v, %v, %v) = %v", max, min, max, p100)
		}

		// midpoint should map to 50%
		mid := (min + max) / 2
		p50 := percentScale(mid, min, max)
		if !almostEqual(p50, 50, 1e-9) {
			t.Errorf("mid didn't map to 50%%: percentScale(%v, %v, %v) = %v", mid, min, max, p50)
		}
	})
}

// =============================================================================
// Fuzz Tests for Scaling Edge Cases
// =============================================================================

// FuzzScalingOverflow tests scaling with values that could cause overflow.
func FuzzScalingOverflow(f *testing.F) {
	f.Add(float64(1e300), float64(1e10))
	f.Add(float64(-1e300), float64(1e10))
	f.Add(float64(1e-300), float64(1e-10))
	f.Add(float64(1e100), float64(1e100))

	f.Fuzz(func(t *testing.T, value, scaleFactor float64) {
		if math.IsNaN(value) || math.IsNaN(scaleFactor) {
			return
		}
		if scaleFactor == 0 {
			return
		}

		// This should not panic
		result := clampedApplyScaling(value, scaleFactor, 0)

		// Result should be a valid number (clamped, not Inf)
		if math.IsNaN(result) {
			t.Errorf("clamped scaling produced NaN for value=%v, scale=%v", value, scaleFactor)
		}
	})
}

// FuzzScalingPrecision tests scaling precision with small values.
func FuzzScalingPrecision(f *testing.F) {
	f.Add(float64(1e-10), float64(1e-5), float64(0))
	f.Add(float64(1e-15), float64(1.0), float64(1e-15))
	f.Add(float64(0.0001), float64(0.0001), float64(0.0001))

	f.Fuzz(func(t *testing.T, value, scaleFactor, offset float64) {
		// Only test small positive values
		if value <= 0 || value > 1 {
			return
		}
		if scaleFactor <= 0 || scaleFactor > 1 {
			return
		}
		if math.Abs(offset) > 1 {
			return
		}

		scaled := applyScaling(value, scaleFactor, offset)
		if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
			return
		}

		unscaled := reverseScaling(scaled, scaleFactor, offset)

		// For small values, use relative tolerance
		relDiff := math.Abs(unscaled-value) / math.Max(math.Abs(value), 1e-15)
		if relDiff > 1e-6 {
			t.Errorf("precision loss in small value scaling: value=%v, scale=%v, offset=%v, relDiff=%v",
				value, scaleFactor, offset, relDiff)
		}
	})
}

// FuzzScalingNegatives tests scaling with negative values.
func FuzzScalingNegatives(f *testing.F) {
	f.Add(float64(-100), float64(-1), float64(-50))
	f.Add(float64(-50), float64(2), float64(100))
	f.Add(float64(100), float64(-0.5), float64(-25))

	f.Fuzz(func(t *testing.T, value, scaleFactor, offset float64) {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return
		}
		if math.IsNaN(scaleFactor) || math.IsInf(scaleFactor, 0) || scaleFactor == 0 {
			return
		}
		if math.IsNaN(offset) || math.IsInf(offset, 0) {
			return
		}

		// Test that sign is handled correctly
		scaled := applyScaling(value, scaleFactor, offset)
		if math.IsNaN(scaled) || math.IsInf(scaled, 0) {
			return
		}

		// Expected sign of (value * scaleFactor) before offset
		expectedProductSign := (value >= 0) == (scaleFactor >= 0)
		actualProduct := scaled - offset
		actualProductSign := actualProduct >= 0

		if value != 0 && scaleFactor != 0 {
			if expectedProductSign != actualProductSign && math.Abs(actualProduct) > 1e-10 {
				t.Errorf("sign error in scaling: value=%v, scale=%v, offset=%v, scaled=%v, product=%v",
					value, scaleFactor, offset, scaled, actualProduct)
			}
		}
	})
}

// =============================================================================
// Helper Functions
// =============================================================================

// almostEqual checks if two float64 values are approximately equal.
func almostEqual(a, b, tolerance float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return false
	}
	if math.IsInf(a, 0) || math.IsInf(b, 0) {
		return a == b
	}
	return math.Abs(a-b) <= tolerance
}

// Note: applyScaling and reverseScaling are defined in modbus_conversion_fuzz_test.go in the same package
