package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestQuoteEnterBeforeCallout(t *testing.T) {
	const src = "> 第一行\n\n> [!NOTE] 提示\n> callout body\n"
	h := paragraphEditingHarness(t, src)
	at := strings.Index(src, "\n")
	h.caretAt(at)
	h.shot("quote-before-enter")
	h.key(0, ui.KeyEnter)
	h.shot("quote-after-enter")
	h.want("> 第一行\n> \n\n> [!NOTE] 提示\n> callout body\n")
	u := h.e.unitAt(h.e.caret)
	if u == nil || u.kind != kBlank {
		t.Fatalf("caret did not land on new blank quote line: %+v", u)
	}
	for _, c := range u.conts {
		if isCalloutContainer(c) {
			t.Fatal("new quote line incorrectly belongs to following callout")
		}
	}
	if h.e.caret != at+len("\n> ") {
		t.Fatalf("caret=%d want=%d", h.e.caret, at+len("\n> "))
	}
	r, ok := h.e.caretRect(h.e.caret)
	for _, l := range h.e.leaves {
		if _, _, callout := h.e.calloutHeader(l); callout && (!ok || r.Y+r.H > l.y) {
			t.Fatalf("quote caret overlaps following callout: caret=%+v callout y=%v", r, l.y)
		}
	}
	h.typ("第二行")
	h.want("> 第一行\n> 第二行\n\n> [!NOTE] 提示\n> callout body\n")
}

func TestQuoteEnterAdjacentCallout(t *testing.T) {
	const src = "> 第一行\n> [!NOTE] 提示\n> callout body\n"
	h := paragraphEditingHarness(t, src)
	at := strings.Index(src, "\n")
	h.caretAt(at)
	h.key(0, ui.KeyEnter)
	h.want("> 第一行\n> \n> [!NOTE] 提示\n> callout body\n")
	h.typ("第二行")
	h.want("> 第一行\n> 第二行\n> [!NOTE] 提示\n> callout body\n")
}

func TestBlankQuoteBeforeCalloutKeepsItsOwnContainers(t *testing.T) {
	for _, prefix := range []string{"> ", "> > "} {
		src := prefix + "\n\n> [!NOTE] 提示\n> body"
		h := paragraphEditingHarness(t, src)
		h.caretAt(len(prefix))
		u := h.e.unitAt(h.e.caret)
		if u == nil || len(u.conts) != strings.Count(prefix, ">") {
			t.Fatalf("standalone quote lost containers: %+v", u)
		}
		for _, c := range u.conts {
			if isCalloutContainer(c) {
				t.Fatal("standalone quote borrowed following callout")
			}
		}
		h.typ("text")
		h.want(prefix + "text\n\n> [!NOTE] 提示\n> body")
	}
}

func TestQuoteEnterAfterSoftContinuation(t *testing.T) {
	const quote = "> 普通引用。\n> 第二行。"
	const tail = "\n\n> [!NOTE] 提示\n> callout 的第一段。\n>\n> callout 的第二段。"
	h := paragraphEditingHarness(t, quote+tail)
	h.caretAt(len(quote))
	h.key(0, ui.KeyEnter)
	h.want(quote + "\n> " + tail)
	u := h.e.unitAt(h.e.caret)
	if u == nil || len(u.conts) != 1 || isCalloutContainer(u.conts[0]) {
		t.Fatalf("continuation Enter moved into callout: %+v", u)
	}
	h.shot("quote-soft-continuation-enter")
	h.e.Undo()
	h.frame()
	h.want(quote + tail)
	h.e.Redo()
	h.frame()
	h.typ("第三行。")
	h.want(quote + "\n> 第三行。" + tail)
}
