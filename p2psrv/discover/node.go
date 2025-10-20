// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package discover

import (
	"math/rand"

	"github.com/ethereum/go-ethereum/common"
)

// Node represents a host on the network.
// The fields of Node may not be modified.
//type Node struct {
//	IP       net.IP // len 4 for IPv4 or 16 for IPv6
//	UDP, TCP uint16 // port numbers
//	ID       NodeID // the node's public key
//
//	// This is a cached copy of sha3(ID) which is used for node
//	// distance calculations. This is part of Node in order to make it
//	// possible to write tests that need a node at a certain distance.
//	// In those tests, the content of sha will not actually correspond
//	// with ID.
//	sha common.Hash
//
//	// Time when the node was added to the table.
//	addedAt time.Time
//}
//
//// NewNode creates a new node. It is mostly meant to be used for
//// testing purposes.
//func NewNode(id NodeID, ip net.IP, udpPort, tcpPort uint16) *Node {
//	if ipv4 := ip.To4(); ipv4 != nil {
//		ip = ipv4
//	}
//	return &Node{
//		IP:  ip,
//		UDP: udpPort,
//		TCP: tcpPort,
//		ID:  id,
//		sha: crypto.Keccak256Hash(id[:]),
//	}
//}
//
//func (n *Node) addr() *net.UDPAddr {
//	return &net.UDPAddr{IP: n.IP, Port: int(n.UDP)}
//}
//
//// Incomplete returns true for nodes with no IP address.
//func (n *Node) Incomplete() bool {
//	return n.IP == nil
//}
//
//// checks whether n is a valid complete node.
//func (n *Node) validateComplete() error {
//	if n.Incomplete() {
//		return errors.New("incomplete node")
//	}
//	if n.UDP == 0 {
//		return errors.New("missing UDP port")
//	}
//	if n.TCP == 0 {
//		return errors.New("missing TCP port")
//	}
//	if n.IP.IsMulticast() || n.IP.IsUnspecified() {
//		return errors.New("invalid IP (multicast/unspecified)")
//	}
//	_, err := n.ID.Pubkey() // validate the key (on curve, etc.)
//	return err
//}
//
//// The string representation of a Node is a URL.
//// Please see ParseNode for a description of the format.
//func (n *Node) String() string {
//	u := url.URL{Scheme: "enode"}
//	if n.Incomplete() {
//		u.Host = fmt.Sprintf("%x", n.ID[:])
//	} else {
//		addr := net.TCPAddr{IP: n.IP, Port: int(n.TCP)}
//		u.User = url.User(fmt.Sprintf("%x", n.ID[:]))
//		u.Host = addr.String()
//		if n.UDP != n.TCP {
//			u.RawQuery = "discport=" + strconv.Itoa(int(n.UDP))
//		}
//	}
//	return u.String()
//}
//
//var incompleteNodeURL = regexp.MustCompile("(?i)^(?:enode://)?([0-9a-f]+)$")
//
//// ParseNode parses a node designator.
////
//// There are two basic forms of node designators
////   - incomplete nodes, which only have the public key (node ID)
////   - complete nodes, which contain the public key and IP/Port information
////
//// For incomplete nodes, the designator must look like one of these
////
////	enode://<hex node id>
////	<hex node id>
////
//// For complete nodes, the node ID is encoded in the username portion
//// of the URL, separated from the host by an @ sign. The hostname can
//// only be given as an IP address, DNS domain names are not allowed.
//// The port in the host name section is the TCP listening port. If the
//// TCP and UDP (discovery) ports differ, the UDP port is specified as
//// query parameter "discport".
////
//// In the following example, the node URL describes
//// a node with IP address 10.3.58.6, TCP listening port 30303
//// and UDP discovery port 30301.
////
////	enode://<hex node id>@10.3.58.6:30303?discport=30301
//func ParseNode(rawurl string) (*Node, error) {
//	if m := incompleteNodeURL.FindStringSubmatch(rawurl); m != nil {
//		id, err := HexID(m[1])
//		if err != nil {
//			return nil, fmt.Errorf("invalid node ID (%v)", err)
//		}
//		return NewNode(id, nil, 0, 0), nil
//	}
//	return parseComplete(rawurl)
//}
//
//func parseComplete(rawurl string) (*Node, error) {
//	var (
//		id               NodeID
//		ip               net.IP
//		tcpPort, udpPort uint64
//	)
//	u, err := url.Parse(rawurl)
//	if err != nil {
//		return nil, err
//	}
//	if u.Scheme != "enode" {
//		return nil, errors.New("invalid URL scheme, want \"enode\"")
//	}
//	// Parse the Node ID from the user portion.
//	if u.User == nil {
//		return nil, errors.New("does not contain node ID")
//	}
//	if id, err = HexID(u.User.String()); err != nil {
//		return nil, fmt.Errorf("invalid node ID (%v)", err)
//	}
//	// Parse the IP address.
//	host, port, err := net.SplitHostPort(u.Host)
//	if err != nil {
//		return nil, fmt.Errorf("invalid host: %v", err)
//	}
//	if ip = net.ParseIP(host); ip == nil {
//		return nil, errors.New("invalid IP address")
//	}
//	// Ensure the IP is 4 bytes long for IPv4 addresses.
//	if ipv4 := ip.To4(); ipv4 != nil {
//		ip = ipv4
//	}
//	// Parse the port numbers.
//	if tcpPort, err = strconv.ParseUint(port, 10, 16); err != nil {
//		return nil, errors.New("invalid port")
//	}
//	udpPort = tcpPort
//	qv := u.Query()
//	if qv.Get("discport") != "" {
//		udpPort, err = strconv.ParseUint(qv.Get("discport"), 10, 16)
//		if err != nil {
//			return nil, errors.New("invalid discport in query")
//		}
//	}
//	return NewNode(id, ip, uint16(udpPort), uint16(tcpPort)), nil
//}
//
//// MustParseNode parses a node URL. It panics if the URL is not valid.
//func MustParseNode(rawurl string) *Node {
//	n, err := ParseNode(rawurl)
//	if err != nil {
//		panic("invalid node URL: " + err.Error())
//	}
//	return n
//}
//
//// MarshalText implements encoding.TextMarshaler.
//func (n *Node) MarshalText() ([]byte, error) {
//	return []byte(n.String()), nil
//}
//
//// UnmarshalText implements encoding.TextUnmarshaler.
//func (n *Node) UnmarshalText(text []byte) error {
//	dec, err := ParseNode(string(text))
//	if err == nil {
//		*n = *dec
//	}
//	return err
//}

// table of leading zero counts for bytes [0..255]
var lzcount = [256]int{
	8, 7, 6, 6, 5, 5, 5, 5,
	4, 4, 4, 4, 4, 4, 4, 4,
	3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3,
	2, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 2, 2,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

// logdist returns the logarithmic distance between a and b, log2(a ^ b).
func logdist(a, b common.Hash) int {
	lz := 0
	for i := range a {
		x := a[i] ^ b[i]
		if x == 0 {
			lz += 8
		} else {
			lz += lzcount[x]
			break
		}
	}
	return len(a)*8 - lz
}

// hashAtDistance returns a random hash such that logdist(a, b) == n
func hashAtDistance(a common.Hash, n int) (b common.Hash) {
	if n == 0 {
		return a
	}
	// flip bit at position n, fill the rest with random bits
	b = a
	pos := len(a) - n/8 - 1
	bit := byte(0x01) << (byte(n%8) - 1)
	if bit == 0 {
		pos++
		bit = 0x80
	}
	b[pos] = a[pos]&^bit | ^a[pos]&bit // TODO: randomize end bits
	for i := pos + 1; i < len(a); i++ {
		b[i] = byte(rand.Intn(255))
	}
	return b
}
