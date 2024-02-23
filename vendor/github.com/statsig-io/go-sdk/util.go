package statsig

func defaultString(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
