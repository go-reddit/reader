package ui

import (
	"testing"

	"github.com/go-widgets/toolkit"
)

func loginScene() *Scene {
	s := NewScene()
	s.OpenLogin()
	s.Resize(1000, 700)
	return s
}

func TestOpenLogin(t *testing.T) {
	s := NewScene()
	s.loginID, s.loginSecret, s.loginErr = "x", "y", "z"
	s.OpenLogin()
	if s.Mode != ModeLogin || s.loginID != "" || s.loginSecret != "" || s.loginFocus != 0 || s.loginErr != "" {
		t.Fatalf("OpenLogin => %+v", s)
	}
}

func TestLoginTyping(t *testing.T) {
	s := loginScene()
	// Focus id (default) and type.
	s.TypeRune('a')
	s.TypeRune('b')
	if id, _ := s.LoginCredentials(); id != "ab" {
		t.Errorf("id = %q", id)
	}
	// Switch focus to secret.
	s.FocusLoginField(1)
	s.TypeRune('s')
	if _, sec := s.LoginCredentials(); sec != "s" {
		t.Errorf("secret = %q", sec)
	}
	// Backspace affects the focused field.
	s.Backspace()
	if _, sec := s.LoginCredentials(); sec != "" {
		t.Errorf("secret after backspace = %q", sec)
	}
	s.FocusLoginField(0)
	s.Backspace()
	if id, _ := s.LoginCredentials(); id != "a" {
		t.Errorf("id after backspace = %q", id)
	}
	// Invalid focus ignored.
	s.FocusLoginField(9)
	if s.loginFocus != 0 {
		t.Error("invalid focus should be ignored")
	}
}

func TestSetLoginError(t *testing.T) {
	s := loginScene()
	s.SetLoginError("Touch ID cancelled")
	if s.loginErr != "Touch ID cancelled" {
		t.Error("SetLoginError")
	}
}

func TestHitLogin(t *testing.T) {
	s := loginScene()
	s.loginLayout()
	m := s.m

	// Cancel.
	cw := m.tab.width("Cancel") + m.rpx(24)
	if h := s.HitTest(s.W-m.pad-cw/2, m.topbarH/2); h.Kind != HitLoginCancel {
		t.Errorf("cancel => %+v", h)
	}
	// Fields.
	if h := s.HitTest(s.loginIDR.X+10, s.loginIDR.Y+s.loginIDR.H/2); h.Kind != HitLoginField || h.Profile != 0 {
		t.Errorf("id field => %+v", h)
	}
	if h := s.HitTest(s.loginSecretR.X+10, s.loginSecretR.Y+s.loginSecretR.H/2); h.Kind != HitLoginField || h.Profile != 1 {
		t.Errorf("secret field => %+v", h)
	}
	// Submit.
	b := s.sButtons[0]
	if h := s.HitTest(b.rect.X+b.rect.W/2, b.rect.Y+b.rect.H/2); h.Kind != HitLoginSubmit {
		t.Errorf("submit => %+v", h)
	}
	// Empty space misses.
	if h := s.HitTest(s.W-m.pad, s.H-m.pad); h.Kind != HitNone {
		t.Errorf("empty => %+v", h)
	}
}

func TestDrawLoginSmoke(t *testing.T) {
	// Empty fields (placeholder path).
	s := loginScene()
	buf := make([]byte, s.W*s.H*4)
	s.Draw(buf)
	if allZero(buf) {
		t.Fatal("login draw empty")
	}
	// Filled + focused secret (caret + mask) + an error line + Extra OnAccent.
	s2 := loginScene()
	s2.SetTheme(toolkit.AdwaitaDark())
	s2.loginID = "id"
	s2.loginSecret = "secret"
	s2.loginFocus = 1
	s2.SetLoginError("bad credentials")
	s2.Draw(make([]byte, s2.W*s2.H*4))

	// Very small surface: the field width clamps to the viewport.
	s3 := loginScene()
	s3.Resize(MinW, MinH)
	s3.Draw(make([]byte, s3.W*s3.H*4))
}

func TestTrimLastRune(t *testing.T) {
	if trimLastRune("") != "" || trimLastRune("ab") != "a" {
		t.Error("trimLastRune")
	}
}

func TestNextLoginField(t *testing.T) {
	s := loginScene()
	if s.loginFocus != 0 {
		t.Fatal("starts on field 0")
	}
	s.NextLoginField()
	if s.loginFocus != 1 {
		t.Error("next -> 1")
	}
	s.NextLoginField()
	if s.loginFocus != 0 {
		t.Error("next -> 0")
	}
}
