package parser

import (
	"reflect"
	"testing"
	"unsafe"
)

func TestSetSingleLineMatchesSet(t *testing.T) {
	for _, tc := range []struct{ old, source string }{
		{"", "plain paragraph with **emphasis**"},
		{"", "![image](data:image/png;base64,AAAA)"},
		{"", "![image](data:image/png;base64,AAAA)\n"},
		{"", "![image](data:image/png;base64,AAAA)\r\n"},
		{"", "## heading with `code`"},
		{"plain", "plain paragraph extended after its first version"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			regular, fast := New(Default), New(Default)
			regular.Set(tc.old)
			fast.Set(tc.old)
			regular.Set(tc.source)
			fast.SetSingleLine(tc.source)
			if regular.Source() != fast.Source() || regular.HTML() != fast.HTML() ||
				!reflect.DeepEqual(regular.blocks, fast.blocks) || !reflect.DeepEqual(regular.segs, fast.segs) {
				t.Fatal("SetSingleLine changed parsed document semantics")
			}
		})
	}
}

func TestSetAliasesInitialSourceThenAppendAndEditStayCorrect(t *testing.T) {
	d := New(Default)
	src := "first paragraph\n\n**注意：**内容\n"
	d.Set(src)
	if got := d.Source(); got != src || unsafe.StringData(got) != unsafe.StringData(src) {
		t.Fatal("initial Set copied or changed its immutable source")
	}

	d.Append("\nsecond paragraph\n")
	appended := src + "\nsecond paragraph\n"
	if got := d.Source(); got != appended {
		t.Fatalf("source after Append = %q, want %q", got, appended)
	}

	d.Edit(0, len("first"), "start")
	want := "start paragraph\n\n**注意：**内容\n\nsecond paragraph\n"
	if got := d.Source(); got != want {
		t.Fatalf("source after Edit = %q, want %q", got, want)
	}
}
