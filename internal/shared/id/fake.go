package id

import "strconv"

// Fake hands out predictable identifiers so a test can assert on them.
type Fake struct {
	prefix string
	next   int
}

func NewFake(prefix string) *Fake {
	return &Fake{prefix: prefix}
}

func (f *Fake) Create() string {
	f.next++
	return f.prefix + strconv.Itoa(f.next)
}
