package plenc

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/philpearl/plenc/plenccodec"
)

// Marshal serialises value, appending it to data. If data is nil a new slice is
// created.
func Marshal(data []byte, value any) ([]byte, error) {
	return defaultPlenc.Marshal(data, value)
}

// Unmarshal deserialises data into value.
func Unmarshal(data []byte, value any) error {
	return defaultPlenc.Unmarshal(data, value)
}

// Size returns the number of bytes required to marshal the value.
func Size(value any) (int, error) {
	return defaultPlenc.Size(value)
}

func (p *Plenc) preamble(value any) (unsafe.Pointer, plenccodec.Codec, error) {
	typ := reflect.TypeOf(value)
	ptr := unpackEFace(value).data
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()

		// When marshalling we don't want a pointer to a map as a map is a
		// pointer-ish type itself.
		if typ.Kind() == reflect.Map {
			ptr = *(*unsafe.Pointer)(ptr)
		}
	}

	c, err := p.CodecForType(typ)
	if err != nil {
		return nil, nil, err
	}

	return ptr, c, nil
}

func (p *Plenc) Marshal(data []byte, value any) ([]byte, error) {
	ptr, c, err := p.preamble(value)
	if err != nil {
		return nil, err
	}

	if c.Omit(ptr) {
		return nil, nil
	}
	if data == nil {
		data = make([]byte, 0, c.Size(ptr, nil))
	}

	return c.Append(data, ptr, nil), nil
}

func (p *Plenc) Unmarshal(data []byte, value any) error {
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("you must pass in a non-nil pointer")
	}

	c, err := p.CodecForType(rv.Type().Elem())
	if err != nil {
		return err
	}

	_, err = c.Read(data, rv.UnsafePointer(), c.WireType())
	return err
}

func (p *Plenc) Size(value any) (int, error) {
	ptr, c, err := p.preamble(value)
	if err != nil {
		return 0, err
	}

	if c.Omit(ptr) {
		return 0, nil
	}

	return c.Size(ptr, nil), nil
}
