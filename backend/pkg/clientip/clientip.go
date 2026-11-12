// Package clientip groups addresses into what counts as one client.
package clientip

import "net/netip"

// Range returns the first and last address of the client owning ip. An IPv6
// customer owns a whole /64, so it counts as one client; IPv4 is one address.
// An unparsable ip comes back unchanged.
func Range(ip string) (first, last string) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip, ip
	}
	addr = addr.Unmap().WithZone("")
	if !addr.Is6() {
		return addr.String(), addr.String()
	}
	raw := addr.As16()
	clear(raw[8:])
	lo := netip.AddrFrom16(raw)
	for i := 8; i < len(raw); i++ {
		raw[i] = 0xff
	}
	return lo.String(), netip.AddrFrom16(raw).String()
}
