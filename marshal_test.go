package plenc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	fuzz "github.com/google/gofuzz"
)

type InnerThing struct {
	A string    `plenc:"1"`
	B float64   `plenc:"2"`
	C time.Time `plenc:"3"`
}

type SliceThing []InnerThing

type TestThing struct {
	A   float64     `plenc:"1"`
	B   []float64   `plenc:"2"`
	C   *float64    `plenc:"3"`
	D   float32     `plenc:"4"`
	E   []float32   `plenc:"5"`
	F   *float32    `plenc:"6"`
	G   int         `plenc:"7"`
	H   []int       `plenc:"8"`
	I   *int        `plenc:"9"`
	J   uint        `plenc:"10"`
	K   []uint      `plenc:"11"`
	L   *uint       `plenc:"12"`
	M   bool        `plenc:"13"`
	N   []bool      `plenc:"14"`
	O   *bool       `plenc:"15"`
	P   string      `plenc:"16"`
	Q   []string    `plenc:"17"`
	R   *string     `plenc:"18"`
	S   time.Time   `plenc:"19"`
	T   []time.Time `plenc:"20"`
	U   *time.Time  `plenc:"21"`
	V   int32       `plenc:"22"`
	W   []int32     `plenc:"23"`
	X   *int32      `plenc:"24"`
	Y   int64       `plenc:"25"`
	Z   []int64     `plenc:"26"`
	A1  *int64      `plenc:"27"`
	A2  int16       `plenc:"29"`
	A3  []int16     `plenc:"30"`
	A4  *int16      `plenc:"31"`
	A5  uint8       `plenc:"32"`
	A6  []uint8     `plenc:"33"`
	A7  *uint8      `plenc:"34"`
	A8  int8        `plenc:"37"`
	A9  []int8      `plenc:"38"`
	A10 *int8       `plenc:"39"`
	A11 uint64      `plenc:"40"`
	A12 []uint64    `plenc:"41"`
	A13 *uint64     `plenc:"42"`
	A14 uint16      `plenc:"43"`
	A15 []uint16    `plenc:"44"`
	A16 *uint16     `plenc:"45"`

	Z1 InnerThing   `plenc:"28"`
	Z2 []InnerThing `plenc:"35"`
	Z3 *InnerThing  `plenc:"36"`
	ZZ SliceThing   `plenc:"46"`
}

func TestMarshal(t *testing.T) {

	f := fuzz.New()
	for i := 0; i < 10000; i++ {
		var in TestThing
		f.Fuzz(&in)

		data, err := Marshal(nil, &in)
		if err != nil {
			t.Fatal(err)
		}

		var out TestThing
		if err := Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(in, out); diff != "" {
			t.Logf("%x", data)

			var out TestThing
			if err := Unmarshal(data, &out); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(in, out); diff != "" {
				t.Logf("re-run differs too")
			} else {
				t.Logf("re-run does not differ")
			}

			t.Fatalf("structs differ. %s", diff)
		}
	}
}

func TestMarshalSlice(t *testing.T) {
	a := SliceThing{
		{A: "a"},
	}

	data, err := Marshal(nil, a)
	if err != nil {
		t.Fatal(err)
	}
	var b SliceThing

	if err := Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
}

func TestSkip(t *testing.T) {
	f := fuzz.New()
	for i := 0; i < 100; i++ {
		var in TestThing
		f.Fuzz(&in)

		data, err := Marshal(nil, &in)
		if err != nil {
			t.Fatal(err)
		}

		// This should skip everything, but we don't know unless it errors
		type nowt struct{}
		var nothing nowt
		if err := Unmarshal(data, &nothing); err != nil {
			t.Fatal(err)
		}

		// So lets do a lower level skip
		i := 0
		for i < len(data) {
			wt, _, n := ReadTag(data[i:])
			if n < 0 {
				t.Fatalf("problem reading tag")
			}
			i += n
			n, err := Skip(data[i:], wt)
			if err != nil {
				t.Fatal(err)
			}
			i += n
		}
		if i != len(data) {
			t.Fatal("data length not as expected")
		}
	}
}

func TestMarshalUnmarked(t *testing.T) {
	type unmarked struct {
		A string
	}

	var in unmarked
	_, err := Marshal(nil, &in)
	if err == nil {
		t.Errorf("expected an error as field has no plenc tag")
	}
	if err.Error() != "no plenc tag on field 0 A of unmarked" {
		t.Errorf("error %q not as expected", err)
	}
}

func TestMarshalDuplicate(t *testing.T) {
	type duplicate struct {
		A string  `plenc:"1"`
		B *string `plenc:"1"`
	}

	var in duplicate
	_, err := Marshal(nil, &in)
	if err == nil {
		t.Errorf("expected an error as fields have duplicate plenc tags")
	}
	if err.Error() != "failed building codec for duplicate. Multiple fields have index 1" {
		t.Errorf("error %q not as expected", err)
	}
}

func TestMarshalComplex(t *testing.T) {
	type my struct {
		A complex64 `plenc:"1"`
	}

	var in my
	_, err := Marshal(nil, &in)
	if err == nil {
		t.Errorf("expected an error as complex types aren't supported")
	}
	if err.Error() != "failed to find codec for field 0 (A) of my. could not find or create a codec for complex64" {
		t.Errorf("error %q not as expected", err)
	}
}

func TestUnMarshalComplex(t *testing.T) {
	type my struct {
		A complex64 `plenc:"1"`
	}

	var in my
	err := Unmarshal(nil, &in)
	if err == nil {
		t.Errorf("expected an error as complex types aren't supported")
	}
	if err.Error() != "failed to find codec for field 0 (A) of my. could not find or create a codec for complex64" {
		t.Errorf("error %q not as expected", err)
	}
}

func TestUnmarshalNoPtr(t *testing.T) {
	var a int
	err := Unmarshal([]byte{}, a)
	if err == nil {
		t.Fatal("expected an error from unmarshal as is requires a pointer")
	}
	if err.Error() != "you must pass in a non-nil pointer" {
		t.Errorf("error %q not as expected", err)
	}
}

func TestUnmarshalNilPtr(t *testing.T) {
	var a *int
	err := Unmarshal([]byte{}, a)
	if err == nil {
		t.Fatal("expected an error from unmarshal as is requires a pointer")
	}
	if err.Error() != "you must pass in a non-nil pointer" {
		t.Errorf("error %q not as expected", err)
	}
}

func BenchmarkCycle(b *testing.B) {
	f := fuzz.New()
	var in TestThing
	f.Fuzz(&in)

	b.Run("plenc", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			var data []byte
			for pb.Next() {
				var err error
				data, err = Marshal(data[:0], &in)
				if err != nil {
					b.Fatal(err)
				}
				var out TestThing
				if err := Unmarshal(data, &out); err != nil {
					b.Fatal(err)
				}
			}
		})
	})

	b.Run("json", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				var err error
				data, err := json.Marshal(&in)
				if err != nil {
					b.Fatal(err)
				}
				var out TestThing
				if err := json.Unmarshal(data, &out); err != nil {
					b.Fatal(err)
				}
			}
		})
	})
}

func TestNamedTypes(t *testing.T) {
	type Bool bool
	type Int int
	type Int64 int64
	type Int32 int32
	type Int16 int16
	type Int8 int8
	type Float64 float64
	type Float32 float32
	type Uint uint
	type Uint64 uint64
	type Uint32 uint32
	type Uint16 uint16
	type Uint8 uint8
	type String string

	type MyStruct struct {
		V1  Bool    `plenc:"1"`
		V2  Int     `plenc:"2"`
		V3  Float64 `plenc:"3"`
		V4  Float32 `plenc:"4"`
		V5  Uint    `plenc:"5"`
		V6  String  `plenc:"6"`
		V7  Int64   `plenc:"7"`
		V8  Int32   `plenc:"8"`
		V9  Int16   `plenc:"9"`
		V10 Int8    `plenc:"10"`
		V11 Uint64  `plenc:"11"`
		V12 Uint32  `plenc:"12"`
		V13 Uint16  `plenc:"13"`
		V14 Uint8   `plenc:"14"`
	}

	var in, out MyStruct

	f := fuzz.New()
	f.Fuzz(&in)

	data, err := Marshal(nil, &in)
	if err != nil {
		t.Fatal(err)
	}

	if err := Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(in, out); diff != "" {
		t.Fatalf("results differ. %s", diff)
	}
}
