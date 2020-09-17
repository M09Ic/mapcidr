// Package mapcidr implements methods to allow working with CIDRs.
package mapcidr

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"net"
)

// AddressRange returns the first and last addresses in the given CIDR range.
func AddressRange(network *net.IPNet) (net.IP, net.IP, error) {
	firstIP := network.IP

	prefixLen, bits := network.Mask.Size()
	if prefixLen == bits {
		lastIP := make([]byte, len(firstIP))
		copy(lastIP, firstIP)
		return firstIP, lastIP, nil
	}

	firstIPInt, bits, err := IPToInteger(firstIP)
	if err != nil {
		return nil, nil, err
	}
	hostLen := uint(bits) - uint(prefixLen)
	lastIPInt := big.NewInt(1)
	lastIPInt.Lsh(lastIPInt, hostLen)
	lastIPInt.Sub(lastIPInt, big.NewInt(1))
	lastIPInt.Or(lastIPInt, firstIPInt)

	return firstIP, IntegerToIP(lastIPInt, bits), nil
}

// AddressCount returns the number of IP addresses in a range
func AddressCount(cidr string) (uint64, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, err
	}
	return AddressCountIpnet(ipnet), nil
}

// AddressCountIpnet returns the number of IP addresses in an IPNet structure
func AddressCountIpnet(network *net.IPNet) uint64 {
	prefixLen, bits := network.Mask.Size()
	return 1 << (uint64(bits) - uint64(prefixLen))
}

// SplitByNumber splits the given cidr into subnets with the closest
// number of hosts per subnet.
func SplitByNumber(iprange string, number int) ([]*net.IPNet, error) {
	_, ipnet, err := net.ParseCIDR(iprange)
	if err != nil {
		return nil, err
	}
	return SplitIPNetByNumber(ipnet, number)
}

// SplitIPNetByNumber splits an IPNet into subnets with the closest n
// umber of hosts per subnet.
func SplitIPNetByNumber(ipnet *net.IPNet, number int) ([]*net.IPNet, error) {
	ipsNumber := AddressCountIpnet(ipnet)

	// truncate result to nearest uint64
	optimalSplit := int(ipsNumber / uint64(number))
	return SplitIPNetIntoN(ipnet, optimalSplit)
}

// SplitN attempts to split a cidr in the exact number of subnets
func SplitN(iprange string, n int) ([]*net.IPNet, error) {
	_, ipnet, err := net.ParseCIDR(iprange)
	if err != nil {
		return nil, err
	}
	return SplitIPNetIntoN(ipnet, n)
}

// SplitIPNetIntoN attempts to split a ipnet in the exact number of subnets
func SplitIPNetIntoN(iprange *net.IPNet, n int) ([]*net.IPNet, error) {
	var err error
	subnets := make([]*net.IPNet, 0, n)

	// invalid value
	if n <= 1 || AddressCountIpnet(iprange) < uint64(n) {
		subnets = append(subnets, iprange)
		return subnets, nil
	}
	// power of two
	if isPowerOfTwo(n) || isPowerOfTwoPlusOne(n) {
		return splitIPNet(iprange, n)
	}

	var closestMinorPowerOfTwo int
	// find the closest power of two in a stupid way
	for i := n; i > 0; i-- {
		if isPowerOfTwo(i) {
			closestMinorPowerOfTwo = i
			break
		}
	}

	subnets, err = splitIPNet(iprange, closestMinorPowerOfTwo)
	if err != nil {
		return nil, err
	}
	for len(subnets) < n {
		var newSubnets []*net.IPNet
		level := 1
		for i := len(subnets) - 1; i >= 0; i-- {
			divided, err := divideIPNet(subnets[i])
			if err != nil {
				return nil, err
			}
			newSubnets = append(newSubnets, divided...)
			if len(subnets)-level+len(newSubnets) == n {
				reverseIPNet(newSubnets)
				subnets = subnets[:len(subnets)-level]
				subnets = append(subnets, newSubnets...)
				return subnets, nil
			}
			level++
		}
		reverseIPNet(newSubnets)
		subnets = newSubnets
	}
	return subnets, nil
}

// divide divides an IPNet into two IPNet structures
func divide(iprange string) ([]*net.IPNet, error) {
	_, ipnet, _ := net.ParseCIDR(iprange)
	return divideIPNet(ipnet)
}

// divideIPNet divides an IPNet into two IPNet structures.
func divideIPNet(ipnet *net.IPNet) ([]*net.IPNet, error) {
	subnets := make([]*net.IPNet, 0, 2)

	maskBits, _ := ipnet.Mask.Size()
	wantedMaskBits := maskBits + 1

	currentSubnet, err := currentSubnet(ipnet, wantedMaskBits)
	if err != nil {
		return nil, err
	}
	subnets = append(subnets, currentSubnet)
	nextSubnet, _, err := nextSubnet(currentSubnet, wantedMaskBits)
	if err != nil {
		return nil, err
	}
	subnets = append(subnets, nextSubnet)

	return subnets, nil
}

// splitIPNet into approximate N counts
func splitIPNet(ipnet *net.IPNet, n int) ([]*net.IPNet, error) {
	var err error
	subnets := make([]*net.IPNet, 0, n)

	maskBits, _ := ipnet.Mask.Size()
	closestPow2 := int(closestPowerOfTwo(uint32(n)))
	pow2 := int(math.Log2(float64(closestPow2)))

	wantedMaskBits := maskBits + pow2

	currentSubnet, err := currentSubnet(ipnet, wantedMaskBits)
	if err != nil {
		return nil, err
	}
	subnets = append(subnets, currentSubnet)
	nxtSubnet := currentSubnet
	for i := 0; i < closestPow2-1; i++ {
		nxtSubnet, _, err = nextSubnet(nxtSubnet, wantedMaskBits)
		if err != nil {
			return nil, err
		}
		subnets = append(subnets, nxtSubnet)
	}

	if len(subnets) < n {
		lastSubnet := subnets[len(subnets)-1]
		subnets = subnets[:len(subnets)-1]
		ipnets, err := divideIPNet(lastSubnet)
		if err != nil {
			return nil, err
		}
		subnets = append(subnets, ipnets...)
	}
	return subnets, nil
}

func split(iprange string, n int) ([]*net.IPNet, error) {
	_, ipnet, _ := net.ParseCIDR(iprange)
	return splitIPNet(ipnet, n)
}

func nextPowerOfTwo(v uint32) uint32 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return v
}

func closestPowerOfTwo(v uint32) uint32 {
	next := nextPowerOfTwo(v)
	if prev := next / 2; (v - prev) < (next - v) {
		next = prev
	}
	return next
}

func currentSubnet(network *net.IPNet, prefixLen int) (*net.IPNet, error) {
	currentFirst, _, err := AddressRange(network)
	if err != nil {
		return nil, err
	}
	mask := net.CIDRMask(prefixLen, 8*len(currentFirst))
	return &net.IPNet{IP: currentFirst.Mask(mask), Mask: mask}, nil
}

func previousSubnet(network *net.IPNet, prefixLen int) (*net.IPNet, bool) {
	startIP := network.IP
	previousIP := make(net.IP, len(startIP))
	copy(previousIP, startIP)
	cMask := net.CIDRMask(prefixLen, 8*len(previousIP))
	previousIP = dec(previousIP)
	previous := &net.IPNet{IP: previousIP.Mask(cMask), Mask: cMask}
	if startIP.Equal(net.IPv4zero) || startIP.Equal(net.IPv6zero) {
		return previous, true
	}
	return previous, false
}

// nextSubnet returns the next subnet for an ipnet
func nextSubnet(network *net.IPNet, prefixLen int) (*net.IPNet, bool, error) {
	_, currentLast, err := AddressRange(network)
	if err != nil {
		return nil, false, err
	}
	mask := net.CIDRMask(prefixLen, 8*len(currentLast))
	currentSubnet := &net.IPNet{IP: currentLast.Mask(mask), Mask: mask}
	_, last, err := AddressRange(currentSubnet)
	if err != nil {
		return nil, false, err
	}
	last = inc(last)
	next := &net.IPNet{IP: last.Mask(mask), Mask: mask}
	if last.Equal(net.IPv4zero) || last.Equal(net.IPv6zero) {
		return next, true, nil
	}
	return next, false, nil
}

func isPowerOfTwoPlusOne(x int) bool {
	return isPowerOfTwo(x - 1)
}

// isPowerOfTwo returns if a number is a power of 2
func isPowerOfTwo(x int) bool {
	return x != 0 && (x&(x-1)) == 0
}

// reverseIPNet reverses an ipnet slice
func reverseIPNet(ipnets []*net.IPNet) {
	for i, j := 0, len(ipnets)-1; i < j; i, j = i+1, j-1 {
		ipnets[i], ipnets[j] = ipnets[j], ipnets[i]
	}
}

// IPAddresses returns all the IP addresses in a CIDR
func IPAddresses(cidr string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return []string{}, err
	}
	return IPAddressesIPnet(ipnet), nil
}

// IPAddressesIPnet returns all IP addresses in an IPNet.
func IPAddressesIPnet(ipnet *net.IPNet) (ips []string) {
	// convert IPNet struct mask and address to uint32
	mask := binary.BigEndian.Uint32(ipnet.Mask)
	start := binary.BigEndian.Uint32(ipnet.IP)

	// find the final address
	finish := (start & mask) | (mask ^ 0xffffffff)

	// loop through addresses as uint32
	for i := start; i <= finish; i++ {
		// convert back to net.IP
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, i)
		ips = append(ips, ip.String())
	}
	return ips
}

// IPToInteger converts an IP address to its integer representation.
// It supports both IPv4 as well as IPv6 addresses.
func IPToInteger(ip net.IP) (*big.Int, int, error) {
	val := &big.Int{}
	val.SetBytes([]byte(ip))

	if len(ip) == net.IPv4len {
		return val, 32, nil
	} else if len(ip) == net.IPv6len {
		return val, 128, nil
	} else {
		return nil, 0, fmt.Errorf("Unsupported address length %d", len(ip))
	}
}

// IntegerToIP converts an Integer IP address to net.IP format.
func IntegerToIP(ipInt *big.Int, bits int) net.IP {
	ipBytes := ipInt.Bytes()
	ret := make([]byte, bits/8)
	for i := 1; i <= len(ipBytes); i++ {
		ret[len(ret)-i] = ipBytes[len(ipBytes)-i]
	}
	return net.IP(ret)
}