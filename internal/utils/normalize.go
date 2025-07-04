package utils

// CreateRankList creates a slice of ranks based on position.
// The rank starts at 1 for the first item and increments for subsequent items.
// Useful for ranking items that are already sorted.
func CreateRankList(count int) []uint16 {
	if count <= 0 {
		return []uint16{}
	}
	ranks := make([]uint16, count)
	for i := 0; i < count; i++ {
		ranks[i] = uint16(i + 1)
	}
	return ranks
}
