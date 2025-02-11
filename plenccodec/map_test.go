package plenccodec_test

import (
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	fuzz "github.com/google/gofuzz"
	"github.com/philpearl/plenc"
)

func TestMap(t *testing.T) {
	one := 1
	two := 2

	type mk struct {
		A int `plenc:"1"`
		B int `plenc:"2"`
	}

	type thing struct {
		M map[string]string `plenc:"1"`
		I int               `plenc:"2"`
	}

	tests := []struct {
		name string
		data interface{}
	}{
		{
			name: "string,string",
			data: map[string]string{
				"hat": "hut",
				"it":  "sit",
			},
		},
		{
			name: "string,string empty",
			data: map[string]string{},
		},
		{
			name: "string,string empty strings",
			data: map[string]string{
				"hat": "",
				"it":  "",
				"":    "elephant",
			},
		},
		{
			name: "string,int",
			data: map[string]int{
				"hat": 0,
				"it":  32,
			},
		},
		{
			name: "string,*int,2 things",
			data: map[string]*int{
				"hat": nil,
				"it":  &one,
			},
		},
		{
			name: "string,*int,3 things",
			data: map[string]*int{
				"hat": nil,
				"it":  &one,
				"at":  &two,
			},
		},
		{
			name: "string,*int just nil",
			data: map[string]*int{
				"hat": nil,
			},
		},
		{
			name: "float,*int",
			data: map[float32]*int{
				0:     nil,
				1.001: &one,
			},
		},
		{
			name: "struct,*int",
			data: map[mk]*int{
				{A: 1, B: 7}: nil,
				{A: 1, B: 3}: &one,
			},
		},
		{
			name: "nil map",
			data: map[string]string(nil),
		},
		{
			name: "map in struct nil",
			data: thing{},
		},
		{
			name: "map in struct empty",
			data: thing{M: map[string]string{}},
		},
		{
			name: "map in struct",
			data: thing{
				M: map[string]string{"A": "B"},
			},
		},
		{
			name: "map in struct with following",
			data: thing{
				M: map[string]string{"A": "B"},
				I: 100,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := plenc.Marshal(nil, test.data)
			if err != nil {
				t.Fatal(err)
			}

			mv := reflect.New(reflect.TypeOf(test.data))
			if err := plenc.Unmarshal(data, mv.Interface()); err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(test.data, mv.Elem().Interface()); diff != "" {
				t.Fatalf("result differs: %s", diff)
			}
		})
	}
}

func TestMapZero(t *testing.T) {
	// This matches the behaviour of the standard json package, but is different
	// to unmarshalling into a struct where the struct is always zeroed first
	m := map[string]string{"hat": "pineapple"}
	n := map[string]string{"cheese": "monkey"}
	data, err := plenc.Marshal(nil, n)
	if err != nil {
		t.Fatal(err)
	}
	if err := plenc.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(map[string]string{
		"hat":    "pineapple",
		"cheese": "monkey",
	}, m); diff != "" {
		t.Fatal(diff)
	}
}

func TestMapFuzz(t *testing.T) {
	fz := fuzz.New()

	for i := 0; i < 100; i++ {
		var in, out map[string]string
		fz.Fuzz(&in)

		data, err := plenc.Marshal(nil, in)
		if err != nil {
			t.Fatal(err)
		}

		if err := plenc.Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(in, out); diff != "" {
			t.Fatalf("result differs (%d): %s", i, diff)
		}
	}
}

func BenchmarkMap(b *testing.B) {
	m := map[string]string{
		"AAA":  "AAA",
		"AAA1": "AAA",
		"AAA2": "AAA",
		"AAA3": "AAA",
		"AAA4": "AAA",
		"AAA5": "AAA",
		"AAA6": "AAA",
		"AAA7": "AAA",
	}

	b.ResetTimer()
	b.ReportAllocs()
	var data []byte
	var o map[string]string
	for i := 0; i < b.N; i++ {
		var err error
		data, err = plenc.Marshal(data[:0], m)
		if err != nil {
			b.Fatal(err)
		}
		for k := range o {
			delete(o, k)
		}
		if err := plenc.Unmarshal(data, &o); err != nil {
			b.Fatal(err)
		}
	}
}
