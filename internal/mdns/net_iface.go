package mdns

import (
	"fmt"
	"net"
	"sort"
)

// Interface describes a usable network interface and its IPv4 addresses.
type Interface struct {
	Index     int
	Name      string
	Addresses []net.IP
}

// ListInterfaces returns interfaces that can carry mDNS traffic: up, multicast
// capable, not loopback, with at least one usable IPv4 address.
func ListInterfaces() ([]Interface, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("枚举网络接口失败: %w", err)
	}
	result := make([]Interface, 0, len(all))
	for _, ni := range all {
		if ni.Flags&net.FlagUp == 0 ||
			ni.Flags&net.FlagMulticast == 0 ||
			ni.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ni.Addrs()
		if err != nil {
			continue
		}
		var ips []net.IP
		seen := make(map[string]bool)
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip4 := usableIPv4(ip)
			if ip4 == nil {
				continue
			}
			key := ip4.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			ips = append(ips, ip4)
		}
		if len(ips) == 0 {
			continue
		}
		sort.Slice(ips, func(i, j int) bool { return bytesLess(ips[i], ips[j]) })
		result = append(result, Interface{
			Index:     ni.Index,
			Name:      ni.Name,
			Addresses: ips,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	return result, nil
}

func usableIPv4(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return nil
	}
	if ip4.IsLoopback() || ip4.IsUnspecified() || ip4.IsLinkLocalUnicast() {
		return nil
	}
	return ip4
}

func bytesLess(a, b net.IP) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// FilterInterfaces keeps only the interfaces whose index is in indexes. An
// empty indexes slice selects every usable interface.
func FilterInterfaces(all []Interface, indexes []int) []Interface {
	if len(indexes) == 0 {
		return all
	}
	wanted := make(map[int]bool, len(indexes))
	for _, idx := range indexes {
		wanted[idx] = true
	}
	out := make([]Interface, 0, len(indexes))
	for _, iface := range all {
		if wanted[iface.Index] {
			out = append(out, iface)
		}
	}
	return out
}

// Addresses returns the deduplicated IPv4 addresses of the given interfaces.
func Addresses(ifaces []Interface) []net.IP {
	var out []net.IP
	seen := make(map[string]bool)
	for _, iface := range ifaces {
		for _, ip := range iface.Addresses {
			key := ip.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, ip)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// Keep the more common private ranges first for a tidier UI.
		pa, pb := out[i].IsPrivate(), out[j].IsPrivate()
		if pa != pb {
			return pa
		}
		return bytesLess(out[i], out[j])
	})
	return out
}

// HasAddress reports whether ip belongs to the local machine's published set.
func HasAddress(addrs []net.IP, ip net.IP) bool {
	if ip == nil {
		return false
	}
	target := ip.To4()
	if target == nil {
		return false
	}
	for _, a := range addrs {
		if a.Equal(target) {
			return true
		}
	}
	return false
}
