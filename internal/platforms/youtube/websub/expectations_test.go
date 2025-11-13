package websub

import "testing"

func TestRegisterLookupConsumeExpectation(t *testing.T) {
	token := "token123"
	exp := Expectation{VerifyToken: token, ChannelID: "abc"}
	RegisterExpectation(exp)
	t.Cleanup(func() { CancelExpectation(token) })

	if lookedUp, ok := LookupExpectation(token); !ok || lookedUp.ChannelID != "abc" {
		t.Fatalf("expected lookup to succeed")
	}

	if consumed, ok := ConsumeExpectation(token); !ok || consumed.ChannelID != "abc" {
		t.Fatalf("expected consume to succeed")
	}

	if _, ok := LookupExpectation(token); ok {
		t.Fatalf("expected token to be removed after consume")
	}
}

func TestCancelExpectation(t *testing.T) {
	token := "cancel"
	RegisterExpectation(Expectation{VerifyToken: token})
	CancelExpectation(token)
	if _, ok := LookupExpectation(token); ok {
		t.Fatalf("expected expectation to be cancelled")
	}
}

func TestExtractChannelID(t *testing.T) {
	if got := ExtractChannelID("https://www.youtube.com/xml/feeds/videos.xml?channel_id=UC123"); got != "UC123" {
		t.Fatalf("unexpected channel id %q", got)
	}
	if got := ExtractChannelID("invalid://"); got != "" {
		t.Fatalf("expected empty result on invalid url")
	}
}

func TestGenerateVerifyTokenReturnsValue(t *testing.T) {
	if GenerateVerifyToken() == "" {
		t.Fatalf("expected token to be generated")
	}
}
