package countrylookup

import (
	_ "embed"
	"strconv"
	"strings"
)

type CountryLookup struct {
	country_codes []string
	ip_ranges     []uint64
	country_table []string
}

//go:embed lookup-data/ip_supalite.table
var b []byte

func New() *CountryLookup {
	index := 0
	country_codes := make([]string, 243)
	ip_ranges := make([]uint64, 198578)
	country_table := make([]string, 198578)
	cc_i := 0
	for range b {
		c1 := b[index]
		c2 := b[index+1]
		index += 2

		country_codes[cc_i] = string([]byte{c1, c2})
		cc_i += 1
		if c1 == '*' {
			break
		}
	}

	var last_end_range uint64 = 0
	var table_idx int = 0
	for index < len(b) {
		var count int = 0
		n1 := b[index]
		index += 1
		if n1 < 240 {
			count = int(n1)
		} else if n1 == 242 {
			n2 := int(b[index])
			n3 := int(b[index+1])
			index += 2
			count = n2 | n3<<8
		} else if n1 == 243 {
			n2 := int(b[index])
			n3 := int(b[index+1])
			n4 := int(b[index+2])
			index += 3
			count = n2 | n3<<8 | n4<<16
		}
		last_end_range += uint64(count * 256)
		cc := b[index]
		index += 1
		ip_ranges[table_idx] = last_end_range
		country_table[table_idx] = country_codes[cc]
		table_idx += 1
	}
	return &CountryLookup{
		country_codes: country_codes,
		country_table: country_table,
		ip_ranges:     ip_ranges,
	}
}

func (l *CountryLookup) LookupIp(ip_string string) (response string, ok bool) {
	if ip_string == "" {
		return "", false
	}
	one, two, three, four, ok := getIpNumbers(ip_string)

	if !ok {
		return "", false
	}

	ipNumber := uint64(
		one*16777216 +
			two*65536 +
			three*256 +
			four)
	return l.LookupNumericIp(ipNumber)
}

func (l *CountryLookup) LookupNumericIp(ip_number uint64) (response string, ok bool) {
	index := l.binarySearch(ip_number)
	cc := l.country_table[index]
	if cc == "--" {
		return "", false
	}
	return cc, true
}

func (l *CountryLookup) binarySearch(ip_number uint64) int {
	min := 0
	max := len(l.ip_ranges) - 1
	var mid int
	for min < max {
		mid = (min + max) >> 1
		if l.ip_ranges[mid] <= ip_number {
			min = mid + 1
		} else {
			max = mid
		}
	}

	return min
}

func getIpNumbers(ip_string string) (one int, two int, three int, four int, ok bool) {
	parts := strings.Split(ip_string, ".")
	if len(parts) != 4 {
		return 0, 0, 0, 0, false
	}

	var err error

	one, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, 0, false
	}
	two, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, 0, false
	}
	three, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, 0, false
	}
	four, err = strconv.Atoi(parts[3])
	if err != nil {
		return 0, 0, 0, 0, false
	}
	return one, two, three, four, true
}
