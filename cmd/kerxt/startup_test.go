package main

import "testing"

func TestJoinStartupOptions(t *testing.T) {
	options, err := runStartupCommand([]string{"join", "--repl", "lounge", "--debug", "ignored"}, "", startupOptions{})
	if err != nil || options.handled || !options.repl || !options.debug || options.initialRoom != "lounge" {
		t.Fatalf("unexpected join options: %+v, %v", options, err)
	}
	options, err = runStartupCommand(nil, "", startupOptions{repl: true})
	if err != nil || !options.repl || options.handled {
		t.Fatalf("default options were lost: %+v, %v", options, err)
	}
}

func TestPurgeFlag(t *testing.T) {
	for _, flag := range []string{"--purge", "-purge"} {
		if !hasPurgeFlag([]string{flag}) {
			t.Errorf("purge flag not recognized: %s", flag)
		}
	}
	if hasPurgeFlag([]string{"purge"}) || hasPurgeFlag(nil) {
		t.Fatal("purge enabled without an explicit flag")
	}
}
