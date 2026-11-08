package main

import "testing"

func TestWindowWatch_StartupRefusalsDoNotCloseTheApp(t *testing.T) {
	var w windowWatch
	for i := 0; i < windowStartupPolls-1; i++ {
		if w.observe(false) {
			t.Fatalf("terminated at poll %d before any page was seen", i+1)
		}
	}
}

func TestWindowWatch_NeverShowsAPageGivesUp(t *testing.T) {
	var w windowWatch
	var closed bool
	for i := 0; i < windowStartupPolls; i++ {
		closed = w.observe(false)
	}
	if !closed {
		t.Fatal("a Chrome that never showed a page must be closed after the startup bound")
	}
}

func TestWindowWatch_ClosedWindowNeedsTwoEmptyPolls(t *testing.T) {
	var w windowWatch
	w.observe(false)
	w.observe(true)
	if w.observe(false) {
		t.Fatal("one empty poll after a page was seen must not close (debounce)")
	}
	if !w.observe(false) {
		t.Fatal("two empty polls after a page was seen must close")
	}
}

func TestWindowWatch_PageReturningResetsTheDebounce(t *testing.T) {
	var w windowWatch
	w.observe(true)
	w.observe(false)
	w.observe(true)
	if w.observe(false) {
		t.Fatal("a page seen in between must reset the empty-poll count")
	}
}
