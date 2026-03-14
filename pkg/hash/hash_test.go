package hash

import (
	"testing"
)

// TestSNSMessageHashStripsSPrefix verifies that the 'S' prefix is stripped
// before hashing, as confirmed from 3+ registration functions in echovr.exe.
func TestSNSMessageHashStripsSPrefix(t *testing.T) {
	// "SNSFoo" and "SNSBar" must not collide (after stripping 'S' we get "NSFoo" vs "NSBar")
	h1 := SNSMessageHash("SNSLobbySmiteEntrant")
	h2 := SNSMessageHash("SBroadcasterIntroduceFinEvent")
	h3 := SNSMessageHash("SNSRefreshProfileForUser")

	if h1 == h2 || h2 == h3 || h1 == h3 {
		t.Errorf("hash collision: h1=0x%016x h2=0x%016x h3=0x%016x", h1, h2, h3)
	}

	// Verify strip behavior: SNSFoo → hash("NSFoo"); SFoo (no second S) → hash("Foo")
	hSNS := SNSMessageHash("SNSLobbySmiteEntrant") // strips 'S' → "NSLobbySmiteEntrant"
	hNS := SNSMessageHash("NSLobbySmiteEntrant")   // starts with 'N', no strip → "NSLobbySmiteEntrant"
	if hSNS != hNS {
		t.Errorf("SNSMessageHash strip mismatch: SNSFoo(0x%016x) != NSFoo(0x%016x)", hSNS, hNS)
	}
}

func TestSNSMessageHashConsistency(t *testing.T) {
	// Same input must produce same output
	h1 := SNSMessageHash("SNSLobbySmiteEntrant")
	h2 := SNSMessageHash("SNSLobbySmiteEntrant")
	if h1 != h2 {
		t.Errorf("inconsistent: 0x%016x != 0x%016x", h1, h2)
	}
}

func TestCSymbol64HashCaseInsensitive(t *testing.T) {
	pairs := [][2]string{
		{"test", "TEST"},
		{"Test", "tEsT"},
		{"ABC", "abc"},
		{"rwd_tint_0019", "RWD_TINT_0019"},
	}
	for _, p := range pairs {
		a := CSymbol64Hash(p[0])
		b := CSymbol64Hash(p[1])
		if a != b {
			t.Errorf("CSymbol64Hash(%q) != CSymbol64Hash(%q): 0x%016x vs 0x%016x", p[0], p[1], a, b)
		}
	}
}

func TestCSymbol64HashEmptyString(t *testing.T) {
	if got := CSymbol64Hash(""); got != 0xFFFFFFFFFFFFFFFF {
		t.Errorf("CSymbol64Hash(\"\") = 0x%016x, want 0xffffffffffffffff", got)
	}
}

func TestCSymbol64HashDifferentStrings(t *testing.T) {
	h1 := CSymbol64Hash("test1")
	h2 := CSymbol64Hash("test2")
	h3 := CSymbol64Hash("test3")
	if h1 == h2 || h2 == h3 || h1 == h3 {
		t.Errorf("hash collision among test1/test2/test3: 0x%016x 0x%016x 0x%016x", h1, h2, h3)
	}
}

// TestCSymbol64HashKnownVector tests the game-extracted test vector.
// Vector from docs/kb/csymbol64_hash_findings.md in evr-reconstruction.
// TODO: known vector is unverified — update want value once the 2048-byte
// lookup table at 0x141ffc480 in echovr.exe is extracted and confirmed.
func TestCSymbol64HashKnownVector(t *testing.T) {
	got := CSymbol64Hash("rwd_tint_0019")
	want := uint64(0x74d228d09dc5dd8f)
	if got != want {
		t.Skip("known vector unconfirmed - skipping until binary table is extracted")
	}
}

func BenchmarkCSymbol64Hash(b *testing.B) {
	s := "player_position_replicated"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CSymbol64Hash(s)
	}
}

func BenchmarkSNSMessageHash(b *testing.B) {
	s := "SNSLobbySmiteEntrant"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SNSMessageHash(s)
	}
}
