package utils

// GeneratePositionalRanks creates a slice of ranks based on position.
// The rank starts at 1.0 for the first item and increments for subsequent items.
// Useful for ranking items that are already sorted.
func GeneratePositionalRanks(count int) []float64 {
	if count <= 0 {
		return []float64{}
	}
	ranks := make([]float64, count)
	for i := 0; i < count; i++ {
		ranks[i] = float64(i + 1)
	}
	return ranks
}
