package xray

import "sort"

func sortTraffic(rows []Traffic) {
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i].Uplink + rows[i].Downlink
		b := rows[j].Uplink + rows[j].Downlink
		if a != b {
			return a > b
		}
		return rows[i].Name < rows[j].Name
	})
}
