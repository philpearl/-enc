package plenccodec

import (
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/philpearl/plenc/plenccore"
)

// MapCodec is a codec for maps. We treat it as a slice of structs with the key
// and value as the fields in the structs.
type MapCodec struct {
	keyCodec   Codec
	valueCodec Codec
	rtype      reflect.Type
	keyTag     []byte
	valueTag   []byte
	kPool      sync.Pool
	kZero      unsafe.Pointer
	vZero      unsafe.Pointer
}

func BuildMapCodec(p CodecBuilder, registry CodecRegistry, typ reflect.Type, tag string) (Codec, error) {
	if typ.Kind() != reflect.Map {
		return nil, fmt.Errorf("type must be a map to build a map codec")
	}

	keyCodec, err := p.CodecForTypeRegistry(registry, typ.Key(), "")
	if err != nil {
		return nil, fmt.Errorf("failed to find codec for map key %s. %w", typ.Key().Name(), err)
	}
	valueCodec, err := p.CodecForTypeRegistry(registry, typ.Elem(), "")
	if err != nil {
		return nil, fmt.Errorf("failed to find codec for map value %s. %w", typ.Elem().Name(), err)
	}

	c := MapCodec{
		keyCodec:   keyCodec,
		valueCodec: valueCodec,
		rtype:      typ,
		keyTag:     plenccore.AppendTag(nil, keyCodec.WireType(), 1),
		valueTag:   plenccore.AppendTag(nil, valueCodec.WireType(), 2),
	}

	c.kPool.New = c.newKey
	if l := int(typ.Key().Size()); l <= len(zero) {
		c.kZero = unsafe.Pointer(&zero)
	} else {
		z := make([]byte, l)
		c.kZero = unsafe.Pointer(&z[0])
	}

	if l := int(typ.Elem().Size()); l <= len(zero) {
		c.vZero = unsafe.Pointer(&zero)
	} else {
		z := make([]byte, l)
		c.vZero = unsafe.Pointer(&z[0])
	}

	if tag == "proto" {
		return ProtoMapCodec{&c}, nil
	}

	return &c, nil
}

func (c *MapCodec) newKey() any {
	return c.keyCodec.New()
}

// When we're writing ptr is a map pointer. When reading it is a pointer to a
// map pointer

func (c *MapCodec) Omit(ptr unsafe.Pointer) bool {
	return ptr == nil
}

func (c *MapCodec) size(ptr unsafe.Pointer) (size int) {
	mv := reflect.NewAt(c.rtype, unsafe.Pointer(&ptr)).Elem()
	size = plenccore.SizeVarUint(uint64(mv.Len()))

	iter := mv.MapRange()

	kv := reflect.New(c.rtype.Key()).Elem()
	vv := reflect.New(c.rtype.Elem()).Elem()

	for iter.Next() {
		kv.SetIterKey(iter)
		vv.SetIterValue(iter)

		k := kv.Addr().UnsafePointer()
		v := vv.Addr().UnsafePointer()

		s := c.sizeForEntry(k, v)
		size += plenccore.SizeVarUint(uint64(s)) + s

	}
	return size
}

func (c *MapCodec) sizeForEntry(k, v unsafe.Pointer) int {
	s := c.sizeFor(c.keyCodec, k, c.keyTag)
	return s + c.sizeFor(c.valueCodec, v, c.valueTag)
}

func (*MapCodec) sizeFor(underlying Codec, ptr unsafe.Pointer, tag []byte) int {
	if underlying.Omit(ptr) {
		return 0
	}
	return underlying.Size(ptr, tag)
}

func (c *MapCodec) append(data []byte, ptr unsafe.Pointer) []byte {
	add := func(underlying Codec, ptr unsafe.Pointer, tag []byte) {
		if !underlying.Omit(ptr) {
			data = underlying.Append(data, ptr, tag)
		}
	}
	mv := reflect.NewAt(c.rtype, unsafe.Pointer(&ptr)).Elem()

	// First add the count of entries
	data = plenccore.AppendVarUint(data, uint64(mv.Len()))

	iter := mv.MapRange()

	kv := reflect.New(c.rtype.Key()).Elem()
	vv := reflect.New(c.rtype.Elem()).Elem()

	for iter.Next() {
		kv.SetIterKey(iter)
		vv.SetIterValue(iter)

		k := kv.Addr().UnsafePointer()
		v := vv.Addr().UnsafePointer()

		// Add the length of each entry, then the key and value
		data = plenccore.AppendVarUint(data, uint64(c.sizeForEntry(k, v)))
		add(c.keyCodec, k, c.keyTag)
		add(c.valueCodec, v, c.valueTag)
	}

	return data
}

var zero [1024]byte

func (c *MapCodec) Read(data []byte, ptr unsafe.Pointer, wt plenccore.WireType) (n int, err error) {
	if len(data) == 0 {
		return 0, nil
	}

	// We start with a count of entries
	count, n := plenccore.ReadVarUint(data)
	if n <= 0 {
		return 0, fmt.Errorf("failed to read map size")
	}

	// ptr is a pointer to a map pointer
	mv := reflect.NewAt(c.rtype, ptr).Elem()
	if mv.IsNil() {
		mv.Set(reflect.MakeMapWithSize(c.rtype, int(count)))
	}

	// We need some space to hold keys and values as we read them out. We can
	// re-use the space on each iteration as the data is copied into the map
	// We also save some memory & time if we cache them in some pools
	k := c.kPool.Get().(unsafe.Pointer)
	defer c.kPool.Put(k)
	kv := reflect.NewAt(c.rtype.Key(), k).Elem()
	vv := reflect.New(c.rtype.Elem()).Elem() // TODO: could save an allocation here too

	offset := int(n)
	for count > 0 {
		// Each entry starts with a length
		entryLength, n := plenccore.ReadVarUint(data[offset:])
		if n <= 0 {
			return 0, fmt.Errorf("failed to read map entry length")
		}

		vv.Set(reflect.Zero(vv.Type()))

		offset += n
		n, err := c.readMapEntry(mv, kv, vv, data[offset:offset+int(entryLength)])
		if err != nil {
			return 0, err
		}
		offset += n
		count--
	}

	return offset, nil
}

// readMapEntry reads out a single map entry. mp is the map pointer. k is an
// area to read key values into. data is the raw data for this map entry
func (c *MapCodec) readMapEntry(mv, kv, vv reflect.Value, data []byte) (int, error) {
	offset, fieldEnd, index, wt, err := c.readTagAndLength(data, 0)
	if err != nil {
		return 0, err
	}

	if index == 1 {
		// Key is present - read it
		k := kv.Addr().UnsafePointer()
		n, err := c.keyCodec.Read(data[offset:fieldEnd], k, wt)
		if err != nil {
			return 0, fmt.Errorf("failed reading key field of %s. %w", c.rtype.Name(), err)
		}
		offset += n
	} else {
		kv.Set(reflect.Zero(kv.Type()))
	}

	if offset < len(data) {
		if index == 1 {
			offset, fieldEnd, _, wt, err = c.readTagAndLength(data, offset)
			if err != nil {
				return 0, err
			}
		}

		val := vv.Addr().UnsafePointer()
		n, err := c.valueCodec.Read(data[offset:fieldEnd], val, wt)
		if err != nil {
			return 0, fmt.Errorf("failed reading value field of %s. %w", c.rtype.Name(), err)
		}
		offset += n
	}

	mv.SetMapIndex(kv, vv)

	return offset, nil
}

func (c *MapCodec) readTagAndLength(data []byte, offset int) (offset2, fieldEnd, index int, wt plenccore.WireType, err error) {
	wt, index, n := plenccore.ReadTag(data[offset:])
	offset += n
	fieldEnd = len(data)
	if wt == plenccore.WTLength {
		// For WTLength types we read out the length and ensure the data we
		// read the field from is the right length
		fieldLen, n := plenccore.ReadVarUint(data[offset:])
		if n <= 0 {
			return 0, 0, 0, wt, fmt.Errorf("varuint overflow reading %d of %s", index, c.rtype.Name())
		}
		offset += n
		fieldEnd = int(fieldLen) + offset
		if fieldEnd > len(data) {
			return 0, 0, 0, wt, fmt.Errorf("length %d of field %d of %s exceeds data length %d", fieldLen, index, c.rtype.Name(), len(data)-offset)
		}
	}

	return offset, fieldEnd, index, wt, nil
}

func (c *MapCodec) New() unsafe.Pointer {
	return unsafe.Pointer(reflect.MakeMap(c.rtype).Pointer())
}

func (c *MapCodec) WireType() plenccore.WireType {
	return plenccore.WTSlice
}

func (c *MapCodec) Descriptor() Descriptor {
	// We treat this as a slice of structs? Perhaps need to define a map descriptor!
	kDesc := c.keyCodec.Descriptor()
	vDesc := c.valueCodec.Descriptor()

	kDesc.Index = 1
	kDesc.Name = "key"
	vDesc.Index = 2
	vDesc.Name = "value"

	kTypeName, vTypeName := kDesc.TypeName, vDesc.TypeName
	if kTypeName == "" {
		kTypeName = kDesc.Type.String()
	}
	if vTypeName == "" {
		vTypeName = vDesc.Type.String()
	}

	return Descriptor{
		Type:        FieldTypeSlice,
		LogicalType: LogicalTypeMap,
		Elements: []Descriptor{
			{
				Type:        FieldTypeStruct,
				LogicalType: LogicalTypeMapEntry,
				TypeName:    fmt.Sprintf("map_%s_%s", kTypeName, vTypeName),
				Elements: []Descriptor{
					kDesc,
					vDesc,
				},
			},
		},
	}
}

func (c *MapCodec) Size(ptr unsafe.Pointer, tag []byte) int {
	return c.size(ptr) + len(tag)
}

func (c *MapCodec) Append(data []byte, ptr unsafe.Pointer, tag []byte) []byte {
	data = append(data, tag...)
	return c.append(data, ptr)
}

type ProtoMapCodec struct {
	*MapCodec
}

func (c ProtoMapCodec) Size(ptr unsafe.Pointer, tag []byte) (size int) {
	// Treat as an array of structs. Each entry carries its own tag
	mv := reflect.NewAt(c.rtype, unsafe.Pointer(&ptr)).Elem()
	size = plenccore.SizeVarUint(uint64(mv.Len()))

	iter := mv.MapRange()

	kv := reflect.New(c.rtype.Key()).Elem()
	vv := reflect.New(c.rtype.Elem()).Elem()

	for iter.Next() {
		kv.SetIterKey(iter)
		vv.SetIterValue(iter)

		k := kv.Addr().UnsafePointer()
		v := vv.Addr().UnsafePointer()

		s := c.sizeForEntry(k, v)
		size += len(tag) + plenccore.SizeVarUint(uint64(s)) + s

	}
	return size
}

func (c ProtoMapCodec) Append(data []byte, ptr unsafe.Pointer, tag []byte) []byte {
	// Each entry is appended separately as if a struct of key & value

	add := func(underlying Codec, ptr unsafe.Pointer, tag []byte) {
		if !underlying.Omit(ptr) {
			data = underlying.Append(data, ptr, tag)
		}
	}

	mv := reflect.NewAt(c.rtype, unsafe.Pointer(&ptr)).Elem()

	// First add the count of entries
	data = plenccore.AppendVarUint(data, uint64(mv.Len()))

	iter := mv.MapRange()

	kv := reflect.New(c.rtype.Key()).Elem()
	vv := reflect.New(c.rtype.Elem()).Elem()

	for iter.Next() {
		kv.SetIterKey(iter)
		vv.SetIterValue(iter)

		k := kv.Addr().UnsafePointer()
		v := vv.Addr().UnsafePointer()

		data = append(data, tag...)
		data = plenccore.AppendVarUint(data, uint64(c.sizeForEntry(k, v)))
		add(c.keyCodec, k, c.keyTag)
		add(c.valueCodec, v, c.valueTag)
	}

	return data
}

func (c ProtoMapCodec) Read(data []byte, ptr unsafe.Pointer, wt plenccore.WireType) (n int, err error) {
	if len(data) == 0 {
		return 0, nil
	}

	// ptr is a pointer to a map pointer
	mv := reflect.NewAt(c.rtype, ptr).Elem()
	if mv.IsNil() {
		mv.Set(reflect.MakeMap(c.rtype))
	}

	// We need some space to hold keys and values as we read them out. We can
	// re-use the space on each iteration as the data is copied into the map
	// We also save some memory & time if we cache them in some pools
	k := c.kPool.Get().(unsafe.Pointer)
	defer c.kPool.Put(k)
	kv := reflect.NewAt(c.rtype.Key(), k).Elem()
	vv := reflect.New(c.rtype.Elem()).Elem()

	return c.readMapEntry(mv, kv, vv, data)
}

func (c ProtoMapCodec) WireType() plenccore.WireType {
	return plenccore.WTLength
}
