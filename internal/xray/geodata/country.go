package geodata

import (
	"net/netip"
	"strings"
)

// CountryOf names the category of a geoip database whose ranges hold addr, in
// upper case, or "" when none does. It scans the database, so call it rarely.
func (s *Store) CountryOf(name string, addr netip.Addr) (string, error) {
	cats, err := s.Categories(name, "", 0, 0)
	if err != nil {
		return "", err
	}
	addr = addr.Unmap()
	for _, cat := range cats.Items {
		if cat.Code == "private" {
			continue
		}
		for offset := 0; offset < cat.Entries; offset += MaxPageSize {
			page, err := s.Entries(name, cat.Code, "", offset, MaxPageSize)
			if err != nil {
				return "", err
			}
			for _, e := range page.Items {
				if prefix, err := netip.ParsePrefix(e.Value); err == nil && prefix.Contains(addr) {
					return strings.ToUpper(cat.Code), nil
				}
			}
		}
	}
	return "", nil
}
