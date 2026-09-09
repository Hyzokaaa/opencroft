// Package id provides the identifier value object used across the domain.
package id

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
	"time"
)

// Id wraps a ULID string. Domain entities never expose the raw value directly.
type Id struct {
	value string
}

func New(value string) Id {
	return Id{value: value}
}

func (i Id) Get() string {
	return i.value
}

func (i Id) IsZero() bool {
	return i.value == ""
}

// Generator hands out identifiers. Tests substitute a fake.
type Generator interface {
	Create() string
}

type ULIDGenerator struct{}

func NewULIDGenerator() *ULIDGenerator {
	return &ULIDGenerator{}
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Create returns a lexicographically sortable identifier: 48 bits of
// millisecond timestamp followed by 80 bits of randomness, Crockford base32.
func (g *ULIDGenerator) Create() string {
	var buf [16]byte
	binary.BigEndian.PutUint64(buf[:8], uint64(time.Now().UnixMilli())<<16)
	if _, err := rand.Read(buf[6:]); err != nil {
		panic("id: no entropy available: " + err.Error())
	}

	var sb strings.Builder
	sb.Grow(26)

	var acc, bits uint16
	for _, b := range buf {
		acc = acc<<8 | uint16(b)
		bits += 8
		for bits >= 5 {
			bits -= 5
			sb.WriteByte(crockford[(acc>>bits)&0x1f])
		}
	}
	if bits > 0 {
		sb.WriteByte(crockford[(acc<<(5-bits))&0x1f])
	}
	return sb.String()[:26]
}
