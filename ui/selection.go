package ui

// restoreSelection keeps a selection attached to the same logical row after a
// refresh reorders the backing data. If the row disappeared (or there was no
// prior selection), the old index is clamped to the refreshed result.
func restoreSelection[T any](rows []T, fallback int, keep bool, matches func(T) bool) int {
	if keep {
		for i, row := range rows {
			if matches(row) {
				return i
			}
		}
	}
	return clampIndex(fallback, len(rows))
}

func sameTopologyHost(a, b topologyHostRow) bool {
	return a.kind == b.kind && a.host == b.host && a.port == b.port
}
