// Package ui renders the reader — a Reddit-style topbar + sidebar + feed —
// into an RGBA pixel buffer. Chrome and cards are drawn with the go-widgets
// painter; text is anti-aliased TrueType (see text.go) so it stays clean at
// any zoom / Retina scale. The sidebar is organised into profiles (tabs) that
// each group a subset of subreddits — e.g. "Pro" and "Perso". The package has
// no build tag, so its layout, hit-testing and rendering are exercised by
// native `go test`; the wasm front-end (cmd/front) drives the same Scene.
package ui

import (
	"fmt"
	"strings"

	"github.com/go-reddit/reader/internal/settings"
	"github.com/go-reddit/reddit"
	"github.com/go-widgets/painter"
	"github.com/go-widgets/toolkit"
)

// Minimum sensible surface.
const (
	MinW = 360
	MinH = 240
)

// Display-zoom bounds and step.
const (
	MinZoom  = 0.5
	MaxZoom  = 3.0
	ZoomStep = 0.1
)

// Sorts is the ordered set of listing sorts shown as topbar tabs.
var Sorts = []string{"hot", "new", "top", "rising"}

// HitKind classifies what a click landed on.
type HitKind int

const (
	HitNone     HitKind = iota
	HitPost             // a feed post (open it)
	HitFeed             // a sidebar bookmark (switch subreddit)
	HitSort             // a topbar sort tab (switch sort)
	HitProfile          // a sidebar profile tab (switch active profile)
	HitSettings         // the sidebar settings entry (open preferences)

	// Settings-view actions (Mode == ModeSettings):
	HitTheme         // Value = "system"|"light"|"dark"
	HitSelectProfile // Profile = index being edited
	HitRemoveFeed    // Profile = index, Value = feed
	HitAddFeed       // commit the input into the edited profile
	HitNewProfile
	HitDeleteProfile // Profile = index
	HitCloseSettings

	// Login-view actions (Mode == ModeLogin):
	HitLoginField  // Profile = 0 (client id) or 1 (client secret)
	HitLoginSubmit // submit the credentials (Touch ID)
	HitLoginCancel

	// Sidebar account entry (feed view):
	HitOpenLogin // open the login form
	HitLogout    // forget stored credentials

	// Topbar search field (feed view):
	HitSearch // focus the filter/search input
)

// Hit is the result of [Scene.HitTest].
type Hit struct {
	Kind    HitKind
	Post    reddit.Post // HitPost
	Feed    string      // HitFeed ("" = front page)
	Sort    string      // HitSort
	Profile int         // HitProfile / HitSelectProfile / HitRemoveFeed / HitDeleteProfile
	Value   string      // HitTheme / HitRemoveFeed
}

// Mode selects which view the scene renders.
type Mode int

const (
	ModeFeed     Mode = iota // the topbar + sidebar + feed
	ModeSettings             // the in-canvas preferences editor
	ModeLogin                // the in-canvas Reddit login form
)

// Scene is the mutable reader state.
type Scene struct {
	W, H int

	theme     *toolkit.Theme
	Subreddit string // selected subreddit ("" => front page)
	Sort      string
	Profiles  []settings.Profile // sidebar tabs
	Active    int                // active profile index
	Posts     []reddit.Post
	Status    string
	ScrollY   int
	Scale     float64 // display scale (zoom × devicePixelRatio); 0 => 1

	// Topbar filter/search. The term lives on a persistent toolkit.SearchEntry
	// (built once in NewScene, edited through its OnEvent) instead of a plain
	// string field, so the field is a real widget that draws its own magnifier
	// (an Iconoir glyph), placeholder and caret. searchR is its hit/paint rect.
	searchEntry   *toolkit.SearchEntry
	searchFocused bool
	searchR       toolkit.Rect

	// Preferences view. The "add subreddit" field is a persistent toolkit.Entry
	// (built once in NewScene, edited through its OnEvent), not a plain string
	// rebuilt every frame.
	Mode          Mode
	ThemeName     string // "system"|"light"|"dark" (persisted)
	selEdit       int    // profile being edited in the settings view
	settingsEntry *toolkit.Entry
	LoggedIn      bool // reflects the auth state (drives the sidebar label)

	// Login view. The client-id + secret fields are persistent toolkit.Entry
	// widgets (the secret one masked); they own the typed values. The submit +
	// cancel buttons are persistent toolkit.Button widgets, built once.
	loginIDEntry     *toolkit.Entry
	loginSecretEntry *toolkit.Entry
	loginFocus       int // 0 = client id, 1 = client secret
	loginErr         string
	loginSubmitBtn   *toolkit.Button
	loginCancelBtn   *toolkit.Button
	loginHits        []widgetHit

	// Sidebar widgets. The profile tabs are a toolkit.FolderTabs (owns its own
	// tab geometry + selection) and the FEEDS list is a toolkit.TreeView (owns
	// row layout, scroll and hit-testing) with a RowRenderer painting each feed
	// label — so no scene-side rect slice mirrors the sidebar draw. Only the two
	// bottom-pinned chrome rows (Account, Settings) keep a rect: they are fixed,
	// single-source (computed once in layout, read by both Draw and HitTest) and
	// sit outside the scrolling list, exactly like go-news-reader's pinned footer.
	profBar   *toolkit.FolderTabs
	sideTree  *toolkit.TreeView
	settingsR toolkit.Rect
	accountR  toolkit.Rect

	// Empty-feed placeholder. A persistent toolkit.EmptyState (built once in
	// NewScene, repositioned each frame) shown when the feed has no posts; it
	// keeps its own state across frames instead of being rebuilt per paint.
	emptyState *toolkit.EmptyState

	m        metrics
	tabs     []tabHit
	rows     []rowLayout
	contentH int

	// Settings-view widgets, all persistent (built once in NewScene, repositioned
	// each layout): the theme + sort pill rows, the profile switcher pills, the
	// per-feed removable chips and the New/Add/Delete/Done buttons. settingsHits
	// pairs each with the Hit its own HitTest resolves to.
	themeButtons   []*toolkit.Button
	sortButtons    []*toolkit.Button
	profileButtons []*toolkit.Button
	feedChips      []*toolkit.Chip
	newProfileBtn  *toolkit.Button
	addBtn         *toolkit.Button
	deleteBtn      *toolkit.Button
	doneBtn        *toolkit.Button
	settingsHits   []widgetHit
}

// widgetHit pairs an interactive widget with the Hit its own HitTest resolves
// to: a view builds a []widgetHit each layout (setting each widget's Bounds),
// then hit-testing is a loop over the widgets' own HitTest — no parallel rect
// slice kept in sync with the draw.
type widgetHit struct {
	w   toolkit.Widget
	hit Hit
}

type rowLayout struct {
	top  int
	post reddit.Post
}
type tabHit struct {
	rect toolkit.Rect
	sort string
}

// NewScene returns a Scene at the default size with the light theme and the
// default (Pro/Perso) profiles.
func NewScene() *Scene {
	d := settings.Default()
	se := toolkit.NewSearchEntry("")
	se.Icon = drawSearchIcon // a real Iconoir magnifier, not a bitmap stand-in
	settingsEntry := toolkit.NewEntry("")
	settingsEntry.Placeholder = "add subreddit…"
	loginSecretEntry := toolkit.NewEntry("")
	loginSecretEntry.Mask = '•' // display the secret masked; Value() keeps the real text

	// The FEEDS list is a TreeView with a synthetic hidden root (its children are
	// the flush, depth-0 feed rows); its own scrollbar is suppressed and the
	// RowRenderer paints each feed label. The profile tabs are a compact,
	// label-width FolderTabs.
	sideTree := toolkit.NewTreeView(nil)
	sideTree.HideRoot = true
	sideTree.HideScrollbar = true

	s := &Scene{
		W:                900,
		H:                660,
		theme:            toolkit.DefaultLight(),
		Sort:             d.Sort,
		Scale:            1,
		Profiles:         d.Profiles,
		Active:           d.Active,
		ThemeName:        d.Theme,
		searchEntry:      se,
		settingsEntry:    settingsEntry,
		loginIDEntry:     toolkit.NewEntry(""),
		loginSecretEntry: loginSecretEntry,
		loginSubmitBtn:   toolkit.NewButton("Log in with Touch ID", nil),
		loginCancelBtn:   toolkit.NewButton("Cancel", nil),
		profBar:          toolkit.NewFolderTabs(nil, 0),
		sideTree:         sideTree,
		emptyState:       toolkit.NewEmptyState("No posts loaded."),
		newProfileBtn:    toolkit.NewButton("+ New", nil),
		addBtn:           toolkit.NewButton("Add", nil),
		deleteBtn:        toolkit.NewButton("Delete profile", nil),
		doneBtn:          toolkit.NewButton("Done", nil),
	}
	s.sideTree.RowRenderer = s.drawSideRow
	s.loginSubmitBtn.Style = toolkit.ButtonProminent
	s.deleteBtn.Style = toolkit.ButtonDanger
	// The theme + sort pill rows are fixed-length, so their buttons are built once.
	for _, tn := range []string{"system", "light", "dark"} {
		s.themeButtons = append(s.themeButtons, toolkit.NewButton(titleCase(tn), nil))
	}
	for _, srt := range Sorts {
		s.sortButtons = append(s.sortButtons, toolkit.NewButton(srt, nil))
	}
	return s
}

// SetTheme swaps the palette. A nil theme is ignored.
func (s *Scene) SetTheme(t *toolkit.Theme) {
	if t != nil {
		s.theme = t
	}
}

// SetFeed records the current subreddit/sort selection. subreddit "" is the
// front page.
func (s *Scene) SetFeed(subreddit, sort string) {
	s.Subreddit = strings.TrimPrefix(strings.TrimSpace(subreddit), "r/")
	if sort == "" {
		sort = "hot"
	}
	s.Sort = sort
}

// SetProfiles replaces the profile list and active index (clamped).
func (s *Scene) SetProfiles(profiles []settings.Profile, active int) {
	s.Profiles = profiles
	s.SetActive(active)
}

// SetActive selects the active profile (clamped to a valid index).
func (s *Scene) SetActive(i int) {
	if i < 0 || i >= len(s.Profiles) {
		i = 0
	}
	s.Active = i
}

// ActiveFeeds returns the subreddits of the active profile.
func (s *Scene) ActiveFeeds() []string {
	if s.Active >= 0 && s.Active < len(s.Profiles) {
		return s.Profiles[s.Active].Feeds
	}
	return nil
}

// SetPosts replaces the visible posts and resets scroll.
func (s *Scene) SetPosts(posts []reddit.Post) {
	s.Posts = posts
	s.ScrollY = 0
}

// FocusSearch sets/clears focus on the topbar filter field.
func (s *Scene) FocusSearch(v bool) { s.searchFocused = v }

// SearchFocused reports whether the topbar filter field has focus.
func (s *Scene) SearchFocused() bool { return s.searchFocused }

// Search returns the current filter term (the SearchEntry's committed text).
func (s *Scene) Search() string { return s.searchEntry.Text().Get() }

// SetSearch replaces the filter term and resets scroll.
func (s *Scene) SetSearch(q string) {
	s.searchEntry.Text().Set(q)
	s.ScrollY = 0
}

// FeedName returns a human-readable label for the current feed.
func (s *Scene) FeedName() string { return s.feedLabel() }

func (s *Scene) feedLabel() string {
	if s.Subreddit == "" {
		return "Front page · " + s.Sort
	}
	return "r/" + s.Subreddit + " · " + s.Sort
}

func (s *Scene) effScale() float64 {
	if s.Scale <= 0 {
		return 1
	}
	return s.Scale
}

// Resize sets the surface to w×h pixels (clamped) and re-clamps scroll.
func (s *Scene) Resize(w, h int) {
	if w < MinW {
		w = MinW
	}
	if h < MinH {
		h = MinH
	}
	s.W, s.H = w, h
	s.layout()
	if m := s.MaxScroll(); s.ScrollY > m {
		s.ScrollY = m
	}
}

func (s *Scene) viewportH() int { return s.H - s.m.topbarH - s.m.footerH }

// MaxScroll is the largest valid ScrollY for the current content.
func (s *Scene) MaxScroll() int {
	if m := s.contentH - s.viewportH(); m > 0 {
		return m
	}
	return 0
}

// Scroll adjusts the vertical scroll by dy, clamped. Returns whether it moved.
func (s *Scene) Scroll(dy int) bool {
	s.layout()
	old := s.ScrollY
	s.ScrollY += dy
	if s.ScrollY < 0 {
		s.ScrollY = 0
	}
	if m := s.MaxScroll(); s.ScrollY > m {
		s.ScrollY = m
	}
	return s.ScrollY != old
}

const appTitle = "go-reddit"

// layout recomputes metrics + the topbar tabs, sidebar widgets and feed rows.
// It also pins the toolkit metric scale to the reader's effective scale so the
// widgets' own geometry (FolderTabs tab widths, TreeView rows, button pads) is
// computed identically whether it is a Draw or a HitTest asking.
func (s *Scene) layout() {
	toolkit.SetMetricScale(s.effScale())
	s.m = computeMetrics(s.effScale())
	m := s.m

	// Topbar sort tabs, after the app title.
	s.tabs = s.tabs[:0]
	x := 2*m.pad + m.header.width(appTitle)
	for _, srt := range Sorts {
		w := m.tab.width(srt) + 2*m.tabPad
		s.tabs = append(s.tabs, tabHit{rect: toolkit.Rect{X: x, Y: 0, W: w, H: m.topbarH}, sort: srt})
		x += w
	}

	// Topbar filter field, right-aligned in the blue bar.
	fieldH := m.tab.height + m.rpx(8)
	searchW := m.rpx(240)
	if min := x + m.pad; m.pad*2+searchW > s.W-min { // don't overlap the tabs
		searchW = s.W - min - m.pad
	}
	if searchW < m.rpx(80) {
		searchW = m.rpx(80)
	}
	s.searchR = toolkit.Rect{X: s.W - m.pad - searchW, Y: (m.topbarH - fieldH) / 2, W: searchW, H: fieldH}

	// Sidebar profile tabs (top row): a FolderTabs owning its own tab geometry
	// and selection. Labels track the profiles; Selected tracks the active one.
	s.profBar.Font = m.tab.font
	s.profBar.Labels = profileNames(s.Profiles)
	s.profBar.Selected().Set(s.Active)
	s.profBar.SetBounds(toolkit.Rect{X: 0, Y: m.topbarH, W: m.sidebarW, H: m.profileTabH})

	// Settings + account entries pinned to the bottom of the sidebar.
	s.settingsR = toolkit.Rect{X: 0, Y: s.H - m.footerH - m.sideItemH, W: m.sidebarW, H: m.sideItemH}
	s.accountR = toolkit.Rect{X: 0, Y: s.settingsR.Y - m.sideItemH, W: m.sidebarW, H: m.sideItemH}

	// Sidebar FEEDS list: a TreeView occupying the band between the "FEEDS"
	// header and the pinned account entry. It owns row layout + hit-testing.
	feedsTop := m.topbarH + m.profileTabH + m.sideItemH
	feedsH := s.accountR.Y - feedsTop
	if feedsH < 0 {
		feedsH = 0
	}
	s.sideTree.RowHeight = m.sideItemH
	s.buildSideTree()
	s.sideTree.SetBounds(toolkit.Rect{X: 0, Y: feedsTop, W: m.sidebarW, H: feedsH})

	// Feed rows (filtered by the topbar search term, by title).
	s.rows = s.rows[:0]
	y := m.pad
	q := strings.ToLower(strings.TrimSpace(s.searchEntry.Text().Get()))
	for _, p := range s.Posts {
		if q != "" && !strings.Contains(strings.ToLower(p.Title), q) {
			continue
		}
		s.rows = append(s.rows, rowLayout{top: y, post: p})
		y += m.rowH + m.rowGap
	}
	s.contentH = y
}

// HitTest maps a click to an action.
func (s *Scene) HitTest(x, y int) Hit {
	switch s.Mode {
	case ModeSettings:
		return s.hitSettings(x, y)
	case ModeLogin:
		return s.hitLogin(x, y)
	}
	s.layout()
	m := s.m
	switch {
	case y < m.topbarH:
		if s.searchR.Contains(x, y) {
			return Hit{Kind: HitSearch}
		}
		for _, t := range s.tabs {
			if t.rect.Contains(x, y) {
				return Hit{Kind: HitSort, Sort: t.sort}
			}
		}
		return Hit{}
	case y >= s.H-m.footerH:
		return Hit{}
	case x < m.sidebarW:
		// Profile tabs: the FolderTabs' own per-tab geometry answers "which tab".
		for i := range s.Profiles {
			if s.profBar.TabRect(i).Contains(x, y) {
				return Hit{Kind: HitProfile, Profile: i}
			}
		}
		if s.settingsR.Contains(x, y) {
			return Hit{Kind: HitSettings}
		}
		if s.accountR.Contains(x, y) {
			if s.LoggedIn {
				return Hit{Kind: HitLogout}
			}
			return Hit{Kind: HitOpenLogin}
		}
		// FEEDS list: the TreeView maps the click to the node under it (band-local
		// coordinates); empty space or a row scrolled out resolves to no node.
		b := s.sideTree.Bounds()
		if node := s.sideTree.NodeAt(x, y-b.Y); node != nil {
			return Hit{Kind: HitFeed, Feed: feedNodeData(node).feed}
		}
		return Hit{}
	default:
		cy := y - m.topbarH + s.ScrollY
		for _, r := range s.rows {
			if cy >= r.top && cy < r.top+m.rowH && x >= m.sidebarW+m.pad && x < s.W-m.pad {
				return Hit{Kind: HitPost, Post: r.post}
			}
		}
		return Hit{}
	}
}

// Draw paints the whole scene into buf (s.W*s.H*4 RGBA bytes).
func (s *Scene) Draw(buf []byte) {
	switch s.Mode {
	case ModeSettings:
		s.drawSettings(buf)
		return
	case ModeLogin:
		s.drawLogin(buf)
		return
	}
	s.layout()
	m := s.m
	p := painter.NewPixelPainter(buf, s.W, s.H)
	th := s.theme
	// The composite widgets (PostCard, SearchEntry, Statusbar, EmptyState) scale
	// their own logical pads by the toolkit metric scale and fall back to the
	// active font for any text they own, so pin both to the reader's zoom + shaped
	// face before composing the frame.
	toolkit.SetMetricScale(s.effScale())
	toolkit.SetFont(m.title.font)
	onAccent := th.Background
	if v, ok := th.Extra["OnAccent"]; ok {
		onAccent = v
	}
	muteS := mute(th.OnSurface, th.Surface)

	fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: s.H}, th.Background)

	// --- feed (drawn first; chrome overpaints any scroll overflow) ---
	feedTop := m.topbarH
	for _, r := range s.rows {
		screenY := feedTop + r.top - s.ScrollY
		if screenY+m.rowH < feedTop || screenY >= s.H-m.footerH {
			continue
		}
		s.drawPost(p, r.post, screenY)
	}
	if len(s.Posts) == 0 {
		// Empty feed: the persistent toolkit.EmptyState centred in the feed
		// viewport (repositioned each frame, not rebuilt).
		s.emptyState.SetBounds(toolkit.Rect{X: m.sidebarW, Y: feedTop, W: s.W - m.sidebarW, H: s.viewportH()})
		s.emptyState.Draw(p, th)
	}

	// Vertical scrollbar for the feed when it overflows its viewport. Drawn over
	// the feed region (between the topbar and footer, right of the sidebar) so no
	// later chrome overpaints it.
	s.drawVScrollbar(p, toolkit.Rect{X: m.sidebarW, Y: feedTop, W: s.W - m.sidebarW, H: s.viewportH()}, s.contentH, s.ScrollY)

	// --- sidebar ---
	fillBox(p, th, painter.Rect{X: 0, Y: m.topbarH, W: m.sidebarW, H: s.H - m.topbarH - m.footerH}, th.SurfaceAlt)

	// Profile tabs: the FolderTabs paints its own strip + tabs + selection.
	s.profBar.Draw(p, th)

	// FEEDS header + the FEEDS TreeView. The tree paints each row's selection
	// fill itself; drawn with a theme whose Surface is the sidebar band so the
	// non-selected rows blend into it (only the selected row gets the accent).
	headerY := m.topbarH + m.profileTabH
	m.side.labelAt(p, th, m.pad, headerY+(m.sideItemH-m.side.height)/2, "FEEDS", muteS)
	sideTheme := *th
	sideTheme.Surface = th.SurfaceAlt
	s.sideTree.Draw(p, &sideTheme)

	// Account + Settings entries (pinned bottom) + sidebar border. Each carries a
	// real Iconoir glyph (log-in / log-out / gear) beside its label.
	account, accIcon := "Log in", iconLogIn
	if s.LoggedIn {
		account, accIcon = "Log out", iconLogOut
	}
	fillBox(p, th, painter.Rect{X: 0, Y: s.accountR.Y - 1, W: m.sidebarW, H: 1}, th.Border)
	s.drawSideEntry(p, th, s.accountR, accIcon, account, muteS)
	s.drawSideEntry(p, th, s.settingsR, iconSettings, "Settings", muteS)
	fillBox(p, th, painter.Rect{X: m.sidebarW - 1, Y: m.topbarH, W: 1, H: s.H - m.topbarH - m.footerH}, th.Border)

	// --- topbar ---
	fillBox(p, th, painter.Rect{X: 0, Y: 0, W: s.W, H: m.topbarH}, th.Accent)
	m.header.labelAt(p, th, m.pad, (m.topbarH-m.header.height)/2, appTitle, onAccent)
	for _, t := range s.tabs {
		active := t.sort == s.Sort
		col := mute(onAccent, th.Accent)
		if active {
			col = onAccent
			fillBox(p, th, painter.Rect{X: t.rect.X, Y: m.topbarH - m.rpx(3), W: t.rect.W, H: m.rpx(3)}, onAccent)
		}
		m.tab.labelAt(p, th, t.rect.X+m.tabPad, (m.topbarH-m.tab.height)/2, t.sort, col)
	}

	// Topbar filter field: the persistent SearchEntry (draws its own Iconoir
	// magnifier, placeholder and caret; focused state mirrors s.searchFocused).
	se := s.searchEntry
	se.Font = m.tab.font
	se.SetFocused(s.searchFocused)
	se.SetBounds(s.searchR)
	se.Draw(p, th)

	// --- footer: a toolkit.Statusbar with a left status cell + a right hint ---
	// Both segments are deliberately non-interactive: the post-count and the
	// keyboard/scroll hint are read-only status, nothing to click. They carry no
	// StatusSegment.OnClick, so the Statusbar renders (and hit-tests) them as
	// plain text cells — the reader has no footer action to wire, so this stays a
	// static status bar rather than an interactive one.
	status := s.Status
	if status == "" {
		status = fmt.Sprintf("%d posts", len(s.Posts))
	}
	sb := &toolkit.Statusbar{
		Left:  []toolkit.StatusSegment{{Text: status}},
		Right: []toolkit.StatusSegment{{Text: "click a feed · scroll · Cmd +/- zoom"}},
	}
	sb.Font = m.meta.font
	sb.SetBounds(toolkit.Rect{X: 0, Y: s.H - m.footerH, W: s.W, H: m.footerH})
	sb.Draw(p, th)
}

// drawSideEntry paints a pinned sidebar row (account / settings) as an Iconoir
// glyph followed by its label, both in ink, vertically centred in the row.
func (s *Scene) drawSideEntry(p painter.Painter, th *toolkit.Theme, r toolkit.Rect, icon, label string, ink toolkit.RGBA) {
	m := s.m
	d := m.side.height
	drawIcon(p, toolkit.Rect{X: m.pad, Y: r.Y + (m.sideItemH-d)/2, W: d, H: d}, icon, ink)
	m.side.labelAt(p, th, m.pad+d+m.rpx(6), r.Y+(m.sideItemH-m.side.height)/2, label, ink)
}

// drawPost renders one post as a toolkit.PostCard at screen y: the compact score
// as the coloured pill, the flair as the badge-row subtitle, the wrapped title
// (capped at two lines) and the "u/author · N comments · domain" meta line — a
// real widget, not a hand-drawn rounded rect plus blitted glyphs.
func (s *Scene) drawPost(p painter.Painter, post reddit.Post, y int) {
	m := s.m
	pc := &toolkit.PostCard{
		Pill:          formatScore(post.Score),
		PillColor:     s.theme.Accent,
		Subtitle:      post.Flair,
		Title:         post.Title,
		Meta:          metaLine(post),
		MaxTitleLines: 2,
		TitleFont:     m.title.font,
		SubtitleFont:  m.meta.font,
		MetaFont:      m.meta.font,
		PillFont:      m.score.font,
	}
	pc.SetBounds(toolkit.Rect{X: m.sidebarW + m.pad, Y: y, W: s.W - m.sidebarW - 2*m.pad, H: m.rowH})
	pc.Draw(p, s.theme)
}

// --- metrics ---------------------------------------------------------------

type metrics struct {
	scale                                         float64
	pad, topbarH, sidebarW, footerH, rowH, rowGap int
	sideItemH, tabPad, profileTabH                int
	title, meta, score, header, side, tab         textFace
}

func (m metrics) rpx(base float64) int { return scalePx(base, m.scale) }

func scalePx(base, scale float64) int {
	n := int(base*scale + 0.5)
	if n < 1 {
		n = 1
	}
	return n
}

func computeMetrics(scale float64) metrics {
	m := metrics{scale: scale}
	m.pad = scalePx(10, scale)
	m.title = getFace(scalePx(14, scale), false)
	m.meta = getFace(scalePx(11, scale), false)
	m.score = getFace(scalePx(14, scale), true)
	m.header = getFace(scalePx(15, scale), true)
	m.side = getFace(scalePx(13, scale), false)
	m.tab = getFace(scalePx(13, scale), true)
	m.sidebarW = scalePx(180, scale)
	m.rowGap = scalePx(8, scale)
	m.tabPad = scalePx(10, scale)
	m.sideItemH = m.side.height + scalePx(10, scale)
	m.profileTabH = m.tab.height + scalePx(12, scale)
	m.topbarH = m.header.height + 2*m.pad
	m.footerH = m.meta.height + m.pad
	m.rowH = 2*m.pad + 2*m.title.height + scalePx(4, scale) + m.meta.height
	return m
}

// --- helpers ---------------------------------------------------------------

// ClampZoom constrains z to [MinZoom, MaxZoom].
func ClampZoom(z float64) float64 {
	switch {
	case z < MinZoom:
		return MinZoom
	case z > MaxZoom:
		return MaxZoom
	default:
		return z
	}
}

// StepZoom moves zoom one step (+1 in, -1 out, 0 none), snapped to the grid.
func StepZoom(z float64, dir int) float64 {
	switch {
	case dir > 0:
		z += ZoomStep
	case dir < 0:
		z -= ZoomStep
	}
	z = float64(int(z/ZoomStep+0.5)) * ZoomStep
	return ClampZoom(z)
}

// metaLine builds the "u/author · N comments · domain" descriptor.
func metaLine(p reddit.Post) string {
	parts := []string{"u/" + p.Author, fmt.Sprintf("%d comments", p.NumComments)}
	if p.Domain != "" {
		parts = append(parts, p.Domain)
	}
	return strings.Join(parts, "  ·  ")
}

// formatScore renders a vote count compactly (1200 -> "1.2k").
func formatScore(n int) string {
	switch {
	case n >= 100000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// mute blends fg 55% toward bg for secondary text.
func mute(fg, bg toolkit.RGBA) toolkit.RGBA {
	blend := func(a, b uint8) uint8 { return uint8((int(a)*45 + int(b)*55) / 100) }
	return toolkit.RGBA{R: blend(fg.R, bg.R), G: blend(fg.G, bg.G), B: blend(fg.B, bg.B), A: 0xFF}
}
