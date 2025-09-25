package service

func damerauLevenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	al, bl := len(ra), len(rb)

	dp := make([][]int, al+1)
	for i := 0; i <= al; i++ {
		dp[i] = make([]int, bl+1)
	}
	for i := 0; i <= al; i++ {
		dp[i][0] = i
	}
	for j := 0; j <= bl; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= al; i++ {
		for j := 1; j <= bl; j++ {
			cost := 0
			if ra[i-1] != rb[j-1] {
				cost = 1
			}
			// вставка / удаление / замена
			dp[i][j] = min3(dp[i-1][j]+1, dp[i][j-1]+1, dp[i-1][j-1]+cost)

			// транспозиция соседних символов
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				if v := dp[i-2][j-2] + 1; v < dp[i][j] {
					dp[i][j] = v
				}
			}
		}
	}
	return dp[al][bl]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func min3(a, b, c int) int { return min(min(a, b), c) }
