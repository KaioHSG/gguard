package ggs

import (
	"testing"
	"time"
)

func TestParseFullScript(t *testing.T) {
	input := `guard "Skyrim Save Backup"
version 1.0
os windows
watch "{home}/Documents/My Games/Skyrim/Saves"
debounce 5s
zip {dest}/skyrim-{timestamp}.zip {watch}
message "Skyrim backup saved to \"{dest}\""`

	guard, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if guard.Name != "Skyrim Save Backup" {
		t.Errorf("expected name %q, got %q", "Skyrim Save Backup", guard.Name)
	}
	if guard.Version != "1.0" {
		t.Errorf("expected version %q, got %q", "1.0", guard.Version)
	}
	if guard.OS != "windows" {
		t.Errorf("expected OS %q, got %q", "windows", guard.OS)
	}
	if guard.Watch != "{home}/Documents/My Games/Skyrim/Saves" {
		t.Errorf("expected watch %q, got %q", "{home}/Documents/My Games/Skyrim/Saves", guard.Watch)
	}
	if guard.Debounce != 5*time.Second {
		t.Errorf("expected debounce %v, got %v", 5*time.Second, guard.Debounce)
	}

	if len(guard.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(guard.Commands))
	}

	zipCmd := guard.Commands[0]
	if zipCmd.Verb != "zip" || zipCmd.Args[0] != "{dest}/skyrim-{timestamp}.zip" || zipCmd.Args[1] != "{watch}" {
		t.Errorf("unexpected zip command: %+v", zipCmd)
	}

	msgCmd := guard.Commands[1]
	if msgCmd.Verb != "message" || msgCmd.Args[0] != "Skyrim backup saved to \"{dest}\"" {
		t.Errorf("unexpected message command: %+v", msgCmd)
	}
}

func TestParseCommentsAndEmptyLines(t *testing.T) {
	input := `
# backup routine
guard "test"
version 1.0

# platform
os linux

watch /tmp/saves
debounce 10s
message "done"
`

	guard, err := Parse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if guard.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", guard.Name)
	}
	if guard.OS != "linux" {
		t.Errorf("expected OS %q, got %q", "linux", guard.OS)
	}
	if guard.Debounce != 10*time.Second {
		t.Errorf("expected debounce %v, got %v", 10*time.Second, guard.Debounce)
	}
}

func TestParseMissingGuardName(t *testing.T) {
	_, err := Parse("guard\n")
	if err == nil {
		t.Fatal("expected error for missing guard name")
	}
}

func TestParseUnknownCommand(t *testing.T) {
	_, err := Parse(`guard "test"
version 1.0
os windows
watch /tmp
debounce 1s
nonsense
`)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestParseInvalidDebounce(t *testing.T) {
	_, err := Parse(`guard "test"
version 1.0
os windows
watch /tmp
debounce xyz
`)
	if err == nil {
		t.Fatal("expected error for invalid debounce duration")
	}
}