package main

import "testing"

func TestAdd(t *testing.T) {
	expected := 10
	actual := Add(4, 6)

	if actual != expected {
		t.Errorf("got %q, wanted %q", actual, expected)
	}
}
