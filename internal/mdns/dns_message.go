package mdns

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
)

// DNS record type and class constants used by this mDNS responder.
const (
	TypeA    uint16 = 1
	TypeAAAA uint16 = 28
	TypeANY  uint16 = 255

	ClassIN  uint16 = 1
	ClassANY uint16 = 255

	flagResponse      uint16 = 0x8000
	flagAuthoritative uint16 = 0x0400
	flagRecursionOK   uint16 = 0x0080

	classUnicastResponse uint16 = 0x8000
	cacheFlushBit        uint16 = 0x8000

	maxNameLength  = 253
	maxLabelLength = 63
	maxPointerHops = 64
	maxRecords     = 4096
)

// ErrMalformed is returned when a DNS message cannot be decoded safely.
var ErrMalformed = errors.New("malformed DNS message")

// Question is a single entry of the question section.
type Question struct {
	Name  string
	Type  uint16
	Class uint16
}

// UnicastResponse reports whether the QU bit is set.
func (q Question) UnicastResponse() bool {
	return q.Class&classUnicastResponse != 0
}

// QClass returns the class without the QU bit.
func (q Question) QClass() uint16 {
	return q.Class &^ classUnicastResponse
}

// Record is a resource record. Data holds the raw RDATA.
type Record struct {
	Name  string
	Type  uint16
	Class uint16
	TTL   uint32
	Data  []byte
}

// CacheFlush reports whether the cache-flush bit is set.
func (r Record) CacheFlush() bool {
	return r.Class&cacheFlushBit != 0
}

// RClass returns the class without the cache-flush bit.
func (r Record) RClass() uint16 {
	return r.Class &^ cacheFlushBit
}

// Message is a decoded or to-be-encoded DNS message.
type Message struct {
	ID          uint16
	Flags       uint16
	Questions   []Question
	Answers     []Record
	Authorities []Record
	Additionals []Record
}

// NewQuery builds a recursive-descent-free mDNS query message.
func NewQuery(name string, qtype uint16, unicastResponse bool) *Message {
	class := ClassIN
	if unicastResponse {
		class |= classUnicastResponse
	}
	return &Message{
		Questions: []Question{{Name: normalizeName(name), Type: qtype, Class: class}},
	}
}

// NewResponse builds a message that can be filled with answers.
func NewResponse(id uint16, questions []Question) *Message {
	return &Message{
		ID:        id,
		Flags:     flagResponse | flagAuthoritative,
		Questions: questions,
	}
}

// IsResponse reports whether the QR bit is set.
func (m *Message) IsResponse() bool {
	return m.Flags&flagResponse != 0
}

// ARecord builds an IPv4 address record.
func ARecord(name string, ip net.IP, ttl uint32, flush bool) Record {
	ip4 := ip.To4()
	if ip4 == nil {
		ip4 = net.IPv4zero.To4()
	}
	class := ClassIN
	if flush {
		class |= cacheFlushBit
	}
	return Record{
		Name:  normalizeName(name),
		Type:  TypeA,
		Class: class,
		TTL:   ttl,
		Data:  []byte{ip4[0], ip4[1], ip4[2], ip4[3]},
	}
}

// ARecordIP extracts the IPv4 address from an A record.
func ARecordIP(r Record) net.IP {
	if r.Type != TypeA || len(r.Data) != 4 {
		return nil
	}
	return net.IPv4(r.Data[0], r.Data[1], r.Data[2], r.Data[3])
}

// Pack encodes the message. Name compression is intentionally not used.
func (m *Message) Pack() ([]byte, error) {
	if len(m.Questions) > 0xffff || len(m.Answers) > 0xffff ||
		len(m.Authorities) > 0xffff || len(m.Additionals) > 0xffff {
		return nil, fmt.Errorf("too many DNS records to encode")
	}

	size := 12
	for _, q := range m.Questions {
		size += packedNameLen(q.Name) + 4
	}
	for _, r := range m.Answers {
		size += packedNameLen(r.Name) + 10 + len(r.Data)
	}
	for _, r := range m.Authorities {
		size += packedNameLen(r.Name) + 10 + len(r.Data)
	}
	for _, r := range m.Additionals {
		size += packedNameLen(r.Name) + 10 + len(r.Data)
	}

	b := make([]byte, 0, size)
	var hdr [12]byte
	binary.BigEndian.PutUint16(hdr[0:2], m.ID)
	binary.BigEndian.PutUint16(hdr[2:4], m.Flags)
	binary.BigEndian.PutUint16(hdr[4:6], uint16(len(m.Questions)))
	binary.BigEndian.PutUint16(hdr[6:8], uint16(len(m.Answers)))
	binary.BigEndian.PutUint16(hdr[8:10], uint16(len(m.Authorities)))
	binary.BigEndian.PutUint16(hdr[10:12], uint16(len(m.Additionals)))
	b = append(b, hdr[:]...)

	for _, q := range m.Questions {
		name, err := packName(q.Name)
		if err != nil {
			return nil, err
		}
		b = append(b, name...)
		var qb [4]byte
		binary.BigEndian.PutUint16(qb[0:2], q.Type)
		binary.BigEndian.PutUint16(qb[2:4], q.Class)
		b = append(b, qb[:]...)
	}

	for _, section := range [][]Record{m.Answers, m.Authorities, m.Additionals} {
		for _, r := range section {
			name, err := packName(r.Name)
			if err != nil {
				return nil, err
			}
			b = append(b, name...)
			var rb [10]byte
			binary.BigEndian.PutUint16(rb[0:2], r.Type)
			binary.BigEndian.PutUint16(rb[2:4], r.Class)
			binary.BigEndian.PutUint32(rb[4:8], r.TTL)
			binary.BigEndian.PutUint16(rb[8:10], uint16(len(r.Data)))
			b = append(b, rb[:]...)
			b = append(b, r.Data...)
		}
	}

	return b, nil
}

// ParseMessage decodes a DNS message from wire format.
func ParseMessage(b []byte) (*Message, error) {
	if len(b) < 12 {
		return nil, ErrMalformed
	}
	m := &Message{
		ID:    binary.BigEndian.Uint16(b[0:2]),
		Flags: binary.BigEndian.Uint16(b[2:4]),
	}
	qd := int(binary.BigEndian.Uint16(b[4:6]))
	an := int(binary.BigEndian.Uint16(b[6:8]))
	ns := int(binary.BigEndian.Uint16(b[8:10]))
	ar := int(binary.BigEndian.Uint16(b[10:12]))
	if qd > maxRecords || an > maxRecords || ns > maxRecords || ar > maxRecords {
		return nil, ErrMalformed
	}

	off := 12
	var err error
	for i := 0; i < qd; i++ {
		var q Question
		q.Name, off, err = decodeName(b, off)
		if err != nil {
			return nil, err
		}
		if off+4 > len(b) {
			return nil, ErrMalformed
		}
		q.Type = binary.BigEndian.Uint16(b[off : off+2])
		q.Class = binary.BigEndian.Uint16(b[off+2 : off+4])
		off += 4
		m.Questions = append(m.Questions, q)
	}

	parseSection := func(count int) ([]Record, error) {
		records := make([]Record, 0, count)
		for i := 0; i < count; i++ {
			var r Record
			r.Name, off, err = decodeName(b, off)
			if err != nil {
				return nil, err
			}
			if off+10 > len(b) {
				return nil, ErrMalformed
			}
			r.Type = binary.BigEndian.Uint16(b[off : off+2])
			r.Class = binary.BigEndian.Uint16(b[off+2 : off+4])
			r.TTL = binary.BigEndian.Uint32(b[off+4 : off+8])
			rdlen := int(binary.BigEndian.Uint16(b[off+8 : off+10]))
			off += 10
			if off+rdlen > len(b) {
				return nil, ErrMalformed
			}
			r.Data = append([]byte(nil), b[off:off+rdlen]...)
			off += rdlen
			records = append(records, r)
		}
		return records, nil
	}
	if m.Answers, err = parseSection(an); err != nil {
		return nil, err
	}
	if m.Authorities, err = parseSection(ns); err != nil {
		return nil, err
	}
	if m.Additionals, err = parseSection(ar); err != nil {
		return nil, err
	}
	return m, nil
}

// decodeName decodes a possibly compressed name. It returns the name and the
// offset immediately after the name in the original stream.
func decodeName(b []byte, off int) (string, int, error) {
	if off < 0 || off >= len(b) {
		return "", 0, ErrMalformed
	}
	var labels []string
	pos := off
	end := -1
	hops := 0
	total := 0
	for {
		if pos >= len(b) {
			return "", 0, ErrMalformed
		}
		l := int(b[pos])
		if l == 0 {
			pos++
			if end < 0 {
				end = pos
			}
			break
		}
		if l&0xC0 == 0xC0 {
			if pos+1 >= len(b) {
				return "", 0, ErrMalformed
			}
			ptr := int(b[pos]&0x3F)<<8 | int(b[pos+1])
			if end < 0 {
				end = pos + 2
			}
			// Pointers must point backwards; this also rules out loops.
			if ptr >= pos || ptr < 12 {
				return "", 0, ErrMalformed
			}
			hops++
			if hops > maxPointerHops {
				return "", 0, ErrMalformed
			}
			pos = ptr
			continue
		}
		if l&0xC0 != 0 {
			return "", 0, ErrMalformed
		}
		pos++
		if l > maxLabelLength || pos+l > len(b) {
			return "", 0, ErrMalformed
		}
		label := string(b[pos : pos+l])
		total += len(label) + 1
		if total > 255 {
			return "", 0, ErrMalformed
		}
		labels = append(labels, label)
		pos += l
	}
	if total > 255 {
		return "", 0, ErrMalformed
	}
	return strings.ToLower(strings.Join(labels, ".")), end, nil
}

func packName(name string) ([]byte, error) {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	if name == "" {
		return []byte{0}, nil
	}
	if len(name) > maxNameLength {
		return nil, fmt.Errorf("DNS name too long: %q", name)
	}
	var out []byte
	for _, part := range strings.Split(name, ".") {
		if part == "" {
			return nil, fmt.Errorf("DNS name contains an empty label: %q", name)
		}
		if len(part) > maxLabelLength {
			return nil, fmt.Errorf("DNS label too long: %q", part)
		}
		out = append(out, byte(len(part)))
		out = append(out, part...)
	}
	return append(out, 0), nil
}

func packedNameLen(name string) int {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	if name == "" {
		return 1
	}
	return len(name) + 2 // one length byte per dot-separated label, plus root
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}
