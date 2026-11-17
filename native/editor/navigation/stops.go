package navigation

import "sort"

// NearestStopAtOrBefore returns the greatest stop no greater than offset. If
// offset precedes every stop, it returns the first stop. Stops must be sorted.
func NearestStopAtOrBefore(stops []int, offset int) (int, bool) {
	if len(stops) == 0 {
		return 0, false
	}
	i := sort.Search(len(stops), func(i int) bool { return stops[i] > offset })
	if i == 0 {
		return stops[0], true
	}
	return stops[i-1], true
}
