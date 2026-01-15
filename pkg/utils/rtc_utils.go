package utils

// ExtractCandidates extracts candidate strings from the interface slice
func ExtractCandidates(candidates []interface{}) []string {
	var candidateStrs []string
	for _, c := range candidates {
		if candidateStr, ok := c.(string); ok {
			candidateStrs = append(candidateStrs, candidateStr)
		}
	}
	return candidateStrs
}
