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
	s.loginIDEntry.SetText("x")
	s.loginSecretEntry.SetText("y")
	s.loginErr = "z"
	s.OpenLogin()
	id, sec := s.LoginCredentials()
	if s.Mode != ModeLogin || id != "" || sec != "" || s.loginFocus != 0 || s.loginErr != "" {
		t.Fatalf("OpenLogin => mode=%d id=%q sec=%q focus=%d err=%q", s.Mode, id, sec, s.loginFocus, s.loginErr)
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

	// Cancel, fields and submit all resolve through their persistent widgets'
	// own bounds — no rect math in hitLogin.
	cr := s.loginCancelBtn.Bounds()
	if h := s.HitTest(cr.X+cr.W/2, cr.Y+cr.H/2); h.Kind != HitLoginCancel {
		t.Errorf("cancel => %+v", h)
	}
	idr := s.loginIDEntry.Bounds()
	if h := s.HitTest(idr.X+10, idr.Y+idr.H/2); h.Kind != HitLoginField || h.Profile != 0 {
		t.Errorf("id field => %+v", h)
	}
	secr := s.loginSecretEntry.Bounds()
	if h := s.HitTest(secr.X+10, secr.Y+secr.H/2); h.Kind != HitLoginField || h.Profile != 1 {
		t.Errorf("secret field => %+v", h)
	}
	br := s.loginSubmitBtn.Bounds()
	if h := s.HitTest(br.X+br.W/2, br.Y+br.H/2); h.Kind != HitLoginSubmit {
		t.Errorf("submit => %+v", h)
	}
	// Empty space misses.
	if h := s.HitTest(s.W-s.m.pad, s.H-s.m.pad); h.Kind != HitNone {
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
	s2.loginIDEntry.SetText("id")
	s2.loginSecretEntry.SetText("secret")
	s2.loginFocus = 1
	s2.SetLoginError("bad credentials")
	s2.Draw(make([]byte, s2.W*s2.H*4))

	// Very small surface: the field width clamps to the viewport.
	s3 := loginScene()
	s3.Resize(MinW, MinH)
	s3.Draw(make([]byte, s3.W*s3.H*4))
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
