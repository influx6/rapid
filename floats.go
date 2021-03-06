// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package rapid

import (
	"fmt"
	"math"
	"reflect"
)

const (
	float32ExpBits    = 8
	float32SignifBits = 23

	float64ExpBits    = 11
	float64SignifBits = 52

	floatExpLabel    = "floatexp"
	floatSignifLabel = "floatsignif"
)

var (
	float32Type = reflect.TypeOf(float32(0))
	float64Type = reflect.TypeOf(float64(0))
)

func Float32s() *Generator {
	return Float32sRange(-math.MaxFloat32, math.MaxFloat32)
}

func Float32sMin(min float32) *Generator {
	return Float32sRange(min, math.MaxFloat32)
}

func Float32sMax(max float32) *Generator {
	return Float32sRange(-math.MaxFloat32, max)
}

func Float32sRange(min float32, max float32) *Generator {
	assertf(min == min, "min should not be a NaN")
	assertf(max == max, "max should not be a NaN")
	assertf(min <= max, "invalid range [%v, %v]", min, max)

	return newGenerator(&floatGen{
		typ:        float32Type,
		expBits:    float32ExpBits,
		signifBits: float32SignifBits,
		min:        float64(min),
		max:        float64(max),
		minVal:     -math.MaxFloat32,
		maxVal:     math.MaxFloat32,
	})
}

func Float64s() *Generator {
	return Float64sRange(-math.MaxFloat64, math.MaxFloat64)
}

func Float64sMin(min float64) *Generator {
	return Float64sRange(min, math.MaxFloat64)
}

func Float64sMax(max float64) *Generator {
	return Float64sRange(-math.MaxFloat64, max)
}

func Float64sRange(min float64, max float64) *Generator {
	assertf(min == min, "min should not be a NaN")
	assertf(max == max, "max should not be a NaN")
	assertf(min <= max, "invalid range [%v, %v]", min, max)

	return newGenerator(&floatGen{
		typ:        float64Type,
		expBits:    float64ExpBits,
		signifBits: float64SignifBits,
		min:        min,
		max:        max,
		minVal:     -math.MaxFloat64,
		maxVal:     math.MaxFloat64,
	})
}

type floatGen struct {
	typ        reflect.Type
	expBits    uint
	signifBits uint
	min        float64
	max        float64
	minVal     float64
	maxVal     float64
}

func (g *floatGen) String() string {
	kind := "Float64s"
	if g.typ == float32Type {
		kind = "Float32s"
	}

	if g.min != g.minVal && g.max != g.maxVal {
		return fmt.Sprintf("%sRange(%g, %g)", kind, g.min, g.max)
	} else if g.min != g.minVal {
		return fmt.Sprintf("%sMin(%g)", kind, g.min)
	} else if g.max != g.maxVal {
		return fmt.Sprintf("%sMax(%g)", kind, g.max)
	}

	return fmt.Sprintf("%s()", kind)
}

func (g *floatGen) type_() reflect.Type {
	return g.typ
}

func (g *floatGen) value(s bitStream) Value {
	f := genFloatRange(s, g.min, g.max, g.expBits, g.signifBits)

	if g.typ == float32Type {
		return float32(f)
	} else {
		return f
	}
}

func ufloatFracBits(e int32, signifBits uint) uint {
	if e <= 0 {
		return signifBits
	} else if uint(e) < signifBits {
		return signifBits - uint(e)
	} else {
		return 0
	}
}

func ufloatParts(f float64, expBits uint, signifBits uint) (int32, uint64, uint64) {
	u := math.Float64bits(f) & math.MaxInt64

	e := int32(u>>float64SignifBits) - int32(bitmask64(float64ExpBits-1))
	b := int32(bitmask64(expBits - 1))
	if e < -b+1 {
		e = -b + 1 // -b is subnormal
	} else if e > b {
		e = b // b+1 is Inf/NaN
	}

	s := (u & bitmask64(float64SignifBits)) >> (float64SignifBits - signifBits)
	n := ufloatFracBits(e, signifBits)

	return e, s >> n, s & bitmask64(n)
}

func genUfloatRange(s bitStream, min float64, max float64, expBits uint, signifBits uint) float64 {
	assert(min >= 0 && min <= max)

	minExp, minSignifI, minSignifF := ufloatParts(min, expBits, signifBits)
	maxExp, maxSignifI, maxSignifF := ufloatParts(max, expBits, signifBits)

	i := s.beginGroup(floatExpLabel, false)
	e := genIntRange(s, int64(minExp), int64(maxExp), true)
	s.endGroup(i, false)

	fracBits := ufloatFracBits(int32(e), signifBits)

	j := s.beginGroup(floatSignifLabel, false)
	var siMin, siMax uint64
	switch {
	case minExp == maxExp:
		siMin, siMax = minSignifI, maxSignifI
	case int32(e) == minExp:
		siMin, siMax = minSignifI, bitmask64(signifBits-fracBits)
	case int32(e) == maxExp:
		siMin, siMax = 0, maxSignifI
	default:
		siMin, siMax = 0, bitmask64(signifBits-fracBits)
	}
	si := genUintRange(s, siMin, siMax, false)
	var sfMin, sfMax uint64
	switch {
	case minExp == maxExp && minSignifI == maxSignifI:
		sfMin, sfMax = minSignifF, maxSignifF
	case int32(e) == minExp && si == minSignifI:
		sfMin, sfMax = minSignifF, bitmask64(fracBits)
	case int32(e) == maxExp && si == maxSignifI:
		sfMin, sfMax = 0, maxSignifF
	default:
		sfMin, sfMax = 0, bitmask64(fracBits)
	}
	sf, w := genUintRangeWidth(s, sfMin, sfMax, true)
	s.endGroup(j, false)

	for i := 0; i < int(fracBits)-w; i++ {
		sf_ := sf << 1
		if sf_ < sfMin || sf_ > sfMax || sf_ < sf {
			break
		}
		sf = sf_
	}

	e_ := (uint64(e) + bitmask64(float64ExpBits-1)) << float64SignifBits
	s_ := (si<<fracBits | sf) << (float64SignifBits - signifBits)

	return math.Float64frombits(e_ | s_)
}

func genFloatRange(s bitStream, min float64, max float64, expBits uint, signifBits uint) float64 {
	var posMin, negMin, pNeg float64
	if min >= 0 {
		posMin = min
		pNeg = 0
	} else if max <= 0 {
		negMin = -max
		pNeg = 1
	} else {
		pos := math.Log1p(math.Log1p(max))
		neg := math.Log1p(math.Log1p(-min))
		pNeg = neg / (neg + pos)
	}

	if flipBiasedCoin(s, pNeg) {
		return -genUfloatRange(s, negMin, -min, expBits, signifBits)
	} else {
		return genUfloatRange(s, posMin, max, expBits, signifBits)
	}
}
