package bots

import (
	"testing"
)

func TestParseRuGramCommand(t *testing.T) {
	tests := []struct {
		body, command, argument string
	}{
		{body: "/account @testing", command: "account", argument: "@testing"},
		{body: "/freeze@rugram 1780243437", command: "freeze", argument: "1780243437"},
		{body: "hello", command: "", argument: ""},
	}
	for _, tt := range tests {
		command, argument := parseRuGramCommand(tt.body)
		if command != tt.command || argument != tt.argument {
			t.Fatalf("parseRuGramCommand(%q) = (%q, %q), want (%q, %q)", tt.body, command, argument, tt.command, tt.argument)
		}
	}
}

func TestParseRuGramCallback(t *testing.T) {
	targetID, frozen, ok := parseRuGramCallback([]byte("gs:account:1780243437:1"))
	if !ok || targetID != 1780243437 || !frozen {
		t.Fatalf("valid freeze callback = (%d, %t, %t)", targetID, frozen, ok)
	}
	for _, data := range [][]byte{
		[]byte("gs:account:1780243437:2"),
		[]byte("gs:account:0:1"),
		[]byte("other:account:1780243437:1"),
	} {
		if _, _, ok := parseRuGramCallback(data); ok {
			t.Fatalf("parseRuGramCallback(%q) accepted invalid data", data)
		}
	}
}

func TestRuGramChatAllowlistIsNumeric(t *testing.T) {
	svc := &Service{rugramAdminChatIDs: map[int64]struct{}{1780243437: {}}}
	if !svc.rugramChatAllowed(1780243437) {
		t.Fatal("configured chat_id was rejected")
	}
	if svc.rugramChatAllowed(1780243438) || svc.rugramChatAllowed(0) {
		t.Fatal("unconfigured chat_id was accepted")
	}
}
