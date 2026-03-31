package brainrag

import (
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	t.Run("identical vectors", func(t *testing.T) {
		v := []float32{1, 2, 3}
		got := CosineSimilarity(v, v)
		if math.Abs(float64(got-1.0)) > 1e-5 {
			t.Errorf("identical vectors: want ~1.0, got %v", got)
		}
	})

	t.Run("orthogonal vectors", func(t *testing.T) {
		a := []float32{1, 0, 0}
		b := []float32{0, 1, 0}
		got := CosineSimilarity(a, b)
		if math.Abs(float64(got)) > 1e-5 {
			t.Errorf("orthogonal vectors: want ~0.0, got %v", got)
		}
	})

	t.Run("parallel vectors (different magnitude)", func(t *testing.T) {
		a := []float32{1, 2, 3}
		b := []float32{2, 4, 6}
		got := CosineSimilarity(a, b)
		if math.Abs(float64(got-1.0)) > 1e-5 {
			t.Errorf("parallel vectors: want ~1.0, got %v", got)
		}
	})

	t.Run("zero-length vector", func(t *testing.T) {
		a := []float32{}
		b := []float32{1, 2, 3}
		got := CosineSimilarity(a, b)
		if got != 0 {
			t.Errorf("empty vector: want 0, got %v", got)
		}
	})

	t.Run("different dimensions", func(t *testing.T) {
		a := []float32{1, 2}
		b := []float32{1, 2, 3}
		got := CosineSimilarity(a, b)
		if got != 0 {
			t.Errorf("different dims: want 0, got %v", got)
		}
	})

	t.Run("zero magnitude vector", func(t *testing.T) {
		a := []float32{0, 0, 0}
		b := []float32{1, 2, 3}
		got := CosineSimilarity(a, b)
		if got != 0 {
			t.Errorf("zero magnitude: want 0, got %v", got)
		}
	})
}
