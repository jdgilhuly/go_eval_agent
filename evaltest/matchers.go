package evaltest

import "fmt"

// ScoreMatcher defines an interface for matching judge scores.
type ScoreMatcher interface {
	Match(score float64) bool
	String() string
}

// scoreAbove matches scores strictly above a threshold.
type scoreAbove struct {
	threshold float64
}

// ScoreAbove returns a matcher that passes when the score is strictly greater
// than the given threshold.
func ScoreAbove(threshold float64) ScoreMatcher {
	return scoreAbove{threshold: threshold}
}

func (m scoreAbove) Match(score float64) bool {
	return score > m.threshold
}

func (m scoreAbove) String() string {
	return fmt.Sprintf("score > %.2f", m.threshold)
}

// scoreExact matches scores that are exactly equal (within floating point
// tolerance).
type scoreExact struct {
	expected float64
}

// ScoreExact returns a matcher that passes when the score equals the expected
// value within a tolerance of 0.001.
func ScoreExact(expected float64) ScoreMatcher {
	return scoreExact{expected: expected}
}

func (m scoreExact) Match(score float64) bool {
	diff := score - m.expected
	if diff < 0 {
		diff = -diff
	}
	return diff < 0.001
}

func (m scoreExact) String() string {
	return fmt.Sprintf("score == %.2f", m.expected)
}

// scoreAtLeast matches scores greater than or equal to a minimum.
type scoreAtLeast struct {
	min float64
}

// ScoreAtLeast returns a matcher that passes when the score is greater than
// or equal to the given minimum.
func ScoreAtLeast(min float64) ScoreMatcher {
	return scoreAtLeast{min: min}
}

func (m scoreAtLeast) Match(score float64) bool {
	return score >= m.min
}

func (m scoreAtLeast) String() string {
	return fmt.Sprintf("score >= %.2f", m.min)
}
