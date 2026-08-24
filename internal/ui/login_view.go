package ui

import (
	"image"

	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// The in-canvas Reddit login form (go-widgets, no HTML). It collects a Reddit
// application's client id + secret (from reddit.com/prefs/apps); submitting
// runs the host-side Touch ID prompt and stores them in the Keychain. Two
// focusable fields, a submit and a cancel button — all sButtons so drawing and
// hit-testing stay in sync.

// OpenLogin enters the login view with empty fields.
func (s *Scene) OpenLogin() {
	s.Mode = ModeLogin
	s.loginID, s.loginSecret, s.loginFocus, s.loginErr = "", "", 0, ""
}

// LoginCredentials returns the typed client id + secret.
func (s *Scene) LoginCredentials() (id, secret string) { return s.loginID, s.loginSecret }

// SetLoginError shows an error under the form (e.g. a cancelled Touch ID).
func (s *Scene) SetLoginError(msg string) { s.loginErr = msg }

// FocusLoginField selects which field receives typed text (0 id, 1 secret).
func (s *Scene) FocusLoginField(i int) {
	if i == 0 || i == 1 {
		s.loginFocus = i
	}
}

// NextLoginField moves focus to the other field (Tab).
func (s *Scene) NextLoginField() { s.loginFocus = 1 - s.loginFocus }

func (s *Scene) loginTypeRune(r rune) {
	if s.loginFocus == 1 {
		s.loginSecret += string(r)
	} else {
		s.loginID += string(r)
	}
}

func (s *Scene) loginBackspace() {
	if s.loginFocus == 1 {
		s.loginSecret = trimLastRune(s.loginSecret)
	} else {
		s.loginID = trimLastRune(s.loginID)
	}
}

// loginLayout computes the field + button rectangles.
func (s *Scene) loginLayout() {
	s.m = computeMetrics(s.effScale())
	m := s.m
	s.sButtons = s.sButtons[:0]

	fieldW := m.rpx(360)
	if fieldW > s.W-2*m.pad {
		fieldW = s.W - 2*m.pad
	}
	rowH := m.tab.height + m.rpx(10)
	y := m.topbarH + m.pad*3 + m.side.height // below the intro text

	s.loginIDR = toolkit.Rect{X: m.pad, Y: y + m.side.height, W: fieldW, H: rowH}
	y = s.loginIDR.Y + rowH + m.pad + m.side.height
	s.loginSecretR = toolkit.Rect{X: m.pad, Y: y, W: fieldW, H: rowH}

	by := s.loginSecretR.Y + rowH + m.pad
	bw := m.tab.width("Log in with Touch ID") + m.rpx(24)
	s.sButtons = append(s.sButtons,
		sButton{rect: toolkit.Rect{X: m.pad, Y: by, W: bw, H: rowH}, label: "Log in with Touch ID", kind: HitLoginSubmit},
	)
}

// drawLogin paints the login form.
func (s *Scene) drawLogin(buf []byte) {
	s.loginLayout()
	m := s.m
	p := painter.NewPixelPainter(buf, s.W, s.H)
	img := &image.RGBA{Pix: buf, Stride: s.W * 4, Rect: image.Rect(0, 0, s.W, s.H)}
	th := s.theme
	onAccent := th.Background
	if v, ok := th.Extra["OnAccent"]; ok {
		onAccent = v
	}
	muteS := mute(th.OnSurface, th.Surface)

	fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: s.H}, th.Background)

	// Intro.
	introY := m.topbarH + m.pad*2
	m.side.draw(img, m.pad, introY, "Enter your Reddit app's client id + secret (reddit.com/prefs/apps).", muteS)

	// Field labels + boxes are stock go-widgets: toolkit.Label captions (muted)
	// over toolkit.Entry fields (the secret one masked by the toolkit's own Mask),
	// with the caret parked at the end since the reader drives the text itself.
	labelFont := ttFont(false, m.side.px)
	fieldFont := ttFont(true, m.tab.px)
	drawField := func(label string, r toolkit.Rect, text string, secret, focused bool) {
		lbl := toolkit.NewLabel(label)
		lbl.Font, lbl.Ink = labelFont, muteS
		lbl.SetBounds(toolkit.Rect{X: r.X, Y: r.Y - m.side.height - m.rpx(2), W: r.W, H: m.side.height})
		lbl.Draw(p, th)

		e := toolkit.NewEntry(text)
		e.Placeholder = "…"
		e.SetFocused(focused)
		e.Font = fieldFont
		if secret {
			e.Mask = '•' // the toolkit masks the display; Text keeps the real value
		}
		e.SetBounds(r)
		e.Draw(p, th)
	}
	drawField("CLIENT ID", s.loginIDR, s.loginID, false, s.loginFocus == 0)
	drawField("CLIENT SECRET", s.loginSecretR, s.loginSecret, true, s.loginFocus == 1)

	// Submit button is a toolkit.Button (accent-primary style).
	for _, b := range s.sButtons {
		w := toolkit.NewButton(b.label, nil)
		w.Style = toolkit.ButtonProminent
		w.Font = fieldFont
		w.SetBounds(b.rect)
		w.Draw(p, th)
	}
	if s.loginErr != "" {
		errY := s.sButtons[len(s.sButtons)-1].rect.Y + s.sButtons[len(s.sButtons)-1].rect.H + m.pad
		m.side.draw(img, m.pad, errY, s.loginErr, rgb(0xD03030))
	}

	// Topbar + Cancel (drawn last).
	fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: m.topbarH}, th.Accent)
	m.header.draw(img, m.pad, (m.topbarH-m.header.height)/2, "Log in to Reddit", onAccent)
	cw := m.tab.width("Cancel") + m.rpx(24)
	cr := toolkit.Rect{X: s.W - m.pad - cw, Y: (m.topbarH - (m.tab.height + m.rpx(8))) / 2, W: cw, H: m.tab.height + m.rpx(8)}
	fillRoundBox(p, th, cr, m.rpx(6), onAccent)
	m.tab.draw(img, cr.X+m.rpx(12), cr.Y+(cr.H-m.tab.height)/2, "Cancel", th.Accent)
}

// hitLogin maps a click in the login view to an action.
func (s *Scene) hitLogin(x, y int) Hit {
	s.loginLayout()
	m := s.m
	cw := m.tab.width("Cancel") + m.rpx(24)
	cancel := toolkit.Rect{X: s.W - m.pad - cw, Y: (m.topbarH - (m.tab.height + m.rpx(8))) / 2, W: cw, H: m.tab.height + m.rpx(8)}
	if cancel.Contains(x, y) {
		return Hit{Kind: HitLoginCancel}
	}
	if s.loginIDR.Contains(x, y) {
		return Hit{Kind: HitLoginField, Profile: 0}
	}
	if s.loginSecretR.Contains(x, y) {
		return Hit{Kind: HitLoginField, Profile: 1}
	}
	for _, b := range s.sButtons {
		if b.rect.Contains(x, y) {
			return Hit{Kind: b.kind}
		}
	}
	return Hit{}
}
